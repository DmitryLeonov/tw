package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"twserver/internal/game"
)

// clientState tracks per-player WebSocket connection state including a
// generation counter that invalidates stale connections on reconnect.
type clientState struct {
	conn       *websocket.Conn
	outbound   chan game.Event
	generation int64
	closed     bool
}

// RoundManager is the single authoritative coordinator for game rounds.
// It receives actions from WebSocket handlers, submits them to the engine,
// and broadcasts TURN_RESOLVED + OBSERVATION events to all connected clients.
//
// Architecture:
//   - One run() goroutine serialises all action processing and round resolution.
//   - Each player has a per-player action channel; the WS handler writes actions
//     to this channel instead of calling the engine directly.
//   - The run() goroutine reads actions, calls engine.Submit(), queues
//     ACTION_ACCEPTED, then monitors the engine's turnChanged channel.
//   - When a turn resolves (all submitted or timeout), the run() goroutine
//     broadcasts TURN_RESOLVED followed by OBSERVATION to every connected client.
//
// Event ordering guarantee (per client):
//
//	ACTION_ACCEPTED → TURN_RESOLVED → OBSERVATION
//
// This is guaranteed because all events are queued from the single run() goroutine.
type RoundManager struct {
	engine *game.Engine

	mu      sync.Mutex
	clients map[string]*clientState // playerID -> current clientState
	gen     int64                   // monotonically increasing generation counter

	actionCh chan playerAction // actions from WS handlers

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	gameStarted chan struct{} // closed when registration delay expires or game is manually started

	paused   bool
	pauseCh  chan struct{}
	pauseMu  sync.Mutex

	resultsDir string // directory for saving results files
}

type playerAction struct {
	playerID string
	msg      game.ActionMessage
	outbound chan game.Event // where to send ACTION_ACCEPTED / ERROR
}

func NewRoundManager(engine *game.Engine, gameStarted chan struct{}) *RoundManager {
	ctx, cancel := context.WithCancel(context.Background())
	rm := &RoundManager{
		engine:      engine,
		clients:     make(map[string]*clientState),
		actionCh:    make(chan playerAction, 256),
		ctx:         ctx,
		cancel:      cancel,
		gameStarted: gameStarted,
		pauseCh:     make(chan struct{}),
		resultsDir:  "results",
	}
	rm.wg.Add(1)
	go rm.run()
	return rm
}

// SetResultsDir sets the directory where results files are saved.
func (rm *RoundManager) SetResultsDir(dir string) {
	rm.resultsDir = dir
}

// Connect registers a new WebSocket connection for a player.
// If the player already has a connection, the old one is invalidated
// (its generation is superseded). Returns the outbound event channel
// that the WS writer goroutine reads from.
func (rm *RoundManager) Connect(conn *websocket.Conn, playerID string) chan game.Event {
	rm.mu.Lock()

	// Invalidate old connection if one exists (reconnect).
	if old := rm.clients[playerID]; old != nil && !old.closed {
		old.closed = true
		// Close the old outbound channel so its writer goroutine exits.
		close(old.outbound)
		rm.gen++
	}

	gen := rm.gen

	ch := make(chan game.Event, 16)
	rm.clients[playerID] = &clientState{
		conn:       conn,
		outbound:   ch,
		generation: gen,
		closed:     false,
	}
	rm.mu.Unlock()

	// Send CONNECTED and initial OBSERVATION.
	rm.enqueue(playerID, game.Event{
		Type:    "CONNECTED",
		Message: "connected to Territory Wars",
	})
	if observation, err := rm.engine.Observation(playerID); err == nil {
		rm.enqueue(playerID, game.Event{
			Type:        "OBSERVATION",
			Turn:        observation.Turn,
			Observation: observation,
		})
	}

	return ch
}

// Disconnect marks a connection as closed. If this is still the current
// connection for the player (generation match), the channel is closed.
// Stale connections are silently ignored.
func (rm *RoundManager) Disconnect(playerID string, gen int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	cs := rm.clients[playerID]
	if cs == nil || cs.generation != gen || cs.closed {
		return
	}
	cs.closed = true
	close(cs.outbound)
	delete(rm.clients, playerID)
}

// enqueue sends an event to a player's outbound channel. If the channel
// is full, the player is disconnected.
func (rm *RoundManager) enqueue(playerID string, event game.Event) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	cs := rm.clients[playerID]
	if cs == nil || cs.closed {
		return
	}
	select {
	case cs.outbound <- event:
	default:
		// Queue full — disconnect this player.
		cs.closed = true
		close(cs.outbound)
	}
}

// run is the single event-loop goroutine. It processes actions, monitors
// turn resolution, and broadcasts events.
func (rm *RoundManager) run() {
	defer rm.wg.Done()

	// Track the last resolved turn to avoid re-broadcasting.
	lastResolvedTurn := -1

	// Monitor turn resolution.
	var turnSignal <-chan struct{}
	currentTurn, signalCh := rm.engine.TurnSignal()
	turnSignal = signalCh

	// Timeout: nil means no timeout is active.
	var timeoutCh <-chan time.Time

	// Wait for registration delay to expire before allowing the first turn.
	<-rm.gameStarted

	for {
		rm.pauseMu.Lock()
		paused := rm.paused
		rm.pauseMu.Unlock()

		if paused {
			// When paused, only listen for resume or shutdown.
			select {
			case <-rm.ctx.Done():
				rm.broadcastGameOver()
				rm.cleanupAll()
				return
			case <-rm.pauseCh:
				// Resumed — fall through to normal processing.
			}
			continue
		}

		select {
		case <-rm.ctx.Done():
			rm.broadcastGameOver()
			rm.cleanupAll()
			return

		case pa, ok := <-rm.actionCh:
			if !ok {
				return
			}

			// Check if this player's connection is still valid.
			rm.mu.Lock()
			cs := rm.clients[pa.playerID]
			if cs == nil {
				rm.mu.Unlock()
				continue
			}
			if cs.closed || cs.generation != rm.gen {
				rm.mu.Unlock()
				continue
			}
			rm.mu.Unlock()

			// Submit action to engine.
			result := rm.engine.Submit(pa.playerID, pa.msg)

			if result.RejectError != nil {
				rm.enqueue(pa.playerID, game.Event{
					Type:    "ERROR",
					Turn:    result.Turn,
					Message: result.RejectError.Error(),
				})
				continue
			}

			// Send ACTION_ACCEPTED.
			rm.enqueue(pa.playerID, game.Event{
				Type: "ACTION_ACCEPTED",
				Turn: result.Turn,
			})

			// Start timeout on first successful action this turn.
			if timeoutCh == nil {
				timer := time.NewTimer(rm.engine.Config().TurnTimeout)
				timeoutCh = timer.C
				_ = timer // timer is kept alive by timeoutCh
			}

		case <-turnSignal:
			// Turn was resolved (all players submitted).
			select {
			case <-turnSignal:
			default:
			}

			// Broadcast TURN_RESOLVED + OBSERVATION.
			resolvedTurn := currentTurn
			if resolvedTurn > lastResolvedTurn {
				rm.broadcastResolvedTurn(resolvedTurn)
				lastResolvedTurn = resolvedTurn
			}

			if rm.engine.IsGameOver() {
				rm.broadcastGameOver()
				rm.cleanupAll()
				return
			}

			// Get the next turn's signal channel.
			currentTurn, signalCh = rm.engine.TurnSignal()
			turnSignal = signalCh

			// Start timeout for the new turn.
			timeoutCh = nil
			timer := time.NewTimer(rm.engine.Config().TurnTimeout)
			timeoutCh = timer.C

		case <-timeoutCh:
			// Timeout expired — try to resolve the turn.
			resolved := rm.engine.ResolveExpiredTurn()
			if resolved {
				resolvedTurn := currentTurn
				if resolvedTurn > lastResolvedTurn {
					rm.broadcastResolvedTurn(resolvedTurn)
					lastResolvedTurn = resolvedTurn
				}

				if rm.engine.IsGameOver() {
					rm.broadcastGameOver()
					rm.cleanupAll()
					return
				}

				// Get the next turn's signal channel.
				currentTurn, signalCh = rm.engine.TurnSignal()
				turnSignal = signalCh

				// Start timeout for the new turn.
				timeoutCh = nil
				timer := time.NewTimer(rm.engine.Config().TurnTimeout)
				timeoutCh = timer.C
			} else {
				// Not resolved — re-arm the timer.
				timeoutCh = nil
				timer := time.NewTimer(rm.engine.Config().TurnTimeout)
				timeoutCh = timer.C
			}
		}
	}
}

func (rm *RoundManager) broadcastResolvedTurn(resolvedTurn int) {
	results := rm.engine.ResultsForTurn(resolvedTurn)
	players := rm.engine.Leaderboard()

	for _, player := range players {
		rm.enqueue(player.ID, game.Event{
			Type:    "TURN_RESOLVED",
			Turn:    resolvedTurn,
			Results: results,
		})
		if observation, err := rm.engine.Observation(player.ID); err == nil {
			rm.enqueue(player.ID, game.Event{
				Type:        "OBSERVATION",
				Turn:        observation.Turn,
				Observation: observation,
			})
		}
	}
}

func (rm *RoundManager) broadcastGameOver() {
	// Print results to stdout.
	rm.printResults()

	players := rm.engine.Leaderboard()
	for _, player := range players {
		rm.enqueue(player.ID, game.Event{
			Type:    "GAME_OVER",
			Message: "game has ended",
		})
	}
}

// printResults prints the leaderboard to stdout and saves to a file.
func (rm *RoundManager) printResults() {
	lb := rm.engine.Leaderboard()
	if len(lb) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("\n=== Game Results ===\n")
	sb.WriteString(fmt.Sprintf("%-4s %-20s %-8s %-8s %-10s\n", "Rank", "Name", "Score", "Alive", "Connected"))
	sb.WriteString(strings.Repeat("-", 55) + "\n")
	for i, p := range lb {
		alive := "yes"
		if p.Eliminated {
			alive = "no"
		}
		conn := "yes"
		if !p.Connected {
			conn = "no"
		}
		sb.WriteString(fmt.Sprintf("%-4d %-20s %-8d %-8s %-10s\n", i+1, p.Name, p.Score, alive, conn))
	}
	sb.WriteString("\n")
	text := sb.String()

	fmt.Print(text)

	// Save to file: results.yy.mm.dd.hhmm.txt
	ts := time.Now().Local().Format("060102.1504")
	filename := fmt.Sprintf("results.%s.txt", ts)
	path := filepath.Join(rm.resultsDir, filename)
	if err := os.MkdirAll(rm.resultsDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create results dir %s: %v\n", rm.resultsDir, err)
		return
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		fmt.Printf("Warning: failed to save results to %s: %v\n", path, err)
		return
	}
	fmt.Printf("Results saved to %s\n", path)
}

func (rm *RoundManager) cleanupAll() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for _, cs := range rm.clients {
		if !cs.closed {
			cs.closed = true
			close(cs.outbound)
		}
	}
	rm.clients = make(map[string]*clientState)
}

// Generation returns the current generation number for a player's connection.
// Used by the WS handler to invalidate stale connections on disconnect.
func (rm *RoundManager) Generation(playerID string) int64 {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	cs := rm.clients[playerID]
	if cs == nil {
		return 0
	}
	return cs.generation
}

// SubmitAction sends an action to the event loop for processing.
// The WS handler calls this instead of engine.Submit() directly.
func (rm *RoundManager) SubmitAction(playerID string, msg game.ActionMessage, outbound chan game.Event) {
	select {
	case rm.actionCh <- playerAction{
		playerID: playerID,
		msg:      msg,
		outbound: outbound,
	}:
	case <-rm.ctx.Done():
	}
}

// Shutdown gracefully stops the Round Manager.
func (rm *RoundManager) Shutdown() {
	rm.cancel()
	rm.wg.Wait()
}

// Pause stops the event loop from processing actions and advancing turns.
// Clients can still connect, but no turns will resolve until Resume is called.
func (rm *RoundManager) Pause() {
	rm.pauseMu.Lock()
	defer rm.pauseMu.Unlock()
	if !rm.paused {
		rm.paused = true
		// Re-create pauseCh so the run loop blocks on it.
		rm.pauseCh = make(chan struct{})
	}
}

// Resume unblocks the event loop so it resumes processing actions and turns.
func (rm *RoundManager) Resume() {
	rm.pauseMu.Lock()
	defer rm.pauseMu.Unlock()
	if rm.paused {
		rm.paused = false
		close(rm.pauseCh)
	}
}

// ResetGameStarted creates a new gameStarted channel so the run loop
// goes back to waiting mode (e.g. after a stop/reset).
func (rm *RoundManager) ResetGameStarted() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	// Drain the old channel if not already closed.
	select {
	case <-rm.gameStarted:
	default:
		close(rm.gameStarted)
	}
	rm.gameStarted = make(chan struct{})
}
