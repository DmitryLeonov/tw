package server

import (
	"context"
	"fmt"
	"sync"

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
	agents       []PlayerAgent // pre-built agent list from config
}

// NewBotManager creates a new BotManager with a pre-built list of agents.
// Bots are NOT started here — they are started on demand via StartBotsOnDemand.
func NewBotManager(engine *game.Engine, rm *RoundManager, agents []PlayerAgent) *BotManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &BotManager{
		engine:       engine,
		roundManager: rm,
		logger:       &botLogger{},
		ctx:          ctx,
		cancel:       cancel,
		botIDs:       make(map[string]bool),
		agents:       agents,
	}
}

// Shutdown stops all bots and waits for their loops to finish.
// After Shutdown, a new context must be created via ResetContext() before
// calling StartBotsOnDemand again.
func (bm *BotManager) Shutdown() {
	if bm.cancel != nil {
		bm.cancel()
	}
	bm.wg.Wait()
	bm.mu.Lock()
	bm.bots = nil
	bm.botIDs = make(map[string]bool)
	bm.mu.Unlock()
}

// ResetContext creates a fresh context for bots after Shutdown.
func (bm *BotManager) ResetContext() {
	ctx, cancel := context.WithCancel(context.Background())
	bm.ctx = ctx
	bm.cancel = cancel
}

// StartBotsOnDemand registers `count` bots from the pre-built agent list.
// Bots start playing immediately.
func (bm *BotManager) StartBotsOnDemand(count int) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if count <= 0 || len(bm.agents) == 0 {
		return nil
	}

	// Take only the first `count` agents.
	agents := bm.agents[:count]

	for i, agent := range agents {
		player, err := bm.engine.Join(agent.Name())
		if err != nil {
			// Roll back already-registered bots.
			for j := 0; j < i; j++ {
				bm.bots[j].roundManager.Disconnect(bm.bots[j].playerID, 0)
			}
			return fmt.Errorf("failed to register bot %s: %w", agent.Name(), err)
		}

		bm.roundManager.ConnectBot(player.ID)

		bp := newBotPlayer(agent, player.ID, bm.roundManager, bm.engine, 0, bm.logger, 0, nil)
		bm.bots = append(bm.bots, bp)
		bm.botIDs[player.ID] = true

		bm.wg.Add(1)
		go func(bp *botPlayer) {
			defer bm.wg.Done()
			bp.Run(bm.ctx)
		}(bp)
	}

	return nil
}

// HumanPlayerCount returns the number of non-bot players currently in the engine.
func (bm *BotManager) HumanPlayerCount() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return len(bm.engine.Players()) - len(bm.botIDs)
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

// BuildBotList creates a list of PlayerAgents for bots.
// Strategies rotate through the 5 available types.
// Each bot gets a unique seed = baseSeed + index.
func BuildBotList(botsEnabled bool, maxPlayers int) ([]PlayerAgent, error) {
	if !botsEnabled {
		return nil, nil
	}

	count := maxPlayers
	seed := int64(42)

	strategies := []BotStrategy{
		StrategyAggressive,
		StrategyDefensive,
		StrategyExpansion,
		StrategyRandom,
		StrategySimple,
	}

	var agents []PlayerAgent
	for i := 0; i < count; i++ {
		strategy := strategies[i%len(strategies)]
		name := fmt.Sprintf("Bot-%s-%d", strategy, i+1)
		agent, err := CreateBot(name, strategy, seed+int64(i))
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}

	return agents, nil
}
