package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"twserver/internal/game"
)

//go:embed viewer.html
var viewerFS embed.FS

// viewerClient represents a connected SSE viewer.
type viewerClient struct {
	events        chan viewerEvent
	done          chan struct{}
	headerWritten bool
}

// viewerEvent is an event sent to a viewer client.
type viewerEvent struct {
	Type    string
	Payload interface{}
}

// Viewer provides HTTP endpoints for the game visualization interface.
// It broadcasts game state updates to connected SSE clients.
type Viewer struct {
	mu         sync.RWMutex
	clients    map[*viewerClient]bool
	engine     *game.Engine
	botManager *BotManager
	history    []BotLogEntry
	lastTurn   int
}

// NewViewer creates a new Viewer.
func NewViewer(engine *game.Engine, botManager *BotManager) *Viewer {
	return &Viewer{
		clients:    make(map[*viewerClient]bool),
		engine:     engine,
		botManager: botManager,
	}
}

// StartMonitoring begins watching for turn resolution events and broadcasting
// state updates to all connected SSE clients. Also sends periodic state
// broadcasts (every 5s) so the viewer stays current during idle periods
// (e.g. while clients are connecting before game start).
func (v *Viewer) StartMonitoring(ctx context.Context) {
	if v == nil || ctx == nil {
		return
	}
	go func() {
		stateTicker := time.NewTicker(5 * time.Second)
		defer stateTicker.Stop()
		for {
			_, signalCh := v.engine.TurnSignal()
			select {
			case <-signalCh:
				v.broadcastState()
				v.broadcastHistory()
			case <-stateTicker.C:
				v.broadcastState()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// broadcastState sends the current game state to all connected viewers.
func (v *Viewer) broadcastState() {
	v.mu.RLock()
	clients := make([]*viewerClient, 0, len(v.clients))
	for c := range v.clients {
		clients = append(clients, c)
	}
	v.mu.RUnlock()

	state := v.buildViewerState()
	data, _ := json.Marshal(state)

	for _, c := range clients {
		select {
		case c.events <- viewerEvent{Type: "state", Payload: string(data)}:
		case <-c.done:
			// Client disconnected; skip.
		default:
			// Drop slow clients.
		}
	}
}

// broadcastHistory sends the current bot action history.
func (v *Viewer) broadcastHistory() {
	v.mu.RLock()
	clients := make([]*viewerClient, 0, len(v.clients))
	for c := range v.clients {
		clients = append(clients, c)
	}
	v.mu.RUnlock()

	entries := v.botManager.LogEntries()
	data, _ := json.Marshal(entries)

	for _, c := range clients {
		select {
		case c.events <- viewerEvent{Type: "history", Payload: string(data)}:
		case <-c.done:
			// Client disconnected; skip.
		default:
		}
	}
}

// buildViewerState constructs a viewer-friendly DTO from the engine state.
func (v *Viewer) buildViewerState() map[string]interface{} {
	status := v.engine.Status()
	leaderboard := v.engine.Leaderboard()

	// Build minimal state when no players exist.
	if len(leaderboard) == 0 {
		return map[string]interface{}{
			"match_id":  status["match_id"],
			"turn":      status["turn"],
			"max_turns": status["max_turns"],
			"game_over": status["game_over"],
			"width":     40,
			"height":    40,
			"players":   []map[string]interface{}{},
			"cells":     []map[string]interface{}{},
		}
	}

	obs, _ := v.engine.Observation(leaderboard[0].ID)

	botIDs := v.botManager.BotIDs()

	// Build player list with bot info.
	players := make([]map[string]interface{}, 0, len(leaderboard))
	botOnly := make([]map[string]interface{}, 0)
	for _, p := range leaderboard {
		entry := map[string]interface{}{
			"id":         p.ID,
			"name":       p.Name,
			"x":          p.X,
			"y":          p.Y,
			"score":      p.Score,
			"energy":     p.Energy,
			"territory":  p.Territory,
			"connected":  p.Connected,
			"eliminated": p.Eliminated,
			"is_bot":     botIDs[p.ID],
		}
		players = append(players, entry)
		if botIDs[p.ID] {
			botOnly = append(botOnly, entry)
		}
	}

	// Build cells list.
	cells := make([]map[string]interface{}, 0, len(obs.Cells))
	for _, c := range obs.Cells {
		cells = append(cells, map[string]interface{}{
			"x":      c.X,
			"y":      c.Y,
			"owner":  c.Owner,
			"type":   c.Type,
			"health": c.Health,
		})
	}

	return map[string]interface{}{
		"match_id":    status["match_id"],
		"turn":        status["turn"],
		"max_turns":   status["max_turns"],
		"game_over":   status["game_over"],
		"width":       obs.Width,
		"height":      obs.Height,
		"players":     players,
		"bot_players": botOnly,
		"cells":       cells,
		"history":     v.botManager.LogEntries(),
	}
}

// ServeIndex serves the viewer HTML page.
func (v *Viewer) ServeIndex(w http.ResponseWriter, r *http.Request) {
	file, err := viewerFS.Open("viewer.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_, _ = w.Write(data)
}

// ServeViewer serves the viewer page at /viewer.
func (v *Viewer) ServeViewer(w http.ResponseWriter, r *http.Request) {
	v.ServeIndex(w, r)
}

// ServeState serves the current game state as JSON.
func (v *Viewer) ServeState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := v.buildViewerState()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

// ServeEvents serves Server-Sent Events for real-time updates.
func (v *Viewer) ServeEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := &viewerClient{
		events: make(chan viewerEvent, 32),
		done:   make(chan struct{}),
	}

	v.mu.Lock()
	v.clients[client] = true
	v.mu.Unlock()

	// Send initial state.
	state := v.buildViewerState()
	data, _ := json.Marshal(state)
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", string(data))
	flusher.Flush()

	// Send initial history.
	entries := v.botManager.LogEntries()
	hdata, _ := json.Marshal(entries)
	fmt.Fprintf(w, "event: history\ndata: %s\n\n", string(hdata))
	flusher.Flush()

	// Keep connection open.
	done := r.Context().Done()
	for {
		select {
		case ev := <-client.events:
			switch ev.Type {
			case "state":
				fmt.Fprintf(w, "event: state\ndata: %s\n\n", ev.Payload)
			case "history":
				fmt.Fprintf(w, "event: history\ndata: %s\n\n", ev.Payload)
			}
			flusher.Flush()
		case <-done:
			v.mu.Lock()
			delete(v.clients, client)
			v.mu.Unlock()
			close(client.done)
			return
		}
	}
}

// ServeHistory serves the bot action history as JSON.
func (v *Viewer) ServeHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries := v.botManager.LogEntries()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

// ServeStatic serves embedded static files.
func (v *Viewer) ServeStatic(w http.ResponseWriter, r *http.Request) {
	// Serve the viewer HTML for any path.
	file, err := viewerFS.Open("viewer.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// ServeViewerAssets serves the viewer HTML as a template with data injection.
func (v *Viewer) ServeViewerAssets(w http.ResponseWriter, r *http.Request) {
	v.ServeIndex(w, r)
}

// ServeViewerAPI serves viewer API endpoints.
func (v *Viewer) ServeViewerAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/v1/viewer/state" || path == "/api/v1/viewer/state/":
		v.ServeState(w, r)
	case path == "/api/v1/viewer/events" || path == "/api/v1/viewer/events/":
		v.ServeEvents(w, r)
	case path == "/api/v1/viewer/history" || path == "/api/v1/viewer/history/":
		v.ServeHistory(w, r)
	default:
		http.NotFound(w, r)
	}
}

// ServeViewerPage serves the viewer HTML page.
func (v *Viewer) ServeViewerPage(w http.ResponseWriter, r *http.Request) {
	v.ServeIndex(w, r)
}

// ServeViewerRoot serves the viewer at /.
func (v *Viewer) ServeViewerRoot(w http.ResponseWriter, r *http.Request) {
	v.ServeIndex(w, r)
}

// ServeViewerHandler is a unified handler for viewer routes.
func (v *Viewer) ServeViewerHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Exact route matches.
	switch path {
	case "/", "/viewer":
		v.ServeViewerPage(w, r)
	case "/api/v1/viewer/state":
		v.ServeState(w, r)
	case "/api/v1/viewer/events":
		v.ServeEvents(w, r)
	case "/api/v1/viewer/history":
		v.ServeHistory(w, r)
	default:
		// Try to serve as static file.
		v.ServeStatic(w, r)
	}
}
