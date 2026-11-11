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
	agent           PlayerAgent
	playerID        string
	roundManager    *RoundManager
	engine          *game.Engine
	decisionTimeout time.Duration
	logger          *botLogger
	turnDelay       time.Duration
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

func newBotPlayer(agent PlayerAgent, playerID string, rm *RoundManager, engine *game.Engine, timeout time.Duration, logger *botLogger, turnDelay time.Duration) *botPlayer {
	return &botPlayer{
		agent:           agent,
		playerID:        playerID,
		roundManager:    rm,
		engine:          engine,
		decisionTimeout: timeout,
		logger:          logger,
		turnDelay:       turnDelay,
	}
}

// Run starts the bot's decision loop. It runs until the context is cancelled
// or the game ends. Each iteration: get observation → decide → submit action.
func (bp *botPlayer) Run(ctx context.Context) {
	// Mark bot as connected.
	bp.engine.SetConnected(bp.playerID, true)

	for {
		select {
		case <-ctx.Done():
			bp.engine.SetConnected(bp.playerID, false)
			return
		default:
		}

		// Check if game is over.
		if bp.engine.IsGameOver() {
			return
		}

		// Get observation.
		obs, err := bp.engine.Observation(bp.playerID)
		if err != nil {
			bp.logger.Log(BotLogEntry{
				Turn: 0, PlayerID: bp.playerID, PlayerName: bp.agent.Name(),
				ActionType: "error", Accepted: false, Reason: err.Error(),
			})
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Check if game is over after observation.
		if obs.GameOver {
			return
		}

		// Decide action with timeout.
		decideCtx, cancel := context.WithTimeout(ctx, bp.decisionTimeout)
		action, err := bp.agent.Decide(decideCtx, obs)
		cancel()

		if err != nil {
			bp.logger.Log(BotLogEntry{
				Turn: obs.Turn, PlayerID: bp.playerID, PlayerName: bp.agent.Name(),
				ActionType: "error", Accepted: false, Reason: err.Error(),
			})
			// On decision error, pass instead.
			action = &game.Action{Type: "pass"}
		}

		// Staggered delay so the viewer has something to watch.
		time.Sleep(bp.turnDelay)

		// Submit action through RoundManager.
		msg := game.ActionMessage{
			Type:    "ACTION",
			Turn:    obs.Turn,
			Actions: []game.Action{*action},
		}
		bp.roundManager.SubmitAction(bp.playerID, msg, nil)

		// Log the action.
		bp.logger.Log(BotLogEntry{
			Turn: obs.Turn, PlayerID: bp.playerID, PlayerName: bp.agent.Name(),
			ActionType: action.Type, Accepted: true,
		})

		// Wait for turn resolution before next decision.
		bp.engine.WaitForTurn(obs.Turn, ctx)
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
	default:
		return nil, fmt.Errorf("unknown bot strategy: %s", strategy)
	}
}
