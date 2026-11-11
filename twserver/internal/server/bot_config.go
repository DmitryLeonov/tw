package server

import (
	"encoding/json"
	"fmt"
	"os"
)

// BotStrategy defines the available bot strategies.
type BotStrategy string

const (
	StrategyRandom     BotStrategy = "random"
	StrategyAggressive BotStrategy = "aggressive"
	StrategyDefensive  BotStrategy = "defensive"
	StrategyExpansion  BotStrategy = "expansion"
)

// BotPlayerConfig defines configuration for a single bot player.
type BotPlayerConfig struct {
	Name            string      `json:"name"`
	Strategy        BotStrategy `json:"strategy"`
	DecisionTimeout string      `json:"decision_timeout"`
	Seed            int64       `json:"seed"`
}

// BotConfig defines the overall bot configuration.
type BotConfig struct {
	Enabled         bool              `json:"enabled"`
	Players         []BotPlayerConfig `json:"players"`
	Count           int               `json:"count"`
	DefaultStrategy BotStrategy       `json:"default_strategy"`
	DecisionTimeout string            `json:"decision_timeout"`
	TurnDelay       string            `json:"turn_delay"`
	Seed            int64             `json:"seed"`
}

// GameConfig is the top-level server configuration.
type GameConfig struct {
	Port              int       `json:"port"`
	MaxPlayers        int       `json:"max_players"`
	MaxTurns          int       `json:"max_turns"`
	Width             int       `json:"width"`
	Height            int       `json:"height"`
	TurnTimeout       string    `json:"turn_timeout"`
	RegistrationDelay string    `json:"registration_delay"`
	ResultsDir        string    `json:"results_dir"`
	Bots              BotConfig `json:"bots"`
}

// LoadConfig reads and parses a JSON config file.
func LoadConfig(path string) (*GameConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg GameConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if cfg.MaxPlayers <= 0 {
		cfg.MaxPlayers = 20
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 200
	}
	if cfg.Width <= 0 {
		cfg.Width = 40
	}
	if cfg.Height <= 0 {
		cfg.Height = 40
	}
	if cfg.TurnTimeout == "" {
		cfg.TurnTimeout = "15s"
	}
	if cfg.RegistrationDelay == "" {
		cfg.RegistrationDelay = "1m"
	}
	if cfg.Bots.DefaultStrategy == "" {
		cfg.Bots.DefaultStrategy = StrategyRandom
	}
	if cfg.Bots.DecisionTimeout == "" {
		cfg.Bots.DecisionTimeout = "2s"
	}
	if cfg.Bots.TurnDelay == "" {
		cfg.Bots.TurnDelay = "5s"
	}
	return &cfg, nil
}
