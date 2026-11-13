package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"twserver/internal/game"
)

func newTestViewerServer(t *testing.T) (*Viewer, *httptest.Server, func()) {
	t.Helper()
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, nil)
	v := NewViewer(engine, bm)

	mux := http.NewServeMux()
	mux.HandleFunc("/", v.ServeViewerHandler)
	mux.HandleFunc("/viewer", v.ServeViewerHandler)
	mux.HandleFunc("/api/v1/viewer/", v.ServeViewerHandler)

	ts := httptest.NewServer(mux)
	cleanup := func() {
		ts.Close()
	}
	return v, ts, cleanup
}

// --- ServeViewer / ServeIndex ---

func TestServeViewer(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestServeRoot(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// --- ServeState ---

func TestServeState(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/api/v1/viewer/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}

	if _, ok := state["turn"]; !ok {
		t.Error("expected 'turn' in state")
	}
	if _, ok := state["players"]; !ok {
		t.Error("expected 'players' in state")
	}
	if _, ok := state["cells"]; !ok {
		t.Error("expected 'cells' in state")
	}
	if _, ok := state["width"]; !ok {
		t.Error("expected 'width' in state")
	}
	if _, ok := state["height"]; !ok {
		t.Error("expected 'height' in state")
	}
}

func TestServeState_MethodNotAllowed(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Post(ts.URL+"/api/v1/viewer/state", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// --- ServeHistory ---

func TestServeHistory(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/api/v1/viewer/history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var entries []BotLogEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	// Initially empty since no bots are running.
	if len(entries) != 0 {
		t.Errorf("expected 0 history entries, got %d", len(entries))
	}
}

func TestServeHistory_MethodNotAllowed(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Post(ts.URL+"/api/v1/viewer/history", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// --- ServeEvents (SSE) ---

func TestServeEvents(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/api/v1/viewer/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Check SSE headers.
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected no-cache, got %s", resp.Header.Get("Cache-Control"))
	}

	// Read initial state event.
	var buf [1024]byte
	n, err := resp.Body.Read(buf[:])
	if err != nil {
		t.Fatalf("read SSE: %v", err)
	}
	data := string(buf[:n])
	if len(data) == 0 {
		t.Fatal("expected SSE data")
	}
	// Should start with "event: state"
	if len(data) < 12 || data[:12] != "event: state" {
		t.Errorf("expected SSE event: state, got: %s", data[:min(50, len(data))])
	}
}

func TestServeEvents_MethodNotAllowed(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Post(ts.URL+"/api/v1/viewer/events", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// --- ServeViewerHandler routing ---

func TestServeViewerHandler_Routes(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	tests := []struct {
		path string
		code int
	}{
		{"/", http.StatusOK},
		{"/viewer", http.StatusOK},
		{"/api/v1/viewer/state", http.StatusOK},
		{"/api/v1/viewer/events", http.StatusOK},
		{"/api/v1/viewer/history", http.StatusOK},
		{"/nonexistent", http.StatusOK}, // SPA: unknown paths serve the viewer HTML
	}

	for _, tt := range tests {
		resp, err := http.Get(ts.URL + tt.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tt.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.code {
			t.Errorf("GET %s: expected %d, got %d", tt.path, tt.code, resp.StatusCode)
		}
	}
}

// --- buildViewerState ---

func TestBuildViewerState(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, nil)
	v := NewViewer(engine, bm)

	// Join a player so leaderboard is non-empty.
	_, err := engine.Join("TestPlayer")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	state := v.buildViewerState()

	if state["width"] != 20 {
		t.Errorf("expected width 20, got %v", state["width"])
	}
	if state["height"] != 20 {
		t.Errorf("expected height 20, got %v", state["height"])
	}
	if state["turn"] != 0 {
		t.Errorf("expected turn 0, got %v", state["turn"])
	}
	players, ok := state["players"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected players to be a slice")
	}
	if len(players) != 1 {
		t.Errorf("expected 1 player, got %d", len(players))
	}
}

func TestBuildViewerState_GameOver(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  2,
		MaxTurns:    2,
		TurnDelay: 100 * time.Millisecond,
		Width:       10,
		Height:      10,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, nil)
	v := NewViewer(engine, bm)

	p1, _ := engine.Join("P1")
	p2, _ := engine.Join("P2")

	// Connect both players and submit actions to trigger turn resolution.
	out1 := rm.Connect(nil, p1.ID)
	out2 := rm.Connect(nil, p2.ID)

	// Submit pass actions to resolve turns.
	rm.SubmitAction(p1.ID, game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, out1)
	rm.SubmitAction(p2.ID, game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, out2)
	time.Sleep(200 * time.Millisecond)

	rm.SubmitAction(p1.ID, game.ActionMessage{Type: "ACTION", Turn: 1, Actions: []game.Action{{Type: "pass"}}}, out1)
	rm.SubmitAction(p2.ID, game.ActionMessage{Type: "ACTION", Turn: 1, Actions: []game.Action{{Type: "pass"}}}, out2)
	time.Sleep(200 * time.Millisecond)

	state := v.buildViewerState()
	if state["game_over"] != true {
		t.Errorf("expected game_over true, got %v", state["game_over"])
	}
}

// --- Viewer with bots ---

func TestViewerWithBots(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  6,
		MaxTurns:    10,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())

	// Pre-build bot agents.
	agents, err := BuildBotList(true, 6)
	if err != nil {
		t.Fatalf("BuildBotList: %v", err)
	}
	bm := NewBotManager(engine, rm, agents)
	v := NewViewer(engine, bm)

	// Start bots.
	err = bm.StartBotsOnDemand(2)
	if err != nil {
		t.Fatalf("StartBots: %v", err)
	}

	// Give bots time to make at least one decision.
	time.Sleep(1 * time.Second)

	// Check history has entries.
	entries := bm.LogEntries()
	if len(entries) == 0 {
		t.Error("expected bot log entries after running bots")
	}

	// Check viewer state includes bot players.
	state := v.buildViewerState()
	players, ok := state["players"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected players to be a slice")
	}
	if len(players) != 2 {
		t.Errorf("expected 2 players (bots), got %d", len(players))
	}

	bm.Shutdown()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- SSE initial data contains valid JSON ---

func TestServeEvents_InitialDataValidJSON(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/api/v1/viewer/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Read only the initial burst of data (state + history events).
	// SSE is a long-lived connection, so we close the body after a short delay
	// to unblock io.ReadAll.
	var fullData []byte
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(resp.Body)
		ch <- readResult{data, err}
	}()
	// Close the body after a brief delay so io.ReadAll returns.
	go func() {
		time.Sleep(200 * time.Millisecond)
		resp.Body.Close()
	}()
	select {
	case result := <-ch:
		fullData = result.data
		if result.err != nil {
			errStr := result.err.Error()
			if !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "closed") {
				t.Fatalf("read SSE body: %v", result.err)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading SSE initial data")
	}

	// Find the state event's JSON data.
	// SSE format: "event: state\ndata: {...}\n\n"
	stateIdx := bytesIndex(fullData, []byte("event: state"))
	if stateIdx < 0 {
		t.Fatalf("could not find state event in SSE data: %s", string(fullData))
	}
	// Find the JSON object after "data: "
	dataPrefix := []byte("data: ")
	dataStart := bytesIndex(fullData[stateIdx:], dataPrefix)
	if dataStart < 0 {
		t.Fatalf("could not find data: prefix in state event: %s", string(fullData))
	}
	dataStart += stateIdx + len(dataPrefix)

	// Find the closing brace of the JSON object.
	jsonEnd := bytesIndex(fullData[dataStart:], []byte("}\n"))
	if jsonEnd < 0 {
		t.Fatalf("could not find end of JSON in state event: %s", string(fullData))
	}
	jsonStr := string(fullData[dataStart : dataStart+jsonEnd+1])

	var state map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		t.Fatalf("invalid JSON in SSE state event: %v, data: %s", err, jsonStr)
	}
}

func bytesIndex(data, sep []byte) int {
	for i := 0; i <= len(data)-len(sep); i++ {
		match := true
		for j := 0; j < len(sep); j++ {
			if data[i+j] != sep[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func findEventEnd(data []byte) int {
	for i := 0; i < len(data)-3; i++ {
		if data[i] == '\n' && data[i+1] == '\n' {
			return i + 2
		}
	}
	return -1
}

func findJSONStart(data []byte) int {
	for i := 0; i < len(data); i++ {
		if data[i] == '{' {
			return i
		}
	}
	return -1
}

// --- Viewer cleanup ---

func TestViewer_ClientRemoval(t *testing.T) {
	_, ts, cleanup := newTestViewerServer(t)
	defer cleanup()

	// Connect and immediately close.
	resp, err := http.Get(ts.URL + "/api/v1/viewer/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Give the viewer time to process the disconnect.
	time.Sleep(100 * time.Millisecond)

	// The viewer should still be functional.
	resp2, err := http.Get(ts.URL + "/api/v1/viewer/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after client disconnect, got %d", resp2.StatusCode)
	}
}
