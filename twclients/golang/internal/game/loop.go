package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"twclient/internal/api"
	"twclient/internal/models"
	"twclient/internal/strategy"
)

// Loop orchestrates the game loop: connecting via WebSocket, receiving
// observations via HTTP polling, delegating decisions to a Strategy, and
// submitting actions via WebSocket.
type Loop struct {
	api        *api.Client
	strategy   strategy.Strategy
	logger     *slog.Logger
	maxRetries int
}

// New creates a new Loop.
func New(apiClient *api.Client, strat strategy.Strategy) *Loop {
	return &Loop{
		api:        apiClient,
		strategy:   strat,
		logger:     slog.Default(),
		maxRetries: 10,
	}
}

// NewWithLogger creates a Loop with a custom logger and max retries.
func NewWithLogger(apiClient *api.Client, strat strategy.Strategy, logger *slog.Logger, maxRetries int) *Loop {
	l := New(apiClient, strat)
	l.logger = logger
	l.maxRetries = maxRetries
	return l
}

// Run executes the game loop until the game ends or the context is cancelled.
func (l *Loop) Run(ctx context.Context) error {
	l.logger.Info("starting game loop")

	// Get initial observation.
	obs, err := l.api.GetObservation(ctx)
	if err != nil {
		return fmt.Errorf("failed to get initial observation: %w", err)
	}

	l.logger.Info("initial observation received", "turn", obs.Turn, "score", obs.Score)

	// Main game loop with reconnect.
	maxReconnects := l.maxRetries
	reconnects := 0
	for !obs.GameOver {
		select {
		case <-ctx.Done():
			l.logger.Info("context cancelled, stopping game loop")
			return ctx.Err()
		default:
		}

		// Connect (or reconnect) via WebSocket.
		conn, err := l.connectOrReconnect(ctx)
		if err != nil {
			l.logger.Error("failed to connect WebSocket", "error", err)
			reconnects++
			if reconnects >= maxReconnects {
				l.logger.Info("max reconnects reached (%d), stopping", maxReconnects)
				return fmt.Errorf("max reconnects reached")
			}
			time.Sleep(2 * time.Second)
			continue
		}
		reconnects = 0

		// Run the game loop on this connection until it breaks.
		obs, err = l.runOnConnection(ctx, conn, obs)
		if err != nil {
			l.logger.Warn("connection lost, will reconnect", "error", err)

			// Before reconnecting, check HTTP state — if game_over, exit.
			newObs, checkErr := l.api.GetObservation(ctx)
			if checkErr == nil && newObs.GameOver {
				l.logger.Info("game over detected via HTTP state")
				obs = newObs
				break
			}

			reconnects++
			if reconnects >= maxReconnects {
				l.logger.Info("max reconnects reached (%d), stopping", maxReconnects)
				return fmt.Errorf("max reconnects reached")
			}
			// Continue to reconnect on next iteration.
			continue
		}
		// err == nil means game over.
		break
	}

	// Game over — print final state.
	l.logger.Info("game over", "final_turn", obs.Turn, "final_score", obs.Score)
	l.printFinalState(obs)

	return nil
}

// connectOrReconnect establishes or re-establishes a WebSocket connection.
// If the connection fails (e.g., token expired after game reset), re-joins
// the game with a fresh token before retrying.
func (l *Loop) connectOrReconnect(ctx context.Context) (*websocket.Conn, error) {
	l.logger.Info("connecting to WebSocket")
	conn, err := l.api.ConnectWebSocket(ctx)
	if err != nil {
		// Before re-joining, check HTTP state — if game_over, exit.
		newObs, checkErr := l.api.GetObservation(ctx)
		if checkErr == nil && newObs.GameOver {
			l.logger.Info("game over detected via HTTP state during reconnect")
			return nil, fmt.Errorf("game over: %w", err)
		}

		// Try re-joining with a fresh token (happens after game reset).
		l.logger.Info("re-joining game with fresh token")
		joinClient := api.New(l.api.BaseURL(), "")
		joinResp, joinErr := joinClient.Join(ctx, l.api.PlayerName())
		if joinErr != nil {
			// If registration is closed (game already started), the game
			// cannot continue — exit gracefully.
			errStr := joinErr.Error()
			if strings.Contains(errStr, "registration is closed") ||
				strings.Contains(errStr, "game stopped") ||
				strings.Contains(errStr, "match is full") {
				l.logger.Info("registration closed — game cannot continue, exiting")
				return nil, fmt.Errorf("registration closed: %w", joinErr)
			}
			return nil, fmt.Errorf("WebSocket dial failed: %w (re-join also failed: %v)", err, joinErr)
		}
		l.api.UpdateToken(joinResp.Token)
		l.api.UpdateWSURL(joinResp.WebsocketURL)
		l.logger.Info("re-joined game", "player_id", joinResp.PlayerID)

		conn, err = l.api.ConnectWebSocket(ctx)
		if err != nil {
			return nil, fmt.Errorf("WebSocket dial failed after re-join: %w", err)
		}
	}
	l.logger.Info("WebSocket connected")
	return conn, nil
}

// runOnConnection runs the game loop on a single WebSocket connection.
// Uses HTTP polling for state updates and WebSocket only for action submission.
// Returns an error when the connection breaks, or nil when the game ends.
func (l *Loop) runOnConnection(ctx context.Context, conn *websocket.Conn, obs *models.Observation) (*models.Observation, error) {
	defer conn.Close()

	// Read loop: process incoming WebSocket messages (ping/pong, events).
	// Without this, server pings are never answered and the connection is
	// closed after the read deadline expires.
	done := make(chan struct{})
	gameOverReceived := false
	go func() {
		defer close(done)
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Respond to pings with pongs.
			if msgType == websocket.PingMessage {
				_ = conn.WriteMessage(websocket.PongMessage, data)
			}
			// Check for GAME_OVER event.
			var evt models.Event
			if err := json.Unmarshal(data, &evt); err == nil && evt.Type == "GAME_OVER" {
				gameOverReceived = true
				return
			}
		}
	}()

	// Main action loop.
	for !obs.GameOver {
		select {
		case <-ctx.Done():
			return obs, ctx.Err()
		default:
		}

		// Always fetch the latest observation before acting.
		// This ensures we submit at the current turn even if we reconnected
		// and the turn has advanced while we were disconnected.
		newObs, err := l.api.GetObservation(ctx)
		if err != nil {
			l.logger.Error("failed to get observation", "error", err)
			return obs, fmt.Errorf("failed to get observation: %w", err)
		}
		obs = newObs

		if obs.GameOver {
			break
		}

		// Wait for game to start (turn > 0) before sending actions.
		if obs.Turn == 0 {
			l.logger.Info("waiting for game to start (turn 0)...")
			time.Sleep(1 * time.Second)
			continue
		}

		// Decide action.
		action := l.strategy.Decide(obs)

		// Build action message.
		msg := api.BuildActionMessage(obs.Turn, action)

		// Submit action via WebSocket.
		if err := l.api.SubmitActionWebSocket(conn, msg); err != nil {
			l.logger.Error("failed to submit action", "error", err, "turn", obs.Turn)
			// Connection is likely dead — exit to reconnect.
			return obs, fmt.Errorf("WebSocket write failed: %w", err)
		}

		l.logger.Info("action submitted", "turn", obs.Turn, "action", msg.Actions[0].Type)

		// Wait for turn resolution by polling observations via HTTP.
		currentTurn := obs.Turn
		pollCtx, pollCancel := context.WithTimeout(ctx, 30*time.Second)

		for obs.Turn == currentTurn && !obs.GameOver {
			select {
			case <-pollCtx.Done():
				l.logger.Warn("turn resolution timed out")
				pollCancel()
				return obs, fmt.Errorf("turn resolution timed out at turn %d", currentTurn)
			case <-done:
				// Read loop exited — check if it was GAME_OVER or connection error.
				pollCancel()
				if gameOverReceived {
					l.logger.Info("GAME_OVER received via WebSocket")
					return obs, nil
				}
				return obs, fmt.Errorf("WebSocket read loop exited")
			default:
			}

			time.Sleep(500 * time.Millisecond)
			newObs, err := l.api.GetObservation(ctx)
			if err != nil {
				l.logger.Error("failed to get observation during polling", "error", err)
				time.Sleep(1 * time.Second)
				continue
			}
			obs = newObs

			if obs.Turn != currentTurn {
				l.logger.Info("turn resolved", "previous_turn", currentTurn, "new_turn", obs.Turn, "score", obs.Score)
			}
		}
		pollCancel()
	}

	<-done
	return obs, nil
}

// printFinalState prints the final game state including leaderboard.
func (l *Loop) printFinalState(obs *models.Observation) {
	fmt.Println("\n========== GAME OVER ==========")
	fmt.Printf("Final Turn: %d\n", obs.Turn)
	fmt.Printf("Your Score: %d\n", obs.Score)
	fmt.Printf("Energy: %d\n", obs.Energy)
	fmt.Printf("Territory: %d cells\n", countOwnedCells(obs))

	// Print leaderboard if available.
	l.logger.Info("fetching final leaderboard")
	leaderboard, err := l.api.GetLeaderboard(context.Background())
	if err != nil {
		l.logger.Error("failed to get final leaderboard", "error", err)
	} else {
		if lb, ok := leaderboard["leaderboard"].([]interface{}); ok {
			fmt.Println("\n--- Final Leaderboard ---")
			for i, entry := range lb {
				if m, ok := entry.(map[string]interface{}); ok {
					fmt.Printf("%d. %s: %d points\n", i+1, m["player_id"], int(m["score"].(float64)))
				}
			}
		}
	}
	fmt.Println("==============================")
}

// countOwnedCells counts how many cells the player owns.
func countOwnedCells(obs *models.Observation) int {
	count := 0
	for _, cell := range obs.Cells {
		if cell.Owner == obs.PlayerID {
			count++
		}
	}
	return count
}
