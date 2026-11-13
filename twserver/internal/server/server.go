package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"twserver/internal/game"
)

type Server struct {
	engine         *game.Engine
	rounds         *RoundManager
	botManager     *BotManager
	viewer         *Viewer
	config         *GameConfig
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	gameStarted    chan struct{}
}

func New(e *game.Engine) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	gameStarted := make(chan struct{})
	rm := NewRoundManager(e, gameStarted)
	bm := NewBotManager(e, rm, nil)
	v := NewViewer(e, bm)

	return &Server{
		engine:         e,
		rounds:         rm,
		botManager:     bm,
		viewer:         v,
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
		gameStarted:    gameStarted,
	}
}

// NewWithConfig creates a Server with full configuration, including bot setup.
// Bots are NOT started here — they are started on demand via Start().
func NewWithConfig(e *game.Engine, cfg *GameConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	gameStarted := make(chan struct{})
	rm := NewRoundManager(e, gameStarted)
	resultsDir := "results"
	if rd := cfg.ResultsDir; rd != "" {
		resultsDir = rd
	}
	rm.SetResultsDir(resultsDir)

	// Pre-build bot agents from config (but don't start them yet).
	var agents []PlayerAgent
	if cfg.BotsEnabled {
		agents, _ = BuildBotList(cfg.BotsEnabled, cfg.MaxPlayers)
	}

	bm := NewBotManager(e, rm, agents)
	v := NewViewer(e, bm)

	s := &Server{
		engine:         e,
		rounds:         rm,
		botManager:     bm,
		viewer:         v,
		config:         cfg,
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
		gameStarted:    gameStarted,
	}

	v.StartMonitoring(ctx)

	return s
}

// Shutdown gracefully stops the Round Manager and all components.
func (s *Server) Shutdown() {
	s.shutdownCancel()
	s.botManager.Shutdown()
	s.rounds.Shutdown()
}

// Stop stops the game — invalidates all tokens, disconnects clients, resets state.
// Clients reconnecting with old tokens get "unauthorized"; after max_retries they exit.
func (s *Server) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("Game stopped via /api/v1/stop — invalidating tokens and disconnecting clients")
	s.StopGame()
	writeJSON(w, http.StatusOK, map[string]string{"status": "game stopped"})
}

// StopGame performs the full game stop sequence: invalidates tokens, disconnects
// clients, resets engine state, shuts down bots, and restarts the round manager.
// Used by both the API handler and the console command.
func (s *Server) StopGame() {
	// 0. Invalidate all tokens so reconnecting clients get "unauthorized".
	s.engine.ClearTokens()

	// 1. Close all WS connections (no GAME_OVER needed — clients exit on reconnect auth failure).
	s.rounds.cleanupAll()

	// 2. Cancel shutdown context to stop Viewer monitoring goroutine.
	s.shutdownCancel()

	// 3. Stop the RoundManager event loop.
	s.rounds.Stop()

	// 4. Reset engine state.
	s.engine.Reset()
	s.engine.ResetStopped()
	s.engine.ClearPlayers()

	// 5. Create a fresh gameStarted channel.
	s.ResetGameStarted()

	// 6. Shutdown old bots.
	s.botManager.Shutdown()
	s.botManager.ResetContext()

	// 7. Restart the RoundManager event loop.
	s.rounds.Restart()

	// 8. Recreate shutdown context and restart Viewer monitoring.
	ctx, cancel := context.WithCancel(context.Background())
	s.shutdownCtx = ctx
	s.shutdownCancel = cancel
	s.viewer.StartMonitoring(ctx)
}

// PauseHTTP handles POST /api/v1/pause — stops the event loop from processing actions.
func (s *Server) PauseHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	s.Pause()
	writeJSON(w, http.StatusOK, map[string]string{"status": "game paused"})
}

// ResumeHTTP handles POST /api/v1/resume — unblocks the event loop.
func (s *Server) ResumeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	s.Resume()
	writeJSON(w, http.StatusOK, map[string]string{"status": "game resumed"})
}

// Pause stops the event loop from processing actions and advancing turns.
func (s *Server) Pause() {
	s.rounds.Pause()
}

// Resume unblocks the event loop to resume processing.
func (s *Server) Resume() {
	s.rounds.Resume()
}

// ResetGameStarted resets the engine and goes back to waiting mode.
// Creates a fresh gameStarted channel shared between Server and RoundManager
// so that StartGame() can close it again after a stop/reset.
func (s *Server) ResetGameStarted() {
	s.engine.Reset()
	ch := make(chan struct{})
	s.rounds.SetGameStarted(ch)
	s.gameStarted = ch
}

// GetViewer returns the Viewer instance, or nil if not configured.
func (s *Server) GetViewer() *Viewer {
	return s.viewer
}

// GetBotManager returns the BotManager instance, or nil if not configured.
func (s *Server) GetBotManager() *BotManager {
	return s.botManager
}

// StartGame manually starts the game (unblocks the registration delay).
// Returns true if the game was not already started.
// Also spawns bots to fill remaining player slots.
func (s *Server) StartGame() bool {
	select {
	case <-s.gameStarted:
		s.engine.ResetStopped()
		return false
	default:
		close(s.gameStarted)
		s.engine.ResetStopped()

		// Spawn bots to fill remaining player slots.
		if s.config == nil {
			// No config — wait for round manager to be ready, then return.
			select {
			case <-s.rounds.Ready():
				return true
			case <-time.After(5 * time.Second):
				log.Printf("Warning: round manager ready timeout")
				return true
			}
		}
		humanCount := s.botManager.HumanPlayerCount()
		botCount := s.config.MaxPlayers - humanCount
		if botCount > 0 && s.config.BotsEnabled {
			log.Printf("Starting %d bots (human players: %d, max: %d)", botCount, humanCount, s.config.MaxPlayers)
			if err := s.botManager.StartBotsOnDemand(botCount); err != nil {
				log.Printf("Warning: failed to start bots: %v", err)
			}
		}

		// Wait for round manager to enter the main loop (game is actually processing).
		select {
		case <-s.rounds.Ready():
			return true
		case <-time.After(5 * time.Second):
			log.Printf("Warning: round manager ready timeout")
			return true
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) Join(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	p, err := s.engine.Join(strings.TrimSpace(req.Name))
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"player_id": p.ID, "token": p.Token, "websocket_url": "/ws"})
}

func (s *Server) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.StartGame() {
		log.Printf("Game started manually via /api/v1/start")
		writeJSON(w, http.StatusOK, map[string]string{"status": "game started"})
	} else {
		writeJSON(w, http.StatusOK, map[string]string{"status": "game already started"})
	}
}

func (s *Server) State(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := bearer(r)
	p, ok := s.engine.Authenticate(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	o, err := s.engine.Observation(p.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) Leaderboard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": s.engine.Status(), "leaderboard": s.engine.Leaderboard(), "results": s.engine.Results()})
}

func (s *Server) Results(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"turn":        s.engine.Leaderboard()[0].Score,
		"leaderboard": s.engine.Leaderboard(),
		"results":     s.engine.Results(),
	})
}

func (s *Server) WebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = bearer(r)
	}
	p, ok := s.engine.Authenticate(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.engine.SetConnected(p.ID, true)

	outbound := s.rounds.Connect(conn, p.ID)
	gen := s.rounds.Generation(p.ID)

	defer func() {
		s.rounds.Disconnect(p.ID, gen)
		s.engine.SetConnected(p.ID, false)
		_ = conn.Close()
	}()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		pingTicker := time.NewTicker(25 * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case event, open := <-outbound:
				if !open {
					_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteJSON(event); err != nil {
					return
				}
			case <-pingTicker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	conn.SetReadLimit(64 * 1024)
	// Reset read deadline on pong (keeps connection alive during inactivity).
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	})

	for {
		var msg game.ActionMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		s.rounds.SubmitAction(p.ID, msg, outbound)
	}
}

func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func intEnvDefault(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
