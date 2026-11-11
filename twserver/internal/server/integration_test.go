package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"twserver/internal/game"
)

// newTestServer creates a Server backed by a test engine and an HTTP
// test server, returning the server, its URL, and a cleanup function.
func newTestServer(t *testing.T) (*Server, *httptest.Server, func()) {
	t.Helper()
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	srv := New(engine)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.WebSocket)
	mux.HandleFunc("/join", srv.Join)
	mux.HandleFunc("/health", srv.Health)
	mux.HandleFunc("/state", srv.State)
	mux.HandleFunc("/leaderboard", srv.Leaderboard)
	ts := httptest.NewServer(mux)
	cleanup := func() {
		srv.Shutdown()
		ts.Close()
	}
	return srv, ts, cleanup
}

// dialWS connects to the WebSocket endpoint with the given token.
func dialWS(t *testing.T, ts *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

// readLoop starts a goroutine that reads JSON events from the connection
// and sends them on the returned channel. It closes the channel when the
// connection is closed or a read error occurs.
func readLoop(conn *websocket.Conn) <-chan game.Event {
	ch := make(chan game.Event, 32)
	go func() {
		defer close(ch)
		for {
			var ev game.Event
			if err := conn.ReadJSON(&ev); err != nil {
				return
			}
			select {
			case ch <- ev:
			default:
				// Channel full, drop event.
			}
		}
	}()
	return ch
}

// readNext reads the next event from the channel, blocking with a timeout.
func readNext(t *testing.T, ch <-chan game.Event, timeout time.Duration) game.Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return ev
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for event")
	}
	return game.Event{}
}

// drainEvents reads all available events from the channel until no more
// arrive within the given idle timeout.
func drainEvents(ch <-chan game.Event, idle time.Duration) []game.Event {
	var events []game.Event
	start := time.Now()
	for time.Since(start) < idle {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-time.After(10 * time.Millisecond):
			// No data for 10ms, keep trying up to idle total.
		}
	}
	return events
}

// --- Integration: Connect ---

func TestIntegration_Connect(t *testing.T) {
	_, ts, cleanup := newTestServer(t)
	defer cleanup()

	// Join as Alice.
	resp, err := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var joinResp struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&joinResp); err != nil {
		t.Fatal(err)
	}
	if joinResp.PlayerID == "" || joinResp.Token == "" {
		t.Fatal("missing player_id or token")
	}

	// Dial WebSocket and start read loop.
	conn := dialWS(t, ts, joinResp.Token)
	defer conn.Close()
	ch := readLoop(conn)

	// Should receive CONNECTED + OBSERVATION.
	events := drainEvents(ch, 100*time.Millisecond)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if events[0].Type != "CONNECTED" {
		t.Errorf("expected CONNECTED, got %s", events[0].Type)
	}
	if events[1].Type != "OBSERVATION" {
		t.Errorf("expected OBSERVATION, got %s", events[1].Type)
	}
}

// --- Integration: Action submission and resolution ---

func TestIntegration_ActionFlow(t *testing.T) {
	_, ts, cleanup := newTestServer(t)
	defer cleanup()

	// Join two players.
	resp1, _ := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Alice"}`))
	var p1 struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	json.NewDecoder(resp1.Body).Decode(&p1)
	resp1.Body.Close()

	resp2, _ := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Bob"}`))
	var p2 struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	json.NewDecoder(resp2.Body).Decode(&p2)
	resp2.Body.Close()

	// Dial both players and start read loops.
	conn1 := dialWS(t, ts, p1.Token)
	defer conn1.Close()
	ch1 := readLoop(conn1)
	conn2 := dialWS(t, ts, p2.Token)
	defer conn2.Close()
	ch2 := readLoop(conn2)

	// Drain initial events.
	drainEvents(ch1, 100*time.Millisecond)
	drainEvents(ch2, 100*time.Millisecond)

	// Player 1 submits a pass action.
	action1 := `{"type":"ACTION","turn":0,"actions":[{"type":"pass"}]}`
	conn1.WriteMessage(websocket.TextMessage, []byte(action1))

	// Wait for ACTION_ACCEPTED on player 1.
	ev := readNext(t, ch1, 2*time.Second)
	if ev.Type != "ACTION_ACCEPTED" {
		t.Errorf("expected ACTION_ACCEPTED, got %s", ev.Type)
	}

	// Player 2 submits a pass action — this resolves the turn.
	action2 := `{"type":"ACTION","turn":0,"actions":[{"type":"pass"}]}`
	conn2.WriteMessage(websocket.TextMessage, []byte(action2))

	// Player 2 receives ACTION_ACCEPTED (their own submission).
	ev = readNext(t, ch2, 2*time.Second)
	if ev.Type != "ACTION_ACCEPTED" {
		t.Errorf("player 2: expected ACTION_ACCEPTED, got %s", ev.Type)
	}

	// Player 1 should receive TURN_RESOLVED + OBSERVATION.
	ev = readNext(t, ch1, 2*time.Second)
	if ev.Type != "TURN_RESOLVED" {
		t.Errorf("expected TURN_RESOLVED, got %s", ev.Type)
	}
	ev = readNext(t, ch1, 2*time.Second)
	if ev.Type != "OBSERVATION" {
		t.Errorf("expected OBSERVATION, got %s", ev.Type)
	}

	// Player 2 should also receive TURN_RESOLVED + OBSERVATION.
	ev = readNext(t, ch2, 2*time.Second)
	if ev.Type != "TURN_RESOLVED" {
		t.Errorf("player 2: expected TURN_RESOLVED, got %s", ev.Type)
	}
	ev = readNext(t, ch2, 2*time.Second)
	if ev.Type != "OBSERVATION" {
		t.Errorf("player 2: expected OBSERVATION, got %s", ev.Type)
	}
}

// --- Integration: Timeout resolution ---

func TestIntegration_TimeoutResolution(t *testing.T) {
	_, ts, cleanup := newTestServer(t)
	defer cleanup()

	// Join one player.
	resp, _ := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Alice"}`))
	var p struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()

	conn := dialWS(t, ts, p.Token)
	defer conn.Close()
	ch := readLoop(conn)

	// Drain initial events.
	drainEvents(ch, 100*time.Millisecond)

	// Submit one action to start the turn.
	action := `{"type":"ACTION","turn":0,"actions":[{"type":"pass"}]}`
	conn.WriteMessage(websocket.TextMessage, []byte(action))

	// Wait for ACTION_ACCEPTED.
	ev := readNext(t, ch, 2*time.Second)
	if ev.Type != "ACTION_ACCEPTED" {
		t.Errorf("expected ACTION_ACCEPTED, got %s", ev.Type)
	}

	// Wait for timeout resolution (500ms timeout).
	ev = readNext(t, ch, 2*time.Second)
	if ev.Type != "TURN_RESOLVED" {
		t.Errorf("expected TURN_RESOLVED after timeout, got %s", ev.Type)
	}
}

// --- Integration: Reconnect ---

func TestIntegration_Reconnect(t *testing.T) {
	_, ts, cleanup := newTestServer(t)
	defer cleanup()

	// Join Alice.
	resp, _ := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Alice"}`))
	var p struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()

	// First connection.
	conn1 := dialWS(t, ts, p.Token)
	ch1 := readLoop(conn1)
	drainEvents(ch1, 100*time.Millisecond)
	conn1.Close()

	// Small delay to let disconnect process.
	time.Sleep(50 * time.Millisecond)

	// Second connection (reconnect).
	conn2 := dialWS(t, ts, p.Token)
	defer conn2.Close()
	ch2 := readLoop(conn2)

	// Should receive CONNECTED + OBSERVATION again.
	events := drainEvents(ch2, 100*time.Millisecond)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events on reconnect, got %d", len(events))
	}
	if events[0].Type != "CONNECTED" {
		t.Errorf("expected CONNECTED on reconnect, got %s", events[0].Type)
	}
}

// --- Integration: Health endpoint ---

func TestIntegration_Health(t *testing.T) {
	_, ts, cleanup := newTestServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status ok, got %s", result.Status)
	}
}

// --- Integration: Event ordering ---

func TestIntegration_EventOrdering(t *testing.T) {
	_, ts, cleanup := newTestServer(t)
	defer cleanup()

	// Join two players.
	resp1, _ := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Alice"}`))
	var p1 struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	json.NewDecoder(resp1.Body).Decode(&p1)
	resp1.Body.Close()

	resp2, _ := http.Post(ts.URL+"/join", "application/json",
		strings.NewReader(`{"name":"Bob"}`))
	var p2 struct {
		PlayerID string `json:"player_id"`
		Token    string `json:"token"`
	}
	json.NewDecoder(resp2.Body).Decode(&p2)
	resp2.Body.Close()

	conn1 := dialWS(t, ts, p1.Token)
	defer conn1.Close()
	ch1 := readLoop(conn1)
	conn2 := dialWS(t, ts, p2.Token)
	defer conn2.Close()
	ch2 := readLoop(conn2)

	drainEvents(ch1, 100*time.Millisecond)
	drainEvents(ch2, 100*time.Millisecond)

	// Both submit actions.
	action1 := `{"type":"ACTION","turn":0,"actions":[{"type":"pass"}]}`
	action2 := `{"type":"ACTION","turn":0,"actions":[{"type":"pass"}]}`
	conn1.WriteMessage(websocket.TextMessage, []byte(action1))
	conn2.WriteMessage(websocket.TextMessage, []byte(action2))

	// Player 1 should receive: ACTION_ACCEPTED, TURN_RESOLVED, OBSERVATION.
	ev1 := readNext(t, ch1, 2*time.Second)
	if ev1.Type != "ACTION_ACCEPTED" {
		t.Errorf("expected ACTION_ACCEPTED, got %s", ev1.Type)
	}
	ev2 := readNext(t, ch1, 2*time.Second)
	if ev2.Type != "TURN_RESOLVED" {
		t.Errorf("expected TURN_RESOLVED, got %s", ev2.Type)
	}
	ev3 := readNext(t, ch1, 2*time.Second)
	if ev3.Type != "OBSERVATION" {
		t.Errorf("expected OBSERVATION, got %s", ev3.Type)
	}
}
