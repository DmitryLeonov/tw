package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"twserver/internal/game"
)

// BotManager creates and manages built-in bot players.
// Bots are registered with the engine and RoundManager, then run
// their decision loops in separate goroutines.
type BotManager struct {
	engine       *game.Engine
	roundManager *RoundManager
	bots         []*botPlayer
	logger       *botLogger
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	botIDs       map[string]bool
	turnDelay    time.Duration
}

// NewBotManager creates a new BotManager.
func NewBotManager(engine *game.Engine, rm *RoundManager, turnDelay time.Duration) *BotManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &BotManager{
		engine:       engine,
		roundManager: rm,
		logger:       &botLogger{},
		ctx:          ctx,
		cancel:       cancel,
		botIDs:       make(map[string]bool),
		turnDelay:    turnDelay,
	}
}

// StartBots registers the given agents as players in the engine,
// connects them to the RoundManager, and starts their decision loops.
// Returns an error if registration fails (e.g., match is full).
func (bm *BotManager) StartBots(agents []PlayerAgent, decisionTimeout time.Duration) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for i, agent := range agents {
		// Register with engine.
		player, err := bm.engine.Join(agent.Name())
		if err != nil {
			// Rollback previously registered bots.
			for j := 0; j < i; j++ {
				bm.bots[j].roundManager.Disconnect(bm.bots[j].playerID, 0)
			}
			return fmt.Errorf("failed to register bot %s: %w", agent.Name(), err)
		}

		// Connect to RoundManager.
		outbound := bm.roundManager.Connect(nil, player.ID)
		_ = outbound // bots don't need outbound events for decision loop

		// Create bot player wrapper.
		bp := newBotPlayer(agent, player.ID, bm.roundManager, bm.engine, decisionTimeout, bm.logger, bm.turnDelay)
		bm.bots = append(bm.bots, bp)

		// Start decision loop.
		bm.wg.Add(1)
		go func(bp *botPlayer) {
			defer bm.wg.Done()
			bp.Run(bm.ctx)
		}(bp)
	}

	return nil
}

// Shutdown stops all bots and waits for their loops to finish.
func (bm *BotManager) Shutdown() {
	bm.cancel()
	bm.wg.Wait()
}

// LogEntries returns all bot action log entries.
func (bm *BotManager) LogEntries() []BotLogEntry {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.logger.Entries()
}

// Count returns the number of active bots.
func (bm *BotManager) Count() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return len(bm.bots)
}

// RegisterBot registers a bot player with the BotManager so the viewer can identify it.
func (bm *BotManager) RegisterBot(playerID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.botIDs[playerID] = true
}

// BotIDs returns the set of player IDs that belong to bots.
func (bm *BotManager) BotIDs() map[string]bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	out := make(map[string]bool, len(bm.botIDs))
	for k, v := range bm.botIDs {
		out[k] = v
	}
	return out
}

// BuildBotList creates a list of PlayerAgents from BotConfig.
func BuildBotList(cfg BotConfig, maxPlayers int) ([]PlayerAgent, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	var agents []PlayerAgent

	// If individual player configs are provided, use them.
	if len(cfg.Players) > 0 {
		if len(cfg.Players) > maxPlayers {
			return nil, fmt.Errorf("bot count %d exceeds max players %d", len(cfg.Players), maxPlayers)
		}
		for _, p := range cfg.Players {
			agent, err := CreateBot(p.Name, p.Strategy, p.Seed)
			if err != nil {
				return nil, err
			}
			agents = append(agents, agent)
		}
		return agents, nil
	}

	// Otherwise, generate bots from count + default strategy.
	count := cfg.Count
	if count <= 0 {
		return nil, nil
	}
	if count > maxPlayers {
		return nil, fmt.Errorf("bot count %d exceeds max players %d", count, maxPlayers)
	}

	seed := cfg.Seed
	if seed == 0 {
		seed = 42
	}

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("Bot-%s-%d", cfg.DefaultStrategy, i+1)
		agent, err := CreateBot(name, cfg.DefaultStrategy, seed+int64(i))
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}

	return agents, nil
}
