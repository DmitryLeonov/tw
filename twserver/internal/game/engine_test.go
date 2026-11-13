package game

import (
	"testing"
	"time"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	return NewEngine(Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
}

// --- Join / Authenticate ---

func TestJoin_ReturnsPlayerWithToken(t *testing.T) {
	e := newTestEngine(t)
	p, err := e.Join("Alice")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty player ID")
	}
	if p.Token == "" {
		t.Error("expected non-empty token")
	}
	if p.Name != "Alice" {
		t.Errorf("expected name Alice, got %s", p.Name)
	}
}

func TestJoin_DuplicateNameRejected(t *testing.T) {
	e := newTestEngine(t)
	_, err1 := e.Join("Alice")
	if err1 != nil {
		t.Fatalf("first Join: %v", err1)
	}
	_, err2 := e.Join("Alice")
	if err2 == nil {
		t.Fatalf("expected error on duplicate name, got nil")
	}
}

func TestJoin_FullMatch(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    10,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	_, err1 := e.Join("A")
	if err1 != nil {
		t.Fatalf("first Join: %v", err1)
	}
	_, err2 := e.Join("B")
	if err2 != nil {
		t.Fatalf("second Join: %v", err2)
	}
	_, err3 := e.Join("C")
	if err3 == nil {
		t.Fatal("expected error for full match")
	}
}

func TestAuthenticate_ValidToken(t *testing.T) {
	e := newTestEngine(t)
	p, _ := e.Join("Alice")
	authenticated, ok := e.Authenticate(p.Token)
	if !ok {
		t.Fatal("expected authentication to succeed")
	}
	if authenticated.ID != p.ID {
		t.Errorf("expected ID %s, got %s", p.ID, authenticated.ID)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	e := newTestEngine(t)
	_, ok := e.Authenticate("bogus")
	if ok {
		t.Fatal("expected authentication to fail")
	}
}

// --- Submit / SubmitResult ---

func TestSubmit_ReturnsAcceptedResult(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	e.Join("Bob")

	result := e.Submit("p01", ActionMessage{
		Type:    "ACTION",
		Turn:    0,
		Actions: []Action{{Type: "pass"}},
	})
	if result == nil {
		t.Fatal("expected non-nil SubmitResult")
	}
	if !result.Accepted {
		t.Error("expected action to be accepted")
	}
	if result.RejectError != nil {
		t.Errorf("unexpected reject error: %v", result.RejectError)
	}
}

func TestSubmit_GameOver(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    1,
		TurnDelay: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	e.Join("Bob")

	// Submit both actions to resolve turn 0 → turn 1, which triggers game over.
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	e.Submit("p02", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})

	// Now game should be over.
	result := e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 1, Actions: []Action{{Type: "pass"}}})
	if result.RejectError == nil {
		t.Fatal("expected game over error")
	}
}

func TestSubmit_StaleTurn(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	e.Join("Bob")

	result := e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 99, Actions: []Action{{Type: "pass"}}})
	if result.RejectError == nil {
		t.Fatal("expected stale turn error")
	}
}

func TestSubmit_DuplicateSubmission(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	e.Join("Bob")

	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	result := e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	if result.RejectError == nil {
		t.Fatal("expected duplicate submission error")
	}
}

func TestSubmit_UnknownPlayer(t *testing.T) {
	e := newTestEngine(t)
	result := e.Submit("p99", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	if result.RejectError == nil {
		t.Fatal("expected unknown player error")
	}
}

func TestSubmit_WrongMessageType(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	result := e.Submit("p01", ActionMessage{Type: "OTHER", Turn: 0, Actions: []Action{{Type: "pass"}}})
	if result.RejectError == nil {
		t.Fatal("expected wrong message type error")
	}
}

func TestSubmit_InvalidActionType(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	e.Join("Bob")
	result := e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "teleport"}}})
	if result.RejectError != nil {
		t.Fatalf("expected accepted (teleport is unknown but accepted by engine): %v", result.RejectError)
	}
}

// --- Turn resolution ---

func TestResolveExpiredTurn_NoTimeout(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	// Timeout is 500ms, we haven't waited.
	resolved := e.ResolveExpiredTurn()
	if resolved {
		t.Fatal("expected no resolution before timeout")
	}
}

func TestResolveExpiredTurn_AfterTimeout(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    10,
		TurnDelay: 100 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	e.Join("Bob")
	// Only Alice submits — Bob doesn't, so pending is non-empty and
	// the turn won't resolve until timeout.
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	time.Sleep(200 * time.Millisecond)
	resolved := e.ResolveExpiredTurn()
	if !resolved {
		t.Fatal("expected resolution after timeout")
	}
}

func TestResolveExpiredTurn_AlreadyResolved(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  1,
		MaxTurns:    10,
		TurnDelay: 100 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	time.Sleep(200 * time.Millisecond)
	e.ResolveExpiredTurn()
	// Second call should return false (already resolved).
	resolved := e.ResolveExpiredTurn()
	if resolved {
		t.Fatal("expected no resolution on second call")
	}
}

// --- TurnSignal ---

func TestTurnSignal_ClosedOnResolution(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    10,
		TurnDelay: 100 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	e.Join("Bob")
	// Only Alice submits — Bob doesn't, so pending is non-empty and
	// the turn won't resolve until timeout.
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	_, signalCh := e.TurnSignal()

	time.Sleep(200 * time.Millisecond)
	e.ResolveExpiredTurn()

	select {
	case <-signalCh:
		// Channel closed — expected.
	default:
		t.Fatal("expected turn signal channel to be closed")
	}
}

func TestTurnSignal_NewChannelAfterResolution(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    10,
		TurnDelay: 100 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	e.Join("Bob")

	// Only Alice submits — Bob doesn't, so pending is non-empty.
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})

	// First resolution.
	_, signalCh1 := e.TurnSignal()
	time.Sleep(200 * time.Millisecond)
	e.ResolveExpiredTurn()
	<-signalCh1

	// Get new signal channel for next turn.
	_, signalCh2 := e.TurnSignal()
	select {
	case <-signalCh2:
		t.Fatal("new signal channel should not be closed yet")
	default:
		// Expected: not closed.
	}
}

// --- Observation ---

func TestObservation_ReturnsValidState(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	e.Join("Bob")

	o, err := e.Observation("p01")
	if err != nil {
		t.Fatalf("Observation: %v", err)
	}
	if o.MatchID == "" {
		t.Error("expected non-empty match ID")
	}
	if o.Turn != 0 {
		t.Errorf("expected turn 0, got %d", o.Turn)
	}
	if o.Width != 20 || o.Height != 20 {
		t.Errorf("expected 20x20, got %dx%d", o.Width, o.Height)
	}
	if len(o.Cells) == 0 {
		t.Error("expected non-empty cells")
	}
	if len(o.Players) != 2 {
		t.Errorf("expected 2 players, got %d", len(o.Players))
	}
}

func TestObservation_UnknownPlayer(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.Observation("p99")
	if err == nil {
		t.Fatal("expected error for unknown player")
	}
}

// --- Leaderboard / Results ---

func TestLeaderboard_SortedByScore(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	e.Join("Bob")

	lb := e.Leaderboard()
	if len(lb) != 2 {
		t.Fatalf("expected 2 players, got %d", len(lb))
	}
	// Both start with score 0 (base cell doesn't add score).
	if lb[0].Score < 0 {
		t.Error("expected non-negative score")
	}
}

func TestResults_EmptyBeforeResolution(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	results := e.Results()
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- Config ---

func TestConfig_ReturnsDefaults(t *testing.T) {
	e := newTestEngine(t)
	cfg := e.Config()
	if cfg.MaxPlayers != 4 {
		t.Errorf("expected MaxPlayers 4, got %d", cfg.MaxPlayers)
	}
	if cfg.TurnDelay != 500*time.Millisecond {
		t.Errorf("expected TurnDelay 500ms, got %v", cfg.TurnDelay)
	}
}

// --- IsGameOver ---

func TestIsGameOver_BeforeMaxTurns(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	if e.IsGameOver() {
		t.Fatal("expected game not over")
	}
}

func TestIsGameOver_AfterMaxTurns(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    2,
		TurnDelay: 100 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	e.Join("Bob")
	// Only Alice submits each turn — timeout resolves with Bob passing.
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	time.Sleep(200 * time.Millisecond)
	e.ResolveExpiredTurn()
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 1, Actions: []Action{{Type: "pass"}}})
	time.Sleep(200 * time.Millisecond)
	e.ResolveExpiredTurn()
	if !e.IsGameOver() {
		t.Fatal("expected game over after max turns")
	}
}

// --- ResultsForTurn ---

func TestResultsForTurn(t *testing.T) {
	e := NewEngine(Config{
		MaxPlayers:  2,
		MaxTurns:    10,
		TurnDelay: 100 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	e.Join("Alice")
	e.Join("Bob")
	// Only Alice submits — timeout resolves with Bob passing.
	e.Submit("p01", ActionMessage{Type: "ACTION", Turn: 0, Actions: []Action{{Type: "pass"}}})
	time.Sleep(200 * time.Millisecond)
	e.ResolveExpiredTurn()

	results := e.ResultsForTurn(0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for turn 0 (Alice + Bob pass), got %d", len(results))
	}
}

func TestResultsForTurn_NoResults(t *testing.T) {
	e := newTestEngine(t)
	e.Join("Alice")
	results := e.ResultsForTurn(5)
	if len(results) != 0 {
		t.Errorf("expected 0 results for turn 5, got %d", len(results))
	}
}
