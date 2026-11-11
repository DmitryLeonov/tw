# Territory Wars Go Client

A Go client for the Territory Wars game server. This client connects to the server, registers a player, and plays the game using a configurable strategy.

## Purpose

This client demonstrates how to interact with the Territory Wars game server API using Go. It handles:

- **Player registration** via HTTP POST to `/api/v1/join`
- **State polling** via HTTP GET to `/api/v1/state`
- **Action submission** via WebSocket to `/ws`
- **Leaderboard queries** via HTTP GET to `/api/v1/leaderboard`
- **Graceful shutdown** on SIGINT/SIGTERM

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  cmd/client                     │
│  Entry point: flag parsing, config loading,     │
│  signal handling, game loop orchestration        │
├─────────────────────────────────────────────────┤
│              internal/config                    │
│  JSON config loading with defaults and          │
│  CLI flag overrides                              │
├─────────────────────────────────────────────────┤
│               internal/models                   │
│  Data types: Observation, Action, Cell,         │
│  PublicPlayer, ActionMessage, JoinResponse      │
├─────────────────────────────────────────────────┤
│                internal/api                     │
│  HTTP client for state/leaderboard,             │
│  WebSocket client for action submission         │
├─────────────────────────────────────────────────┤
│             internal/strategy                   │
│  Strategy interface + SimpleStrategy            │
│  (expand, attack, fortify, pass heuristics)     │
├─────────────────────────────────────────────────┤
│               internal/game                     │
│  Game loop: connect → observe → decide →        │
│  submit → poll → repeat until game over         │
└─────────────────────────────────────────────────┘
```

### Component Responsibilities

| Package | Responsibility |
|---------|---------------|
| `cmd/client` | CLI entry point, config loading, signal handling |
| `config` | JSON config parsing with defaults |
| `models` | JSON-serializable game data types |
| `api` | HTTP requests and WebSocket connections |
| `strategy` | Decision logic (pluggable) |
| `game` | Main game loop orchestration |

## Installation

### Prerequisites

- **Go 1.22+** ([download](https://go.dev/dl/))
- A running Territory Wars game server

### Setup

```bash
# Clone or copy the client to your workspace
cd /apps/tw/twclients/golang

# Download dependencies
go mod tidy

# Build the client
go build -o tw-client ./cmd/client/

# Run it
./tw-client
```

## Usage

### Basic Usage

```bash
# Use default config (config.json)
./tw-client

# Specify server URL
./tw-client --server http://localhost:8080

# Specify player name
./tw-client --name MyBot

# Specify config file
./tw-client --config my-config.json

# Set log level
./tw-client --log-level DEBUG
```

### All Flags

```
./tw-client --help

Flags:
  --config string    Path to config JSON file (default "config.json")
  --log-level string Log level: DEBUG, INFO, WARN, ERROR
  --name string      Player name
  --server string    Game server URL (e.g. http://localhost:8080)
```

### Using the Config File

Copy the example config and customize:

```bash
# config.json уже содержит пример конфигурации
```

Edit `config.json`:

```json
{
  "server": "http://localhost:8080",
  "player_name": "MyAwesomeBot",
  "http_timeout": 10,
  "log_level": "INFO",
  "strategy": "simple"
}
```

## Configuration

### config.json Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `server` | string | `http://localhost:8080` | Game server URL |
| `player_name` | string | `StudentBot` | Display name for your player |
| `http_timeout` | duration | `10s` | HTTP request timeout |
| `log_level` | string | `INFO` | Log level: DEBUG, INFO, WARN, ERROR |
| `strategy` | string | `simple` | Strategy to use (currently: `simple`) |

### CLI Flag Precedence

Command-line flags override config file values:

```bash
# This overrides player_name from config.json
./tw-client --name OverrideName --log-level DEBUG
```

## How to Write Custom Strategies

### The Strategy Interface

All strategies implement this interface:

```go
package strategy

import "territory-wars-client/internal/models"

type Strategy interface {
    Decide(observation *models.Observation) *models.Action
}
```

### Step-by-Step: Creating a New Strategy

1. **Create a new file** in `internal/strategy/`:

```go
// internal/strategy/aggressive.go
package strategy

import (
    "fmt"
    "territory-wars-client/internal/models"
)

type AggressiveStrategy struct{}

func NewAggressiveStrategy() *AggressiveStrategy {
    return &AggressiveStrategy{}
}

func (s *AggressiveStrategy) Decide(obs *models.Observation) *models.Action {
    // Your logic here
    return nil // nil means "pass"
}
```

2. **Analyze the observation**:

```go
func (s *AggressiveStrategy) Decide(obs *models.Observation) *models.Action {
    playerID := obs.PlayerID

    // Find enemy cells adjacent to your territory
    for _, cell := range obs.Cells {
        if cell.Owner != "" && cell.Owner != playerID {
            // This is an enemy cell - consider attacking
            if cell.Health <= 2 && obs.Energy >= 2 {
                return &models.Action{
                    Type:   "attack",
                    Target: &models.Point{X: cell.X, Y: cell.Y},
                }
            }
        }
    }

    // Find neutral cells adjacent to your territory
    for _, cell := range obs.Cells {
        if cell.Owner == "" && obs.Energy >= 1 {
            return &models.Action{
                Type:   "expand",
                Target: &models.Point{X: cell.X, Y: cell.Y},
            }
        }
    }

    // Nothing to do
    return nil
}
```

3. **Register your strategy** in `cmd/client/main.go`:

```go
func createStrategy(strategyName string, logger *slog.Logger) strategy.Strategy {
    switch strings.ToLower(strategyName) {
    case "simple":
        return strategy.NewSimpleStrategy(logger)
    case "aggressive":
        return strategy.NewAggressiveStrategy()
    default:
        return strategy.NewSimpleStrategy(logger)
    }
}
```

4. **Update config**:

```json
{
  "strategy": "aggressive"
}
```

### Action Types Reference

| Action | Energy Cost | Target Required | Description |
|--------|------------|-----------------|-------------|
| `expand` | 1 | Yes (neutral cell) | Claim a neutral cell adjacent to your territory |
| `attack` | 2 | Yes (enemy cell) | Attack an enemy cell adjacent to your territory |
| `fortify` | 2 | Yes (your cell) | Increase health of your cell (max 7) |
| `pass` | 0 | No | Skip your turn |

### Key Game Mechanics

- **Energy**: Starts at 5, increases by 1 per turn (more with resource cells), max 20
- **Cell Health**: Starts at 3 for normal cells, 5 for bases, max 7
- **Adjacency**: Actions require the target to be adjacent (up/down/left/right) to your territory
- **Turn Limit**: Default 200 turns
- **Scoring**: +1 for expand, +3 for attack (conquest), -2 for attacked cell owner

## Troubleshooting

### Connection Refused

```
Error: join request failed: Post "http://localhost:8080/api/v1/join": dial tcp 127.0.0.1:8080: connect: connection refused
```

**Fix**: Make sure the game server is running:

```bash
cd /apps/tw/twserver
go run ./cmd/server/
```

### Unauthorized Error

```
Error: observation failed with status 401: unauthorized
```

**Fix**: Your token may have expired or been invalidated. Restart the client.

### Stale Turn Error

```
Error: action failed with status 400: stale turn: current turn is 5
```

**Fix**: Your client is too slow. The server has already moved to the next turn. This is normal during high latency. The client will retry on the next observation.

### Strategy Not Found

```
Warning: unknown strategy, falling back to simple
```

**Fix**: Check your `config.json` strategy field. Currently supported: `simple`.

### High Memory Usage

The client loads the entire game state (all cells) into memory each turn. For a 40x40 map, this is 1600 cells per observation, which is negligible. If you're seeing issues, check for goroutine leaks in custom strategies.

### Debugging

Run with `--log-level DEBUG` for verbose output:

```bash
./tw-client --log-level DEBUG
```

This shows every decision, action submission, and turn resolution.

## License

This client is part of the Territory Wars project.
