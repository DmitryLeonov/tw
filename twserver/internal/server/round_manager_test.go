package server

import (
	"sync"
	"testing"
	"time"

	"twserver/internal/game"
)

func newTestRoundManager(t *testing.T) (*RoundManager, *game.Engine) {
	t.Helper()
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	return NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }()), engine
}

// --- Connect / Disconnect ---

func TestConnect_SendsConnectedAndObservation(t *testing.T) {
	rm, engine := newTestRoundManager(t)
	defer rm.Shutdown()

	engine.Join("Alice")

	outbound := rm.Connect(nil, "p01") // conn is nil but enqueue doesn't use it directly

	// Drain events.
	var events []game.Event
	for {
		select {
		case ev := <-outbound:
			events = append(events, ev)
		default:
			goto done
		}
	}
done:
	// We should have received events. Check types.
	hasConnected := false
	hasObservation := false
	for _, ev := range events {
		if ev.Type == "CONNECTED" {
			hasConnected = true
		}
		if ev.Type == "OBSERVATION" {
			hasObservation = true
		}
	}
	if !hasConnected {
		t.Error("expected CONNECTED event")
	}
	if !hasObservation {
		t.Error("expected OBSERVATION event")
	}
}

func TestConnect_GenerationIncrements(t *testing.T) {
	rm, _ := newTestRoundManager(t)
	defer rm.Shutdown()

	rm.Connect(nil, "p01")
	gen1 := rm.Generation("p01")
	if gen1 != 0 {
		t.Errorf("expected generation == 0 for first connect, got %d", gen1)
	}

	// Simulate reconnect — generation should increment.
	rm.Connect(nil, "p01")
	gen2 := rm.Generation("p01")
	if gen2 <= gen1 {
		t.Errorf("expected generation %d > %d, got %d", gen2, gen1, gen2)
	}
}

func TestDisconnect_StaleGenerationIgnored(t *testing.T) {
	rm, _ := newTestRoundManager(t)
	defer rm.Shutdown()

	rm.Connect(nil, "p01")
	gen1 := rm.Generation("p01")

	// Reconnect to get a new generation.
	rm.Connect(nil, "p01")
	gen2 := rm.Generation("p01")

	// Disconnect with old generation should be ignored.
	rm.Disconnect("p01", gen1)
	// Current generation should still be gen2.
	genAfter := rm.Generation("p01")
	if genAfter != gen2 {
		t.Errorf("expected generation %d, got %d", gen2, genAfter)
	}
}

// --- SubmitAction ---

func TestSubmitAction_SendsEvent(t *testing.T) {
	rm, engine := newTestRoundManager(t)
	defer rm.Shutdown()

	engine.Join("Alice")
	engine.Join("Bob")

	outbound1 := rm.Connect(nil, "p01")
	_ = rm.Connect(nil, "p02")

	// Drain initial CONNECTED + OBSERVATION events.
	for len(outbound1) > 0 {
		<-outbound1
	}

	// Submit a pass action.
	rm.SubmitAction("p01", game.ActionMessage{
		Type:    "ACTION",
		Turn:    0,
		Actions: []game.Action{{Type: "pass"}},
	}, outbound1)

	// Wait for ACTION_ACCEPTED.
	select {
	case ev := <-outbound1:
		if ev.Type != "ACTION_ACCEPTED" {
			t.Errorf("expected ACTION_ACCEPTED, got %s", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ACTION_ACCEPTED")
	}
}

func TestSubmitAction_GameOver(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  2,
		MaxTurns:    1,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	defer rm.Shutdown()

	engine.Join("Alice")
	engine.Join("Bob")

	outbound1 := rm.Connect(nil, "p01")
	outbound2 := rm.Connect(nil, "p02")

	// Drain initial CONNECTED + OBSERVATION events.
	for len(outbound1) > 0 {
		<-outbound1
	}
	for len(outbound2) > 0 {
		<-outbound2
	}

	// Both submit pass to resolve turn 0 → game over.
	rm.SubmitAction("p01", game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, outbound1)
	rm.SubmitAction("p02", game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, outbound2)

	// Collect events from both players.
	var events1, events2 []game.Event
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go func() {
		defer close(done1)
		for {
			select {
			case ev, ok := <-outbound1:
				if !ok {
					return
				}
				events1 = append(events1, ev)
			case <-time.After(500 * time.Millisecond):
				return
			}
		}
	}()
	go func() {
		defer close(done2)
		for {
			select {
			case ev, ok := <-outbound2:
				if !ok {
					return
				}
				events2 = append(events2, ev)
			case <-time.After(500 * time.Millisecond):
				return
			}
		}
	}()

	// Wait for both goroutines.
	<-done1
	<-done2

	// Both should have received GAME_OVER.
	hasGameOver1, hasGameOver2 := false, false
	for _, ev := range events1 {
		if ev.Type == "GAME_OVER" {
			hasGameOver1 = true
		}
	}
	for _, ev := range events2 {
		if ev.Type == "GAME_OVER" {
			hasGameOver2 = true
		}
	}
	if !hasGameOver1 {
		t.Error("player 1: expected GAME_OVER event")
	}
	if !hasGameOver2 {
		t.Error("player 2: expected GAME_OVER event")
	}
}

// --- Game over handler ---

// Regression test: the game over handler is invoked from the run() goroutine
// and may itself call Stop() (like the server's StopGame does after waiting
// for Enter). It must run on its own goroutine, otherwise run() waits for
// wg.Wait() of itself and the server hangs on "Stopping server...".
func TestGameOverHandler_CanStopWithoutDeadlock(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers: 2,
		MaxTurns:   1,
		TurnDelay:  500 * time.Millisecond,
		Width:      20,
		Height:     20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	defer rm.Shutdown()

	stopped := make(chan struct{})
	rm.SetGameOverHandler(func() {
		rm.Stop() // blocks forever if the handler runs on the run() goroutine
		close(stopped)
	})

	engine.Join("Alice")
	engine.Join("Bob")

	outbound1 := rm.Connect(nil, "p01")
	outbound2 := rm.Connect(nil, "p02")
	for len(outbound1) > 0 {
		<-outbound1
	}
	for len(outbound2) > 0 {
		<-outbound2
	}

	rm.SubmitAction("p01", game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, outbound1)
	rm.SubmitAction("p02", game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, outbound2)

	select {
	case <-stopped:
		// OK: handler completed, no deadlock.
	case <-time.After(5 * time.Second):
		t.Fatal("gameOverHandler deadlocked with the run() event loop")
	}
}

// --- Event ordering ---

func TestSubmitAction_EventOrdering(t *testing.T) {
	rm, engine := newTestRoundManager(t)
	defer rm.Shutdown()

	engine.Join("Alice")
	engine.Join("Bob")

	outbound1 := rm.Connect(nil, "p01")
	_ = rm.Connect(nil, "p02")

	// Drain initial CONNECTED + OBSERVATION events.
	for len(outbound1) > 0 {
		<-outbound1
	}

	// Submit both actions.
	rm.SubmitAction("p01", game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, outbound1)
	rm.SubmitAction("p02", game.ActionMessage{Type: "ACTION", Turn: 0, Actions: []game.Action{{Type: "pass"}}}, outbound1)

	// Collect events until we see TURN_RESOLVED.
	var events []game.Event
	for {
		select {
		case ev := <-outbound1:
			events = append(events, ev)
			if ev.Type == "TURN_RESOLVED" {
				goto checkOrder
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for TURN_RESOLVED")
		}
	}
checkOrder:
	// Verify ordering: ACTION_ACCEPTED must come before TURN_RESOLVED.
	actionAcceptedIdx := -1
	turnResolvedIdx := -1
	for i, ev := range events {
		if ev.Type == "ACTION_ACCEPTED" && actionAcceptedIdx == -1 {
			actionAcceptedIdx = i
		}
		if ev.Type == "TURN_RESOLVED" && turnResolvedIdx == -1 {
			turnResolvedIdx = i
		}
	}
	if actionAcceptedIdx == -1 {
		t.Error("expected ACTION_ACCEPTED event")
	}
	if turnResolvedIdx == -1 {
		t.Error("expected TURN_RESOLVED event")
	}
	if actionAcceptedIdx > turnResolvedIdx {
		t.Errorf("ACTION_ACCEPTED (idx %d) came after TURN_RESOLVED (idx %d)", actionAcceptedIdx, turnResolvedIdx)
	}
}

// --- Shutdown ---

func TestShutdown_StopsEventLoop(t *testing.T) {
	rm, _ := newTestRoundManager(t)

	// Shutdown should complete without deadlock.
	done := make(chan struct{})
	go func() {
		rm.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete within timeout (possible deadlock)")
	}
}

// --- Bounded queue disconnect ---

func TestEnqueue_FullQueue(t *testing.T) {
	rm, _ := newTestRoundManager(t)
	defer rm.Shutdown()

	// Connect returns a 16-capacity channel. Fill it to simulate overflow.
	outbound := rm.Connect(nil, "p01")

	// Drain the CONNECTED and OBSERVATION events first.
	for len(outbound) > 0 {
		<-outbound
	}

	// Fill the channel to capacity.
	for i := 0; i < 16; i++ {
		outbound <- game.Event{Type: "FILL"}
	}

	// Next enqueue should fail (queue full) and disconnect the player.
	rm.enqueue("p01", game.Event{Type: "OVERFLOW"})

	// Try to enqueue again — should be ignored since closed.
	rm.enqueue("p01", game.Event{Type: "TEST2"})
}

// --- Concurrent safety ---

func TestRoundManager_ConcurrentSubmit(t *testing.T) {
	rm, engine := newTestRoundManager(t)
	defer rm.Shutdown()

	engine.Join("Alice")
	engine.Join("Bob")

	outbound1 := rm.Connect(nil, "p01")
	_ = rm.Connect(nil, "p02")

	// Submit from multiple goroutines concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rm.SubmitAction("p01", game.ActionMessage{
				Type:    "ACTION",
				Turn:    0,
				Actions: []game.Action{{Type: "pass"}},
			}, outbound1)
		}(i)
	}
	wg.Wait()

	// Drain events without blocking.
	var count int
	for {
		select {
		case <-outbound1:
			count++
		default:
			goto done
		}
	}
done:
	// At least one ACTION_ACCEPTED should have been sent.
	if count == 0 {
		t.Error("expected at least one event")
	}
}
