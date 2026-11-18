package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	// bot marks players that have no WebSocket connection (built-in bots).
	// Events are not enqueued for them because nobody would drain the queue.
	bot bool
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

	actionCh    chan playerAction // actions from WS handlers
	forceTurnCh chan struct{}     // forces select to check turnSignal after immediate resolve

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	gameStarted chan struct{} // closed when registration delay expires or game is manually started
	ready       chan struct{} // closed when run() enters the main loop (game is actually processing)

	paused  bool
	pauseCh chan struct{}
	pauseMu sync.Mutex

	resultsDir      string // directory for saving results files
	gameOverHandler func() // called when game ends (max turns) — triggers full server stop
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
		forceTurnCh: make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
		gameStarted: gameStarted,
		ready:       make(chan struct{}),
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

// SetGameOverHandler sets the callback called when the game ends naturally.
func (rm *RoundManager) SetGameOverHandler(h func()) {
	rm.gameOverHandler = h
}

// Connect registers a new WebSocket connection for a player.
// If the player already has a connection, the old one is invalidated
// (its generation is superseded). Returns the outbound event channel
// that the WS writer goroutine reads from.
func (rm *RoundManager) Connect(conn *websocket.Conn, playerID string) chan game.Event {
	ch := rm.register(playerID, conn, false)

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

// ConnectBot registers a built-in bot player that has no WebSocket connection.
// Bots submit actions through their per-player action channel and never read
// events, so nothing is enqueued for them.
func (rm *RoundManager) ConnectBot(playerID string) {
	rm.register(playerID, nil, true)
}

// register performs the shared connection bookkeeping: it invalidates a
// previous connection for the same player and installs the new client state.
func (rm *RoundManager) register(playerID string, conn *websocket.Conn, bot bool) chan game.Event {
	rm.mu.Lock()

	// Invalidate old connection if one exists (reconnect).
	if old := rm.clients[playerID]; old != nil && !old.closed {
		old.closed = true
		// Close the old outbound channel so its writer goroutine exits.
		close(old.outbound)
		rm.gen++
	}

	gen := rm.gen

	ch := make(chan game.Event, 256)
	rm.clients[playerID] = &clientState{
		conn:       conn,
		outbound:   ch,
		generation: gen,
		closed:     false,
		bot:        bot,
	}
	rm.mu.Unlock()

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
	// Do NOT delete from rm.clients — the run() loop checks
	// rm.clients[pa.playerID] to validate actions. If the entry
	// is removed, subsequent actions from this playerID are silently
	// dropped. The entry stays so reconnect via Connect() works
	// and the run() loop can still see the player.
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
	// Bots never read their outbound queue — skip enqueueing events for them.
	if cs.bot {
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

	// Heartbeat ticker — broadcast STILL_ALIVE every 3s so clients
	// know the server is alive even when no turns are resolving.
	// This must be created BEFORE the gameStarted wait so idle clients
	// don't time out while waiting for the game to start.
	heartbeat := time.NewTicker(3 * time.Second)
	defer heartbeat.Stop()

	// Wait for registration delay to expire before allowing the first turn.
	// Also listen for ctx cancellation so Stop() can unblock this.
	// Include heartbeat so idle clients don't time out while waiting.
	for {
		select {
		case <-rm.gameStarted:
			goto mainLoop
		case <-rm.ctx.Done():
			return
		case <-heartbeat.C:
			// Broadcast STILL_ALIVE to all connected clients (waiting for game start).
			players := rm.engine.Leaderboard()
			for _, p := range players {
				rm.enqueue(p.ID, game.Event{
					Type:    "STILL_ALIVE",
					Message: "server alive",
				})
			}
		}
	}
mainLoop:

	// Signal that the main loop has started — game is actually processing now.
	close(rm.ready)

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

		case <-rm.forceTurnCh:
			// Drain any remaining signals on this channel.
			select {
			case <-rm.forceTurnCh:
			default:
			}

			// Get the next turn's signal channel.
			currentTurn, signalCh = rm.engine.TurnSignal()
			turnSignal = signalCh

			// Start timeout for the new turn.
			timeoutCh = nil
			timer := time.NewTimer(rm.engine.Config().TurnDelay)
			timeoutCh = timer.C

		case pa, ok := <-rm.actionCh:
			if !ok {
				return
			}

			// Check if this player's connection is still valid.
			rm.mu.Lock()
			cs := rm.clients[pa.playerID]
			if cs == nil || cs.closed {
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

			// Check if turn was resolved immediately (all players submitted).
			// If so, handle it inline instead of relying on forceTurnCh.
			engineTurn, _ := rm.engine.TurnSignal()
			if engineTurn > currentTurn {
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
				timer := time.NewTimer(rm.engine.Config().TurnDelay)
				timeoutCh = timer.C
			}

			// Start timeout on first successful action this turn.
			if timeoutCh == nil {
				timer := time.NewTimer(rm.engine.Config().TurnDelay)
				timeoutCh = timer.C
				_ = timer // timer is kept alive by timeoutCh
			}

		case <-rm.forceTurnCh:
			// Turn was resolved immediately — handle it now.
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
			timer := time.NewTimer(rm.engine.Config().TurnDelay)
			timeoutCh = timer.C

		case _, ok := <-turnSignal:
			if !ok {
				// Channel closed — get a fresh one and retry.
				currentTurn, signalCh = rm.engine.TurnSignal()
				turnSignal = signalCh
				continue
			}

			// Turn was resolved (all players submitted).
			// Drain any remaining signals on this channel.
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
			timer := time.NewTimer(rm.engine.Config().TurnDelay)
			timeoutCh = timer.C

		case <-timeoutCh:
			// Timeout expired — resolve the turn.
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
				timer := time.NewTimer(rm.engine.Config().TurnDelay)
				timeoutCh = timer.C
			} else {
				// Not resolved — re-arm the timer.
				timeoutCh = nil
				timer := time.NewTimer(rm.engine.Config().TurnDelay)
				timeoutCh = timer.C
			}

		case <-heartbeat.C:
			// Broadcast STILL_ALIVE to all connected clients (during game).
			players := rm.engine.Leaderboard()
			for _, p := range players {
				rm.enqueue(p.ID, game.Event{
					Type:    "STILL_ALIVE",
					Message: "server alive",
				})
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

	// Call the game over handler (e.g., prompt for Enter to stop).
	if rm.gameOverHandler != nil {
		rm.gameOverHandler()
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
	sb.WriteString(fmt.Sprintf("%-4s %-20s %-8s %-8s %-10s\n", "Rank", "Name", "Territory", "Score", "Alive"))
	sb.WriteString(strings.Repeat("-", 55) + "\n")
	for i, p := range lb {
		alive := "yes"
		if p.Eliminated {
			alive = "no"
		}
		sb.WriteString(fmt.Sprintf("%-4d %-20s %-8d %-8d %-10s\n", i+1, p.Name, p.Territory, p.Score, alive))
	}

	// Second table sorted by name.
	sb.WriteString("\n=== Results by Name ===\n")
	sb.WriteString(fmt.Sprintf("%-20s %-8s %-8s %-8s %-10s\n", "Name", "Territory", "Score", "Mark", "Alive"))
	sb.WriteString(strings.Repeat("-", 55) + "\n")
	names := make([]game.PublicPlayer, len(lb))
	copy(names, lb)
	sort.Slice(names, func(i, j int) bool { return names[i].Name < names[j].Name })
	for _, p := range names {
		alive := "yes"
		if p.Eliminated {
			alive = "no"
		}
		mark := p.Territory
		if rm.engine.Config().MarkRate > 0 {
			mark = (p.Territory + rm.engine.Config().MarkRate/2) / rm.engine.Config().MarkRate
		}
		if mark > rm.engine.Config().MarkThreshold {
			mark = rm.engine.Config().MarkThreshold
		}
		if mark < 0 {
			mark = 0
		}
		sb.WriteString(fmt.Sprintf("%-20s %-8d %-8d %-8d %-10s\n", p.Name, p.Territory, p.Score, mark, alive))
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

	// Close all WS connections. No GAME_OVER needed — clients get
	// "unauthorized" on reconnect because tokens were invalidated by Stop().
	for _, cs := range rm.clients {
		if !cs.closed {
			cs.closed = true
			close(cs.outbound)
			if cs.conn != nil {
				cs.conn.Close()
			}
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

// Stop halts the event loop and waits for it to exit.
// Used by the "stop" command to fully halt turn processing.
func (rm *RoundManager) Stop() {
	rm.cancel()
	rm.wg.Wait()
}

// Restart creates a fresh context and restarts the event loop.
func (rm *RoundManager) Restart() {
	ctx, cancel := context.WithCancel(context.Background())
	rm.ctx = ctx
	rm.cancel = cancel
	rm.wg.Add(1)
	go rm.run()
}

// Shutdown gracefully stops the Round Manager.
func (rm *RoundManager) Shutdown() {
	if rm.cancel != nil {
		rm.cancel()
	}
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
	// Recreate ready channel — the old one is already closed.
	rm.ready = make(chan struct{})
}

// SetGameStarted replaces the gameStarted channel.
// Used to keep Server.gameStarted and RoundManager.gameStarted in sync.
func (rm *RoundManager) SetGameStarted(ch chan struct{}) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	select {
	case <-rm.gameStarted:
	default:
		close(rm.gameStarted)
	}
	rm.gameStarted = ch
}

// Ready returns the channel that is closed when the run() loop enters the
// main processing loop. Used by StartGame() to wait until the game is
// actually processing before returning success.
func (rm *RoundManager) Ready() <-chan struct{} {
	return rm.ready
}

// Reset stops the event loop and restarts it with a fresh context.
// Used by the "stop" command to fully halt turn processing.
func (rm *RoundManager) Reset() {
	// Cancel current context — run() exits via ctx.Done().
	rm.cancel()
	rm.wg.Wait()

	// Recreate ready channel — the old one is already closed.
	rm.ready = make(chan struct{})

	// Create a fresh context and restart the run loop.
	ctx, cancel := context.WithCancel(context.Background())
	rm.ctx = ctx
	rm.cancel = cancel
	rm.wg.Add(1)
	go rm.run()
}
