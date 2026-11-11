package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"twclient/internal/api"
	"twclient/internal/config"
	"twclient/internal/game"
	"twclient/internal/strategy"
)

func main() {
	// Command-line flags override config file values.
	server := flag.String("server", "", "Game server URL (e.g. http://localhost:8080)")
	name := flag.String("name", "", "Player name")
	configPath := flag.String("config", "config.json", "Path to config JSON file")
	logLevel := flag.String("log-level", "", "Log level: DEBUG, INFO, WARN, ERROR")
	flag.Parse()

	// Load configuration from JSON file.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply command-line overrides.
	if *server != "" {
		cfg.Server = *server
	}
	if *name != "" {
		cfg.PlayerName = *name
	}
	if *logLevel != "" {
		cfg.LogLevel = strings.ToUpper(*logLevel)
	}

	// Set up logger.
	logger := setupLogger(cfg.LogLevel)

	logger.Info("starting Territory Wars client",
		"server", cfg.Server,
		"player", cfg.PlayerName,
		"strategy", cfg.Strategy,
	)

	// Create context with signal handling for graceful shutdown.
	ctx, cancel := signalContext()
	defer cancel()

	// Join the game.
	logger.Info("registering player", "name", cfg.PlayerName)
	joinClient := api.New(cfg.Server, "")
	joinResp, err := joinClient.Join(ctx, cfg.PlayerName)
	if err != nil {
		logger.Error("failed to join game", "error", err)
		os.Exit(1)
	}
	logger.Info("joined game", "player_id", joinResp.PlayerID, "websocket_url", joinResp.WebsocketURL)

	// Create API client with token.
	client := api.NewWithLogger(cfg.Server, joinResp.Token, logger)
	defer client.Close()

	// Create strategy.
	strat := createStrategy(cfg.Strategy, logger)

	// Create and run game loop.
	loop := game.NewWithLogger(client, strat, logger)

	logger.Info("entering game loop")
	if err := loop.Run(ctx); err != nil {
		logger.Error("game loop error", "error", err)
		os.Exit(1)
	}

	logger.Info("client exiting successfully")
}

// setupLogger creates a slog.Logger configured with the given log level.
func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
}

// signalContext creates a context that is cancelled when SIGINT or SIGTERM
// is received, enabling graceful shutdown.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

// createStrategy instantiates a strategy based on the config.
func createStrategy(strategyName string, logger *slog.Logger) strategy.Strategy {
	switch strings.ToLower(strategyName) {
	case "simple":
		return strategy.NewSimpleStrategy(logger)
	default:
		logger.Warn("unknown strategy, falling back to simple", "strategy", strategyName)
		return strategy.NewSimpleStrategy(logger)
	}
}
