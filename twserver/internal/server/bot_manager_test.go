package server

import (
	"testing"
	"time"

	"twserver/internal/game"
)

func mustCreateBot(name string, strategy BotStrategy, seed int64) PlayerAgent {
	agent, err := CreateBot(name, strategy, seed)
	if err != nil {
		panic(err)
	}
	return agent
}

func TestNewBotManager(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, 0)

	if bm == nil {
		t.Fatal("expected non-nil BotManager")
	}
	if bm.Count() != 0 {
		t.Errorf("expected 0 bots, got %d", bm.Count())
	}
}

func TestBotManager_StartBots(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, 0)

	agents, err := BuildBotList(BotConfig{
		Enabled:         true,
		Count:           2,
		DefaultStrategy: StrategyRandom,
		Seed:            42,
	}, 4)
	if err != nil {
		t.Fatalf("BuildBotList: %v", err)
	}

	err = bm.StartBots(agents, 2*time.Second)
	if err != nil {
		t.Fatalf("StartBots: %v", err)
	}

	if bm.Count() != 2 {
		t.Errorf("expected 2 bots, got %d", bm.Count())
	}

	// Verify bots are registered in the engine.
	leaderboard := engine.Leaderboard()
	if len(leaderboard) != 2 {
		t.Errorf("expected 2 players in engine, got %d", len(leaderboard))
	}

	bm.Shutdown()
}

func TestBotManager_StartBots_ExceedsMaxPlayers(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  2,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, 0)

	// Pre-build agents (bypass BuildBotList's own max-players check).
	agents := []PlayerAgent{
		mustCreateBot("Bot-1", StrategyRandom, 1),
		mustCreateBot("Bot-2", StrategyRandom, 2),
		mustCreateBot("Bot-3", StrategyRandom, 3),
	}

	err := bm.StartBots(agents, 2*time.Second)
	if err == nil {
		t.Fatal("expected error when bot count exceeds max players")
	}
}

func TestBotManager_Shutdown(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, 0)

	agents, _ := BuildBotList(BotConfig{
		Enabled:         true,
		Count:           2,
		DefaultStrategy: StrategyRandom,
		Seed:            42,
	}, 4)

	err := bm.StartBots(agents, 2*time.Second)
	if err != nil {
		t.Fatalf("StartBots: %v", err)
	}

	// Shutdown should complete without hanging.
	done := make(chan struct{})
	go func() {
		bm.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown hung")
	}
}

func TestBotManager_LogEntries(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, 0)

	// Log entries should be empty initially.
	entries := bm.LogEntries()
	if len(entries) != 0 {
		t.Errorf("expected 0 log entries, got %d", len(entries))
	}
}

func TestBotManager_ConcurrentStartAndShutdown(t *testing.T) {
	engine := game.NewEngine(game.Config{
		MaxPlayers:  4,
		MaxTurns:    10,
		TurnTimeout: 500 * time.Millisecond,
		Width:       20,
		Height:      20,
	})
	rm := NewRoundManager(engine, func() chan struct{} { c := make(chan struct{}); close(c); return c }())
	bm := NewBotManager(engine, rm, 0)

	agents, _ := BuildBotList(BotConfig{
		Enabled:         true,
		Count:           2,
		DefaultStrategy: StrategyRandom,
		Seed:            42,
	}, 4)

	// Start bots and immediately shutdown — should not panic.
	err := bm.StartBots(agents, 2*time.Second)
	if err != nil {
		t.Fatalf("StartBots: %v", err)
	}

	bm.Shutdown()
}
