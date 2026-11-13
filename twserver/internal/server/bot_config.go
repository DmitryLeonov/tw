package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BotStrategy defines the available bot strategies.
type BotStrategy string

const (
	StrategyRandom     BotStrategy = "random"
	StrategyAggressive BotStrategy = "aggressive"
	StrategyDefensive  BotStrategy = "defensive"
	StrategyExpansion  BotStrategy = "expansion"
	StrategySimple     BotStrategy = "simple"
)

// GameConfig is the top-level server configuration.
type GameConfig struct {
	Port              int    `json:"port"`
	MaxPlayers        int    `json:"max_players"`
	MaxTurns          int    `json:"max_turns"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	TurnDelay       string `json:"turn_delay"`
	ResultsDir        string `json:"results_dir"`
	MarkRate          int    `json:"mark_rate"`
	MarkThreshold     int    `json:"mark_threshold"`
	BotsEnabled       bool   `json:"bots_enabled"`
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
	if cfg.TurnDelay == "" {
		cfg.TurnDelay = "15s"
	}
	// Resolve results_dir relative to the config file's directory.
	if cfg.ResultsDir != "" && !filepath.IsAbs(cfg.ResultsDir) {
		cfg.ResultsDir = filepath.Join(filepath.Dir(path), cfg.ResultsDir)
	}
	if cfg.MarkRate <= 0 {
		cfg.MarkRate = 10
	}
	if cfg.MarkThreshold <= 0 {
		cfg.MarkThreshold = 5
	}
	return &cfg, nil
}
