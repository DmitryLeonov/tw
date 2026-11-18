package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the client configuration loaded from a JSON file or
// overridden via command-line flags.
type Config struct {
	Server      string `json:"server"`
	PlayerName  string `json:"player_name"`
	HTTPTimeout int    `json:"http_timeout"` // seconds
	LogLevel    string `json:"log_level"`
	LogsDir     string `json:"logs_dir"`
	Strategy    string `json:"strategy"`
	MaxRetries  int    `json:"max_retries"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server:      "http://localhost:8080",
		PlayerName:  "StudentBot",
		HTTPTimeout: 10,
		LogLevel:    "INFO",
		Strategy:    "simple",
		MaxRetries:  10,
	}
}

// Load reads a JSON config file from the given path and merges it over
// the defaults. Flags passed at the command line take precedence.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	// Apply defaults for empty fields.
	if cfg.Server == "" {
		cfg.Server = DefaultConfig().Server
	}
	if cfg.PlayerName == "" {
		cfg.PlayerName = DefaultConfig().PlayerName
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = DefaultConfig().HTTPTimeout
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultConfig().LogLevel
	}
	if cfg.Strategy == "" {
		cfg.Strategy = DefaultConfig().Strategy
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultConfig().MaxRetries
	}

	return cfg, nil
}
