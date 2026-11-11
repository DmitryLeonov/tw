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
	close(gameStarted) // immediate start for non-config mode
	rm := NewRoundManager(e, gameStarted)
	bm := NewBotManager(e, rm, 0)
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
func NewWithConfig(e *game.Engine, cfg *GameConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	turnTimeout := time.Duration(intEnvDefault("TURN_TIMEOUT_SECONDS", 15)) * time.Second
	if tt := cfg.TurnTimeout; tt != "" {
		if d, err := time.ParseDuration(tt); err == nil {
			turnTimeout = d
		}
	}
	turnDelay := time.Duration(intEnvDefault("TURN_DELAY_SECONDS", 5)) * time.Second
	if td := cfg.Bots.TurnDelay; td != "" {
		if d, err := time.ParseDuration(td); err == nil {
			turnDelay = d
		}
	}

	regDelay := time.Duration(intEnvDefault("REGISTRATION_DELAY_SECONDS", 60)) * time.Second
	if rd := cfg.RegistrationDelay; rd != "" {
		if d, err := time.ParseDuration(rd); err == nil {
			regDelay = d
		}
	}

	gameStarted := make(chan struct{})
	rm := NewRoundManager(e, gameStarted)
	resultsDir := "results"
	if rd := cfg.ResultsDir; rd != "" {
		resultsDir = rd
	}
	rm.SetResultsDir(resultsDir)
	bm := NewBotManager(e, rm, turnDelay)
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

	// Start viewer monitoring.
	v.StartMonitoring(ctx)

	// Register and start bots if enabled.
	if cfg.Bots.Enabled {
		agents, err := BuildBotList(cfg.Bots, cfg.MaxPlayers)
		if err != nil {
			log.Printf("Warning: failed to build bot list: %v", err)
		} else if len(agents) > 0 {
			// Register all bots with the engine first.
			type botReg struct {
				agent PlayerAgent
				pid   string
			}
			var registered []botReg
			for _, agent := range agents {
				player, err := e.Join(agent.Name())
				if err != nil {
					log.Printf("Warning: failed to register bot %s: %v", agent.Name(), err)
					continue
				}
				outbound := rm.Connect(nil, player.ID)
				// Drain outbound events — bots don't need them.
				go func(ch chan game.Event) {
					for range ch {
					}
				}(outbound)
				registered = append(registered, botReg{agent, player.ID})
			}
			// Then start bot decision loops.
			for _, br := range registered {
				go func(agent PlayerAgent, pid string) {
					bp := newBotPlayer(agent, pid, rm, e, turnTimeout, bm.logger, turnDelay)
					bp.Run(ctx)
				}(br.agent, br.pid)
			}
			// Register bots with BotManager so viewer can identify them.
			for _, br := range registered {
				bm.RegisterBot(br.pid)
			}
		}
	}

	// Start registration delay timer — game begins after delay or manual start.
	go func() {
		time.Sleep(regDelay)
		log.Printf("Registration delay expired — starting game")
		select {
		case <-s.gameStarted:
			// already started
		default:
			close(s.gameStarted)
		}
	}()

	return s
}

// Shutdown gracefully stops the Round Manager and all components.
func (s *Server) Shutdown() {
	s.shutdownCancel()
	s.botManager.Shutdown()
	s.rounds.Shutdown()
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
func (s *Server) ResetGameStarted() {
	s.engine.Reset()
	s.rounds.ResetGameStarted()
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
func (s *Server) StartGame() bool {
	select {
	case <-s.gameStarted:
		return false // already started
	default:
		close(s.gameStarted)
		return true
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

	// Connect registers the connection with the Round Manager and returns
	// the outbound event channel. It also assigns a generation number.
	outbound := s.rounds.Connect(conn, p.ID)
	gen := s.rounds.Generation(p.ID)

	defer func() {
		s.rounds.Disconnect(p.ID, gen)
		s.engine.SetConnected(p.ID, false)
		_ = conn.Close()
	}()

	// Writer goroutine: reads events from the Round Manager and writes them
	// to the WebSocket connection. One writer per connection.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		pingTicker := time.NewTicker(25 * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case event, open := <-outbound:
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if !open {
					_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteJSON(event); err != nil {
					return
				}
			case <-pingTicker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Reader goroutine: reads actions from the WebSocket and sends them
	// to the Round Manager's event loop. One reader per connection.
	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		var msg game.ActionMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		// Send action to the Round Manager's event loop for processing.
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
