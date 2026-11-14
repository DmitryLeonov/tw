# Territory Wars — External Game Clients

Ready-made clients for connecting to the Territory Wars game server in Python, C#, and Go.

## Purpose

These clients provide students with a working foundation for developing their own game algorithms. Each client:

- Connects to the server via HTTP API and WebSocket
- Registers a player and receives a session token
- Receives game state (Observation)
- Sends actions (Actions) via WebSocket
- Runs autonomously until the match ends

## General Architecture

Each client consists of the same logical components:

| Component | Description |
|-----------|-------------|
| **HTTP Client** | REST API interaction (registration, state polling) |
| **WebSocket Client** | Action submission and real-time event reception |
| **API Models** | Data structures matching the server contract |
| **Game Agent** | Decision-making algorithm (Strategy) |
| **Game Loop** | Main game loop: observe → decide → submit → repeat |
| **Configuration** | Connection parameters, player name, timeouts |
| **Logging** | Game progress reporting |
| **Error Handling** | Network errors, timeouts, invalid responses |

## HTTP Protocol

### Registration
```
POST /api/v1/join
Content-Type: application/json

{"name": "PlayerName"}

Response: {"player_id": "p01", "token": "...", "websocket_url": "/ws"}
```

### Get Game State
```
GET /api/v1/state
Authorization: Bearer TOKEN

Response: Observation JSON
```

### Get Leaderboard
```
GET /api/v1/leaderboard

Response: {"status": {...}, "leaderboard": [...], "results": [...]}
```

### Send Actions
Actions are sent via WebSocket:
```
ws://localhost:8080/ws?token=YOUR_TOKEN

Message: {"type": "ACTION", "turn": 0, "actions": [{"type": "expand", "target": {"x": 20, "y": 4}}]}
```

### Observation (Game State)
```json
{
  "match_id": "string",
  "player_id": "string",
  "turn": 0,
  "game_over": false,
  "width": 40,
  "height": 40,
  "energy": 5,
  "score": 0,
  "cells": [{"x": 0, "y": 0, "owner": "p01", "health": 5, "type": "base"}],
  "players": [{"player_id": "p01", "x": 20, "y": 20, "score": 0, "connected": true, "eliminated": false}],
  "available_actions": ["expand", "attack", "fortify", "pass"]
}
```

### Action
```json
{"type": "expand", "target": {"x": 20, "y": 4}}
```

Valid action types:
- **expand** — claim an adjacent neutral cell (costs 1 energy)
- **attack** — attack an adjacent enemy cell (costs 2 energy)
- **fortify** — reinforce your own cell (+2 health, costs 2 energy)
- **pass** — skip your turn (free)

## Installation and Running

### Python Client

**Requirements:** Python 3.12+, [uv](https://github.com/astral-sh/uv)

```bash
cd twclients/python
uv sync
uv run python main.py --name PythonClient --config config.json
```

Or with command-line arguments:
```bash
uv run python main.py --server http://localhost:8080 --name "MyBot"
```

### C# Client

**Requirements:** .NET 10 SDK

```bash
cd twclients/csharp
dotnet restore
dotnet run -- --server http://localhost:8080 --name "MyBot"
```

Or with a configuration file:
```bash
dotnet run -- --config config.json
```

### Go Client

**Requirements:** Go 1.22+

```bash
cd twclients/golang
go mod tidy
go build -o bin/client ./cmd/client/
./client --name MyBot --config config.json
```

Or with command-line arguments:
```bash
./client --server http://localhost:8080 --name "MyBot" --log-level DEBUG
```

## Running All Clients

### `run.sh` Script

```bash
# Start server + all 15 clients + 10 bots (default)
./run.sh

# Start server + 15 clients (no bots)
./run.sh --no-bots

# Skip a specific language
./run.sh --no-go --no-python
```

### Running 5 Copies of Each Client

```bash
# Go
cd twclients/golang
for i in 1 2 3 4 5; do
  nohup ./bin/client --name "GoClient$i" &
done

# Python
cd twclients/python
for i in 1 2 3 4 5; do
  uv run python main.py --name "PythonClient$i" &
done

# C#
cd twclients/csharp
for i in 1 2 3 4 5; do
  dotnet exec bin/Release/net10.0/publish/TerritoryWars.Client.dll --name "CSharpClient$i" &
done
```

## Strategies

### Simple — Basic Strategy

All three clients include the `simple` strategy, which:

1. **Analyzes territory** — identifies own cells and adjacent neutral/enemy cells
2. **Evaluates resources** — checks energy level
3. **Chooses action** by priority:
   - **Expand** — if there is energy and adjacent neutral cells
   - **Attack** — if there is energy and adjacent enemy cells
   - **Fortify** — if there is energy and own cells with low health (≤3)
   - **Pass** — if no valid actions

Note: Only the `simple` strategy is currently implemented across all clients.

## How to Write Your Own Strategy

### Python

```python
from client.strategy import IGameStrategy
from client.models import Observation, Action

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Your algorithm here
        return Action(type="expand", target={"x": 10, "y": 10})
```

Register in `build_strategy()`:
```python
def build_strategy(name: str) -> IGameStrategy:
    strategies = {
        "simple": SimpleStrategy,
        "my_strategy": MyStrategy,
    }
    return strategies[name]()
```

### C#

```csharp
using TerritoryWars.Strategy;

public class MyStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        // Your algorithm here
        return new Action { Type = "expand", Target = new Point { X = 10, Y = 10 } };
    }
}
```

Register in `Program.cs`:
```csharp
IGameStrategy strategy = strategyName.ToLower() switch
{
    "simple" => new SimpleStrategy(),
    "my_strategy" => new MyStrategy(),
    _ => throw new ArgumentException(...)
};
```

### Go

```go
package strategy

import "twclient/internal/models"

type MyStrategy struct{}

func (s *MyStrategy) Decide(observation *models.Observation) *models.Action {
    // Your algorithm here
    return &models.Action{Type: "expand", Target: &models.Point{X: 10, Y: 10}}
}
```

Register in `cmd/client/main.go`:
```go
func createStrategy(strategyName string, logger *slog.Logger) strategy.Strategy {
    switch strings.ToLower(strategyName) {
    case "simple":
        return strategy.NewSimpleStrategy(logger)
    case "my_strategy":
        return NewMyStrategy(logger)
    default:
        return strategy.NewSimpleStrategy(logger)
    }
}
```

## Game Mechanics

### Map

- Size: 40 × 40
- **Toroidal** — coordinates wrap-around: `(x + 1) % Width`, `(y + 1) % Height`
- 30 spawn positions
- Cell types: `base` (base), `normal` (normal), `resource` (resource)

### Rules

- Each player starts with 5 energy and one base
- One action per turn
- All actions are resolved simultaneously at the round boundary
- A missing action becomes `pass` on timeout
- Match ends after `MAX_TURNS` (default 200)

### Energy

| Action | Cost |
|--------|------|
| `expand` | 1 |
| `attack` | 2 |
| `fortify` | 2 |
| `pass` | 0 |

### Cell Health

- Normal cells: start 3, max 7
- Bases: start 5, max 7

## Troubleshooting Common Errors

### Connection refused
```
ERROR: Failed to connect to server: Connection refused
```
**Fix:** Make sure the server is running: `./bin/server -config config.json`

### Invalid token
```
ERROR: Unauthorized — invalid token
```
**Fix:** Token expired or invalid. Restart the client.

### Stale turn
```
ERROR: Stale turn: current turn is 5
```
**Fix:** Client sends action for an old turn. Check synchronization.

### Not enough energy
```
ERROR: Action rejected: not enough energy
```
**Fix:** Strategy tries an action without enough energy. Use `pass`.

### Target outside map
```
ERROR: Action rejected: target is outside the map
```
**Fix:** Coordinates outside the map (0..39 for 40x40). Use `% Width` / `% Height` for toroidal wrap-around.

## Integration Testing

To test, start the server and connect all three clients simultaneously:

```bash
# Terminal 1: server
cd twserver
./bin/server -config config.json

# Terminal 2: Python client
cd twclients/python
uv run python main.py --server http://localhost:8080 --name "PythonBot"

# Terminal 3: C# client
cd twclients/csharp
dotnet run -- --server http://localhost:8080 --name "CSBot"

# Terminal 4: Go client
cd twclients/golang
./client --server http://localhost:8080 --name "GoBot"
```

All three clients should register, receive game state, send actions, and wait for the match to end.

## Repository Structure

```
twclients/
├── README.md                    # English version
├── README.rus.md                # Russian version
├── run.sh                       # Server + clients launch script
├── python/
│   ├── README.md
│   ├── pyproject.toml
│   ├── config.json
│   ├── main.py
│   ├── client/
│   │   ├── __init__.py
│   │   ├── api.py
│   │   ├── models.py
│   │   ├── strategy.py
│   │   └── game_loop.py
│   └── tests/
│       ├── test_models.py
│       ├── test_strategy.py
│       └── test_api.py
├── csharp/
│   ├── README.md
│   ├── TerritoryWars.Client.csproj
│   ├── config.json
│   ├── Program.cs
│   ├── Models/
│   │   ├── Observation.cs
│   │   ├── Cell.cs
│   │   ├── PublicPlayer.cs
│   │   ├── Action.cs
│   │   ├── ActionMessage.cs
│   │   ├── JoinResponse.cs
│   │   ├── ServerEvent.cs
│   │   └── Result.cs
│   ├── Api/
│   │   └── GameApiClient.cs
│   ├── Strategy/
│   │   ├── IGameStrategy.cs
│   │   └── SimpleStrategy.cs
│   ├── Game/
│   │   └── GameLoop.cs
│   └── Tests/
│       └── StrategyTests.cs
└── golang/
    ├── README.md
    ├── go.mod
    ├── go.sum
    ├── config.json
    ├── cmd/client/main.go
    └── internal/
        ├── api/
        │   └── client.go
        ├── models/
        │   └── models.go
        ├── strategy/
        │   ├── strategy.go
        │   └── simple.go
        ├── game/
        │   └── loop.go
        └── config/
            └── config.go
```

## Compatibility

All three clients:
- Use the same HTTP protocol
- Have identical JSON fields and data types
- Work with one server without changes
- Support cross-platform launch (Windows, Linux, macOS)
- Do not require server-side modifications
