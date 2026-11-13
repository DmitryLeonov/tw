# Territory Wars

Turn-based simultaneous-action territory control game server with Python, C#, and Go clients.

## Overview

Territory Wars is a multiplayer game where players expand their territory on a toroidal 40x40 grid. The server is written in Go; clients are available in Python, C#, and Go. Students develop AI strategies to compete against each other and built-in bots.

## Quick Start

### Prerequisites

- **Go 1.22+** — server
- **Python 3.12+** with [uv](https://github.com/astral-sh/uv) — Python client
- **.NET 10 SDK** — C# client
- **curl** — for API calls

### One-command launch

```bash
# Server + 5 Go + 5 Python + 5 C# clients (no bots)
./run.sh

# Server + 10 bots (bots play, no external clients)
./run.sh --bots

# Server + 10 bots + all 15 external clients
./run.sh --mixed
```

### Manual start

```bash
# Build and start server
cd twserver
go build -o bin/server ./cmd/server/
./bin/server -config config.json

# Start clients in separate terminals
cd ../twclients/golang && go run ./cmd/client/ --name GoClient --config config.json
cd ../twclients/python && uv run python main.py --name PythonClient --config config.json
cd ../twclients/csharp && dotnet run -- --name CSharpClient --config config.json
```

## Architecture

```
tw/
├── run.sh                    # Cross-platform launch script (Linux/macOS)
├── run.cmd                   # Windows launch script
├── README.md                 # This file
├── README.rus.md             # Русская версия
├── twserver/                 # Go game server
│   ├── README.md
│   ├── README.rus.md
│   ├── config.json           # Server config (bots, limits, map size)
│   ├── cmd/server/main.go    # Entry point
│   ├── internal/
│   │   ├── server/           # HTTP handlers, WebSocket, Round Manager
│   │   ├── game/             # Game engine (state, actions, resolution)
│   │   └── config/           # Config loading
│   └── bin/server            # Built binary
└── twclients/                # External game clients
    ├── README.md
    ├── README.rus.md
    ├── golang/               # Go client
    │   ├── README.md
    │   ├── README.rus.md
    │   ├── cmd/client/main.go
    │   ├── internal/
    │   └── config.json
    ├── python/               # Python client
    │   ├── README.md
    │   ├── README.rus.md
    │   ├── main.py
    │   ├── client/
    │   │   ├── api.py
    │   │   ├── models.py
    │   │   ├── strategy.py
    │   │   └── game_loop.py
    │   ├── pyproject.toml
    │   └── config.json
    └── csharp/               # C# client
        ├── README.md
        ├── README.rus.md
        ├── Program.cs
        ├── TerritoryWars.Client.csproj
        ├── Api/
        ├── Game/
        ├── Models/
        ├── Strategy/
        └── config.json
```

## Game Mechanics

### Map

- **Size:** 40 × 40
- **Toroidal:** coordinates wrap-around via `% Width` and `% Height`
- **20 spawn positions**
- **Cell types:** `base` (start), `normal`, `resource`

### Actions

| Action | Cost | Target | Description |
|--------|------|--------|-------------|
| `expand` | 1 energy | Neutral cell | Claim an adjacent neutral cell |
| `attack` | 2 energy | Enemy cell | Attack an adjacent enemy cell |
| `fortify` | 2 energy | Own cell | Reinforce your cell (+2 health, max 7) |
| `pass` | 0 | — | Skip your turn |

### Rules

- Each player starts with **5 energy** and one **base**
- **One action per turn**
- All actions resolve **simultaneously** at the round boundary
- Missing action becomes `pass` on timeout
- Match ends after **MAX_TURNS** (default 200)

### Energy & Health

- Energy: starts at 5, +1 per turn (more with resource cells), max 20
- Cell health: normal cells start at 3, bases at 5, max 7

## API

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/join` | Register player (`{"name":"..."}`) |
| `GET` | `/api/v1/state` | Get observation (`Authorization: Bearer TOKEN`) |
| `GET` | `/api/v1/leaderboard` | Get leaderboard and game status |
| `GET` | `/api/v1/results` | Get final results (after game over) |
| `POST` | `/api/v1/start` | Start the game |
| `POST` | `/api/v1/stop` | Stop the game, reset state, disconnect all clients |

### WebSocket

```
ws://localhost:8080/ws?token=YOUR_TOKEN
```

Events: `CONNECTED`, `OBSERVATION`, `ACTION_ACCEPTED`, `TURN_RESOLVED`, `GAME_OVER`

Action format:
```json
{
  "type": "ACTION",
  "turn": 0,
  "actions": [{"type": "expand", "target": {"x": 20, "y": 4}}]
}
```

## Running Multiple Clients

### Using `run.sh`

```bash
./run.sh              # Server + 5 Go + 5 Python + 5 C# (no bots)
./run.sh --bots       # Server + 10 bots
./run.sh --mixed      # Server + 10 bots + 15 clients
./run.sh --no-go      # Skip Go clients
./run.sh --no-python  # Skip Python clients
./run.sh --no-csharp  # Skip C# clients
```

### Manual (5 copies each)

```bash
# Go
cd twclients/golang
for i in 1 2 3 4 5; do
  nohup go run ./cmd/client --name "GoClient$i" > /tmp/go_client_$i.log 2>&1 &
done

# Python
cd twclients/python
for i in 1 2 3 4 5; do
  uv run python main.py --name "PythonClient$i" > /tmp/python_client_$i.log 2>&1 &
done

# C#
cd twclients/csharp
for i in 1 2 3 4 5; do
  dotnet run --project . -- --name "CSharpClient$i" > /tmp/csharp_client_$i.log 2>&1 &
done
```

## Viewer

Real-time map viewer: `http://localhost:8080/viewer`

Uses Server-Sent Events (SSE) at `/api/v1/viewer/events`.

## Bots

The server can run 10 built-in AI bots configured in `twserver/config.json`:

```json
{
  "bots": {
    "enabled": true,
    "default_strategy": "simple",
    "turn_delay": "3s",
    "players": [
      { "name": "Alpha", "strategy": "aggressive", "seed": 100 },
      { "name": "Bravo", "strategy": "aggressive", "seed": 101 },
      { "name": "Beta", "strategy": "defensive", "seed": 200 },
      { "name": "Charlie", "strategy": "defensive", "seed": 201 },
      { "name": "Gamma", "strategy": "expansion", "seed": 300 },
      { "name": "Delta", "strategy": "expansion", "seed": 301 },
      { "name": "Echo", "strategy": "random", "seed": 400 },
      { "name": "Foxtrot", "strategy": "random", "seed": 401 },
      { "name": "Golf", "strategy": "simple", "seed": 500 },
      { "name": "Hammer", "strategy": "simple", "seed": 501 }
    ]
  }
}
```

Supported strategies: `simple`, `aggressive`, `defensive`, `expansion`, `random`.

## Writing Your Own Strategy

### Python

```python
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Your algorithm here
        return Action(type="expand", target=...)
```

### C#

```csharp
using TerritoryWars.Models;
using TerritoryWars.Strategy;

public class MyStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        return new Action { Type = "expand", Target = new Point { X = 10, Y = 10 } };
    }
}
```

### Go

```go
package strategy

import "twclient/internal/models"

type MyStrategy struct{}

func (s *MyStrategy) Decide(obs *models.Observation) *models.Action {
    return &models.Action{Type: "expand", Target: &models.Point{X: 10, Y: 10}}
}
```

## Troubleshooting

| Error | Fix |
|-------|-----|
| `Connection refused` | Server not running: `./bin/server -config config.json` |
| `401 Unauthorized` | Token expired. Restart client. |
| `Stale turn` | Client too slow. Normal under load. |
| `Not enough energy` | Strategy tries action without energy. Use `pass`. |
| `Target outside map` | Coordinates out of bounds. Use `% Width` / `% Height` for wrap-around. |

## Logs

- Server: `/tmp/server.log`
- Go clients: `/tmp/go_client_{1-5}.log`
- Python clients: `/tmp/python_client_{1-5}.log`
- C# clients: `/tmp/csharp_client_{1-5}.log`

## Compatibility

- All clients use the same HTTP protocol and JSON contract
- Cross-platform: Windows, Linux, macOS
- No server-side modifications required for new clients
