package server

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"twserver/internal/game"
)

// PlayerAgent is the internal interface for game participants that make
// decisions without using HTTP. Both built-in bots and future AI agents
// implement this contract.
type PlayerAgent interface {
	// Decide returns the action to take for the given observation.
	// Must respect ctx cancellation and decision timeout.
	Decide(ctx context.Context, obs *game.Observation) (*game.Action, error)

	// Name returns the agent's display name.
	Name() string

	// Strategy returns the agent's strategy type.
	Strategy() string
}

// botPlayer wraps a PlayerAgent and integrates it with the RoundManager.
// It receives observations, calls Decide(), and submits the action back
// through the RoundManager's event loop.
type botPlayer struct {
	agent      PlayerAgent
	playerID   string
	roundManager *RoundManager
	engine     *game.Engine
	logger     *botLogger
}

// botLogger records bot actions and results for the viewer history.
type botLogger struct {
	mu      sync.Mutex
	entries []BotLogEntry
}

// BotLogEntry records a single bot action and its resolution result.
type BotLogEntry struct {
	Turn       int    `json:"turn"`
	PlayerID   string `json:"player_id"`
	PlayerName string `json:"player_name"`
	ActionType string `json:"action_type"`
	Accepted   bool   `json:"accepted"`
	Reason     string `json:"reason,omitempty"`
}

func (bl *botLogger) Log(entry BotLogEntry) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	bl.entries = append(bl.entries, entry)
}

func (bl *botLogger) Entries() []BotLogEntry {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	out := make([]BotLogEntry, len(bl.entries))
	copy(out, bl.entries)
	return out
}

func newBotPlayer(agent PlayerAgent, playerID string, rm *RoundManager, engine *game.Engine, timeout time.Duration, logger *botLogger, turnDelay time.Duration, gameStarted <-chan struct{}) *botPlayer {
	return &botPlayer{
		agent:           agent,
		playerID:        playerID,
		roundManager:    rm,
		engine:          engine,
		logger:          logger,
	}
}

// Run starts the bot's decision loop. One observation/decision/submit per turn.
func (bp *botPlayer) Run(ctx context.Context) {
	bp.engine.SetConnected(bp.playerID, true)
	defer bp.engine.SetConnected(bp.playerID, false)

	turnCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if bp.engine.IsGameOver() {
			return
		}

		// Observation once per turn — provides expand/attack/fortify targets.
		obs, err := bp.engine.Observation(bp.playerID)
		if err != nil {
			bp.logger.Log(BotLogEntry{
				Turn: 0, PlayerID: bp.playerID, PlayerName: bp.agent.Name(),
				ActionType: "error", Accepted: false, Reason: err.Error(),
			})
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if obs.GameOver {
			return
		}

		// (no debug logging)

		action, err := bp.agent.Decide(ctx, obs)
		if err != nil {
			bp.logger.Log(BotLogEntry{
				Turn:  obs.Turn, PlayerID: bp.playerID, PlayerName: bp.agent.Name(),
				ActionType: "error", Accepted: false, Reason: err.Error(),
			})
			action = &game.Action{Type: "pass"}
		}

		msg := game.ActionMessage{
			Type:    "ACTION",
			Turn:    obs.Turn,
			Actions: []game.Action{*action},
		}
		bp.roundManager.SubmitAction(bp.playerID, msg, nil)

		bp.logger.Log(BotLogEntry{
			Turn: obs.Turn, PlayerID: bp.playerID, PlayerName: bp.agent.Name(),
			ActionType: action.Type, Accepted: true,
		})

		// Block until the next turn starts.
		bp.engine.WaitForTurn(obs.Turn, ctx)
		turnCount++
	}
}

// --- Bot Strategies ---

// randomBot chooses a random valid action each turn.
type randomBot struct {
	name string
	rng  *rand.Rand
}

func (b *randomBot) Name() string     { return b.name }
func (b *randomBot) Strategy() string { return "random" }

func (b *randomBot) Decide(ctx context.Context, obs *game.Observation) (*game.Action, error) {
	actions := []string{"pass", "expand", "attack", "fortify"}
	action := actions[b.rng.Intn(len(actions))]

	switch action {
	case "expand":
		targets := obs.ExpandTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "expand", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "attack":
		targets := obs.AttackTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "attack", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "fortify":
		targets := obs.FortifyTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "fortify", Target: &targets[b.rng.Intn(len(targets))]}, nil
	default:
		return &game.Action{Type: action}, nil
	}
}

// aggressiveBot favors attacks over other actions.
type aggressiveBot struct {
	name string
	rng  *rand.Rand
}

func (b *aggressiveBot) Name() string     { return b.name }
func (b *aggressiveBot) Strategy() string { return "aggressive" }

func (b *aggressiveBot) Decide(ctx context.Context, obs *game.Observation) (*game.Action, error) {
	r := b.rng.Float64()
	var action string
	switch {
	case r < 0.50:
		action = "attack"
	case r < 0.65:
		action = "expand"
	case r < 0.80:
		action = "fortify"
	default:
		action = "pass"
	}

	switch action {
	case "attack":
		targets := obs.AttackTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "attack", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "expand":
		targets := obs.ExpandTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "expand", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "fortify":
		targets := obs.FortifyTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "fortify", Target: &targets[b.rng.Intn(len(targets))]}, nil
	default:
		return &game.Action{Type: action}, nil
	}
}

// defensiveBot favors fortification and passing.
type defensiveBot struct {
	name string
	rng  *rand.Rand
}

func (b *defensiveBot) Name() string     { return b.name }
func (b *defensiveBot) Strategy() string { return "defensive" }

func (b *defensiveBot) Decide(ctx context.Context, obs *game.Observation) (*game.Action, error) {
	r := b.rng.Float64()
	var action string
	switch {
	case r < 0.10:
		action = "attack"
	case r < 0.20:
		action = "expand"
	case r < 0.60:
		action = "fortify"
	default:
		action = "pass"
	}

	switch action {
	case "attack":
		targets := obs.AttackTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "attack", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "expand":
		targets := obs.ExpandTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "expand", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "fortify":
		targets := obs.FortifyTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "fortify", Target: &targets[b.rng.Intn(len(targets))]}, nil
	default:
		return &game.Action{Type: action}, nil
	}
}

// expansionBot favors expanding territory.
type expansionBot struct {
	name string
	rng  *rand.Rand
}

func (b *expansionBot) Name() string     { return b.name }
func (b *expansionBot) Strategy() string { return "expansion" }

func (b *expansionBot) Decide(ctx context.Context, obs *game.Observation) (*game.Action, error) {
	r := b.rng.Float64()
	var action string
	switch {
	case r < 0.20:
		action = "attack"
	case r < 0.70:
		action = "expand"
	case r < 0.80:
		action = "fortify"
	default:
		action = "pass"
	}

	switch action {
	case "attack":
		targets := obs.AttackTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "attack", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "expand":
		targets := obs.ExpandTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "expand", Target: &targets[b.rng.Intn(len(targets))]}, nil
	case "fortify":
		targets := obs.FortifyTargets
		if len(targets) == 0 {
			return &game.Action{Type: "pass"}, nil
		}
		return &game.Action{Type: "fortify", Target: &targets[b.rng.Intn(len(targets))]}, nil
	default:
		return &game.Action{Type: action}, nil
	}
}

// simpleBot follows a deterministic priority: attack > expand > fortify > pass.
// When weakest cell health <= 3, defensive fallback activates: fortify > expand > pass.
// It always picks the first available target from the server's pre-computed lists,
// which already account for toroidal wrap-around.
// Priority matches the C# and Go client SimpleStrategy for comparable scores.
type simpleBot struct {
	name string
}

func (b *simpleBot) Name() string     { return b.name }
func (b *simpleBot) Strategy() string { return "simple" }

func (b *simpleBot) Decide(ctx context.Context, obs *game.Observation) (*game.Action, error) {
	// Find weakest cell health and its coordinate for defensive awareness.
	weakestHealth := 999
	var weakestCell *game.Point
	for _, cell := range obs.Cells {
		if cell.Owner == obs.PlayerID && cell.Health < weakestHealth {
			weakestHealth = cell.Health
			p := game.Point{X: cell.X, Y: cell.Y}
			weakestCell = &p
		}
	}

	// Defensive fallback: when weakest cell health <= 3, prioritize survival.
	// Fortify weakest cell > expand > pass. Attack is skipped to prevent
	// losing all territory when under pressure.
	if weakestHealth <= 3 {
		// Priority 1: Fortify weakest cell (costs 2 energy).
		if obs.Energy >= 2 && weakestCell != nil {
			return &game.Action{Type: "fortify", Target: weakestCell}, nil
		}
		// Priority 2: Expand to recover territory (costs 1 energy).
		if obs.Energy >= 1 && len(obs.ExpandTargets) > 0 {
			return &game.Action{Type: "expand", Target: &obs.ExpandTargets[0]}, nil
		}
		return &game.Action{Type: "pass"}, nil
	}

	// Priority 1: Attack adjacent enemy cell (costs 2 energy, gives +3 score).
	if obs.Energy >= 2 && len(obs.AttackTargets) > 0 {
		return &game.Action{Type: "attack", Target: &obs.AttackTargets[0]}, nil
	}

	// Priority 2: Expand into adjacent neutral cell (costs 1 energy, gives +1 score).
	if obs.Energy >= 1 && len(obs.ExpandTargets) > 0 {
		return &game.Action{Type: "expand", Target: &obs.ExpandTargets[0]}, nil
	}

	// Priority 3: Fortify weakest cell (costs 2 energy).
	if obs.Energy >= 2 && weakestCell != nil {
		return &game.Action{Type: "fortify", Target: weakestCell}, nil
	}

	// No valid action.
	return &game.Action{Type: "pass"}, nil
}

// CreateBot creates a PlayerAgent from a strategy name.
func CreateBot(name string, strategy BotStrategy, seed int64) (PlayerAgent, error) {
	rng := rand.New(rand.NewSource(seed))
	switch strategy {
	case StrategyRandom:
		return &randomBot{name: name, rng: rng}, nil
	case StrategyAggressive:
		return &aggressiveBot{name: name, rng: rng}, nil
	case StrategyDefensive:
		return &defensiveBot{name: name, rng: rng}, nil
	case StrategyExpansion:
		return &expansionBot{name: name, rng: rng}, nil
	case StrategySimple:
		return &simpleBot{name: name}, nil
	default:
		return nil, fmt.Errorf("unknown bot strategy: %s", strategy)
	}
}
