package server

import (
	"context"
	"testing"
	"time"

	"twserver/internal/game"
)

// --- CreateBot ---

func TestCreateBot_UnknownStrategy(t *testing.T) {
	_, err := CreateBot("test", BotStrategy("unknown"), 0)
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestCreateBot_AllStrategies(t *testing.T) {
	strategies := []BotStrategy{
		StrategyRandom,
		StrategyAggressive,
		StrategyDefensive,
		StrategyExpansion,
	}
	for _, s := range strategies {
		agent, err := CreateBot("test-"+string(s), s, 42)
		if err != nil {
			t.Fatalf("CreateBot(%s): %v", s, err)
		}
		if agent.Name() != "test-"+string(s) {
			t.Errorf("expected name test-%s, got %s", s, agent.Name())
		}
		if agent.Strategy() != string(s) {
			t.Errorf("expected strategy %s, got %s", s, agent.Strategy())
		}
	}
}

// --- randomBot ---

func TestRandomBot_Decide(t *testing.T) {
	agent, err := CreateBot("random-test", StrategyRandom, 42)
	if err != nil {
		t.Fatal(err)
	}
	rng, ok := agent.(*randomBot)
	if !ok {
		t.Fatal("expected *randomBot")
	}

	obs := &game.Observation{Turn: 0, GameOver: false, Cells: []game.Cell{
		{X: 0, Y: 0, Owner: "p01", Type: "base", Health: 5},
	}}

	action, err := rng.Decide(context.Background(), obs)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	valid := map[string]bool{"pass": true, "expand": true, "attack": true, "fortify": true}
	if !valid[action.Type] {
		t.Errorf("unexpected action type: %s", action.Type)
	}
}

func TestRandomBot_ContextCancelled(t *testing.T) {
	agent, _ := CreateBot("random-cancel", StrategyRandom, 42)
	rng := agent.(*randomBot)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	obs := &game.Observation{Turn: 0, GameOver: false, Cells: []game.Cell{
		{X: 0, Y: 0, Owner: "p01", Type: "base", Health: 5},
	}}

	// randomBot.Decide does not check context, so it should still return an action.
	action, err := rng.Decide(ctx, obs)
	if err != nil {
		t.Fatalf("expected no error on cancelled context, got: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
}

// --- aggressiveBot ---

func TestAggressiveBot_Decide(t *testing.T) {
	agent, _ := CreateBot("agg-test", StrategyAggressive, 42)
	agg := agent.(*aggressiveBot)

	obs := &game.Observation{Turn: 0, GameOver: false, Cells: []game.Cell{
		{X: 0, Y: 0, Owner: "p01", Type: "base", Health: 5},
	}}

	action, err := agg.Decide(context.Background(), obs)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	valid := map[string]bool{"pass": true, "expand": true, "attack": true, "fortify": true}
	if !valid[action.Type] {
		t.Errorf("unexpected action type: %s", action.Type)
	}
}

// --- defensiveBot ---

func TestDefensiveBot_Decide(t *testing.T) {
	agent, _ := CreateBot("def-test", StrategyDefensive, 42)
	def := agent.(*defensiveBot)

	obs := &game.Observation{Turn: 0, GameOver: false, Cells: []game.Cell{
		{X: 0, Y: 0, Owner: "p01", Type: "base", Health: 5},
	}}

	action, err := def.Decide(context.Background(), obs)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	valid := map[string]bool{"pass": true, "expand": true, "attack": true, "fortify": true}
	if !valid[action.Type] {
		t.Errorf("unexpected action type: %s", action.Type)
	}
}

// --- expansionBot ---

func TestExpansionBot_Decide(t *testing.T) {
	agent, _ := CreateBot("exp-test", StrategyExpansion, 42)
	exp := agent.(*expansionBot)

	obs := &game.Observation{Turn: 0, GameOver: false, Cells: []game.Cell{
		{X: 0, Y: 0, Owner: "p01", Type: "base", Health: 5},
	}}

	action, err := exp.Decide(context.Background(), obs)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
	valid := map[string]bool{"pass": true, "expand": true, "attack": true, "fortify": true}
	if !valid[action.Type] {
		t.Errorf("unexpected action type: %s", action.Type)
	}
}

// --- BuildBotList ---

func TestBuildBotList_Disabled(t *testing.T) {
	agents, err := BuildBotList(false, 20)
	if err != nil {
		t.Fatalf("BuildBotList: %v", err)
	}
	if agents != nil {
		t.Errorf("expected nil agents for disabled bots, got %d", len(agents))
	}
}

func TestBuildBotList_FillsMaxPlayers(t *testing.T) {
	agents, err := BuildBotList(true, 20)
	if err != nil {
		t.Fatalf("BuildBotList: %v", err)
	}
	if len(agents) != 20 {
		t.Fatalf("expected 20 agents, got %d", len(agents))
	}
	for _, a := range agents {
		// Names are "Bot-aggressive-1", "Bot-defensive-2", "Bot-expansion-3", ...
		_ = a.Name()
	}
}

func TestBuildBotList_StrategyRotation(t *testing.T) {
	agents, err := BuildBotList(true, 5)
	if err != nil {
		t.Fatalf("BuildBotList: %v", err)
	}
	if len(agents) != 5 {
		t.Fatalf("expected 5 agents, got %d", len(agents))
	}
	names := []string{"Bot-aggressive-1", "Bot-defensive-2", "Bot-expansion-3", "Bot-random-4", "Bot-simple-5"}
	for i, a := range agents {
		if a.Name() != names[i] {
			t.Errorf("agent %d: expected %s, got %s", i, names[i], a.Name())
		}
	}
}

func TestBuildBotList_SeedUniqueness(t *testing.T) {
	agents, err := BuildBotList(true, 3)
	if err != nil {
		t.Fatalf("BuildBotList: %v", err)
	}
	// Each bot should have a different seed (42, 43, 44), so decisions differ.
	// We verify by checking names are unique.
	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name()] = true
	}
	if len(names) != 3 {
		t.Errorf("expected 3 unique names, got %d", len(names))
	}
}

// --- BotStrategy constants ---

func TestBotStrategyConstants(t *testing.T) {
	if StrategyRandom != "random" {
		t.Errorf("StrategyRandom = %q, want %q", StrategyRandom, "random")
	}
	if StrategyAggressive != "aggressive" {
		t.Errorf("StrategyAggressive = %q, want %q", StrategyAggressive, "aggressive")
	}
	if StrategyDefensive != "defensive" {
		t.Errorf("StrategyDefensive = %q, want %q", StrategyDefensive, "defensive")
	}
	if StrategyExpansion != "expansion" {
		t.Errorf("StrategyExpansion = %q, want %q", StrategyExpansion, "expansion")
	}
}

// --- Bot decision timeout ---

func TestBot_DecideWithTimeout(t *testing.T) {
	agent, _ := CreateBot("timeout-test", StrategyRandom, 42)

	obs := &game.Observation{Turn: 0, GameOver: false, Cells: []game.Cell{
		{X: 0, Y: 0, Owner: "p01", Type: "base", Health: 5},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	action, err := agent.Decide(ctx, obs)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if action == nil {
		t.Fatal("expected non-nil action")
	}
}
