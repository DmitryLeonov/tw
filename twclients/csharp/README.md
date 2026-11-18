# Territory Wars C# Client

A C# client for the Territory Wars game server, implementing an AI bot that plays the turn-based territory control game.

## Purpose

This client connects to a Territory Wars game server, receives game state observations via WebSocket, and submits strategic actions. It demonstrates how to build an AI agent for the game using C# and .NET 10.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                    Program.cs                    │
│  (Entry point, CLI args, registration, loop)    │
├─────────────────────────────────────────────────┤
│                  GameLoop.cs                     │
│  (WebSocket loop: receive → decide → submit)    │
├──────────────┬──────────────────────────────────┤
│  Api/        │  Strategy/                        │
│ GameApiClient│ IGameStrategy (interface)         │
│              │ SimpleStrategy (default impl)     │
├──────────────┴──────────────────────────────────┤
│                  Models/                         │
│  Observation, Cell, PublicPlayer, Action, etc.  │
├─────────────────────────────────────────────────┤
│                  Tests/                          │
│  StrategyTests (unit tests, excluded from build)│
└─────────────────────────────────────────────────┘
```

### Key Components

- **Program.cs** — Entry point. Parses CLI arguments, loads config, registers the player, connects WebSocket, and starts the game loop with Ctrl+C handling.
- **GameLoop.cs** — Core game loop. Polls observations over HTTP, delegates to the strategy, submits actions over WebSocket, and waits for the server response (`ACTION_ACCEPTED` / `ERROR`).
- **GameApiClient.cs** — HTTP and WebSocket client. Handles registration, action submission, and WebSocket connection management.
- **Logger.cs** — Dual writer: every log line goes to the console and, when `LogsDir` is set, to `logs_dir/player_name.log`.
- **IGameStrategy.cs** — Strategy interface. Defines how a bot decides its action each turn.
- **SimpleStrategy.cs** — Default strategy: defensive mode when the weakest owned cell has health ≤ 3 (fortify → expand → pass), otherwise attack → expand → fortify → pass.
- **Models/** — Data classes matching the server's JSON API contract.
- **Tests/** — Unit tests for strategy logic. They are excluded from the build (`<Compile Remove="Tests\**" />` in the csproj) and there is no test project, so `dotnet test` does not run them.

## Installation

### Prerequisites

- [.NET 10 SDK](https://dotnet.microsoft.com/download/dotnet/10.0) (10.0.0 or later)
- `dotnet` on PATH (e.g. `/opt/dotnet`)

### Build

```bash
cd /apps/tw/twclients/csharp
dotnet restore
dotnet publish --configuration Release
```

Output: `bin/Release/net10.0/publish/TerritoryWars.Client.dll`

### Run

```bash
# Using default config (StudentBot on localhost:8080)
dotnet run

# Override server URL and player name
dotnet run -- --server http://localhost:8080 --name MyBot

# Use a specific config file
dotnet run -- --config config.json

# Specify strategy
dotnet run -- --strategy Simple
```

### Run All Clients (Go + Python + C#)

```bash
# Go clients
cd /apps/tw/twclients/golang
for i in 1 2 3 4 5; do
  nohup ./bin/client --name "GoClient$i" &
done

# Python clients
cd /apps/tw/twclients/python
export PATH="/root/.local/bin:$PATH"
for i in 1 2 3 4 5; do
  nohup uv run python main.py --name "PythonClient$i" &
done

# C# clients
cd /apps/tw/twclients/csharp
export PATH="/opt/dotnet:$PATH"
for i in 1 2 3 4 5; do
  nohup dotnet exec bin/Release/net10.0/publish/TerritoryWars.Client.dll --name "CSharpClient$i" --strategy simple &
done
```

## Usage Examples

### Basic Usage

```bash
dotnet run -- --server http://localhost:8080 --name TerritoryBot
```

### With Configuration File

1. Edit `config.json`:

```json
{
  "ServerUrl": "http://game-server.example.com:8080",
  "PlayerName": "MyAwesomeBot",
  "HttpTimeoutSeconds": 15,
  "MaxRetries": 10,
  "LogsDir": "logs",
  "Strategy": "Simple"
}
```

2. Run:

```bash
dotnet run -- --config config.json
```

### Command-Line Arguments

| Argument     | Description                        | Default           |
|-------------|------------------------------------|-------------------|
| `--server`  | Game server HTTP URL               | `http://localhost:8080` |
| `--name`    | Player name for registration       | `StudentBot`      |
| `--config`  | Path to JSON config file           | `config.json`     |
| `--strategy`| Strategy implementation to use     | `Simple`          |

## Configuration

### config.json

| Key                  | Type    | Description                          | Default             |
|----------------------|---------|--------------------------------------|---------------------|
| `ServerUrl`          | string  | Game server base URL                 | `http://localhost:8080` |
| `PlayerName`         | string  | Display name for your bot            | `StudentBot`        |
| `HttpTimeoutSeconds` | int     | HTTP request timeout in seconds      | `10`                |
| `MaxRetries`         | int     | Maximum WebSocket reconnects before the client exits | `10` |
| `LogsDir`            | string  | Directory for `LogsDir/PlayerName.log`; empty means console only | `""` |
| `Strategy`           | string  | Strategy to use (`Simple`)           | `Simple`            |

Note: the C# client has no log level setting — every message is written to the console and to the log file.

### Environment Variables

Configuration values can also be overridden via environment variables. The
`Microsoft.Extensions.Configuration` prefix is `TERRITORYWARS_` (one underscore):

```bash
export TERRITORYWARS_SERVERURL="http://localhost:8080"
export TERRITORYWARS_PLAYERNAME="EnvBot"
```

## How to Write Custom Strategies

### Step 1: Implement IGameStrategy

Create a new class in the `Strategy/` folder that implements `IGameStrategy`:

```csharp
using TerritoryWars.Models;
using TerritoryWars.Strategy;

namespace TerritoryWars.Strategy;

public class MyStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        var owned = observation.Cells
            .Where(c => c.Owner == observation.PlayerId)
            .ToList();
        if (owned.Count == 0)
            return new Action { Type = "pass" };

        // Expand into neutral territory, then attack, then fortify
        foreach (var cell in owned)
        {
            foreach (var neighbor in GetNeighbors(cell, observation.Width, observation.Height))
            {
                var target = observation.Cells.FirstOrDefault(c => c.X == neighbor.X && c.Y == neighbor.Y);
                if (target != null && target.Owner == "" && observation.Energy >= 1)
                {
                    return new Action
                    {
                        Type = "expand",
                        Target = new Point { X = neighbor.X, Y = neighbor.Y }
                    };
                }
            }
        }

        return new Action { Type = "pass" };
    }

    private static IEnumerable<Point> GetNeighbors(Cell cell, int width, int height)
    {
        yield return new Point { X = (cell.X - 1 + width) % width, Y = cell.Y };
        yield return new Point { X = (cell.X + 1 + width) % width, Y = cell.Y };
        yield return new Point { X = cell.X, Y = (cell.Y - 1 + height) % height };
        yield return new Point { X = cell.X, Y = (cell.Y + 1 + height) % height };
    }
}
```

### Step 2: Register Your Strategy

In `Program.cs`, add your strategy to the switch expression:

```csharp
IGameStrategy strategy = strategyName.ToLower() switch
{
    "simple" => new SimpleStrategy(),
    "my_strategy" => new MyStrategy(),
    _ => throw new ArgumentException($"Unknown strategy: {strategyName}. Supported: Simple")
};
```

### Step 3: Run with Your Strategy

The switch key is matched case-insensitively, so use the registration key:

```bash
dotnet run -- --strategy my_strategy
```

## Game Mechanics Reference

### Action Types

| Action    | Description                              | Energy Cost |
|-----------|------------------------------------------|-------------|
| `expand`  | Claim a neutral cell adjacent to yours   | 1           |
| `attack`  | Attack an enemy cell adjacent to yours   | 2           |
| `fortify` | Reinforce one of your own cells          | 2           |
| `pass`    | Skip your turn                           | 0           |

### Cell Types

| Type   | Description                          |
|--------|--------------------------------------|
| `base` | Starting position, high health       |
| `normal` | Regular territory cell             |
| `resource` | Resource cell (marked visually)   |

### Movement Rules

- Actions can only target cells **adjacent** (Manhattan distance = 1) to your owned cells.
- The map is **toroidal** — coordinates wrap around using `% Width` and `% Height`.
- You need sufficient **energy** to perform actions.
- Each turn you submit **one action**.

## Troubleshooting

### Connection Refused

```
WebSocket connection failed: Connection refused
```

- Verify the server is running: `curl http://localhost:8080/health`
- Check the `ServerUrl` in your config matches the server address.
- Ensure no firewall is blocking the port.

### Authentication Errors

```
HTTP 401 Unauthorized
```

- Your token may have expired. Restart the client to re-register.
- Verify the server is accepting new players.

### Strategy Errors

```
NullReferenceException in Decide()
```

- Ensure your strategy handles edge cases: empty cell lists, eliminated players, zero energy.
- Check that returned actions have valid `Target` coordinates within map bounds.

### WebSocket Disconnections

```
WebSocket closed unexpectedly
```

- The server may have timed out your connection. Restart the client.
- Check network stability.
- The client will attempt to reconnect on the next observation cycle.

### Build Errors

```
error NETSDK1004: File not found
```

- Ensure .NET 10 SDK is installed: `dotnet --version`
- Run `dotnet restore` to download packages.

## Project Structure

```
twclients/csharp/
├── TerritoryWars.Client.csproj
├── README.md
├── config.json
├── Program.cs                    # Entry point
├── Logger.cs                     # Dual writer: console + log file
├── Api/
│   └── GameApiClient.cs          # HTTP + WebSocket client
├── Game/
│   └── GameLoop.cs               # Main game loop
├── Models/
│   ├── Observation.cs            # Game state model
│   ├── Cell.cs                   # Cell model
│   ├── PublicPlayer.cs           # Player info model
│   ├── Action.cs                 # Action model
│   ├── ActionMessage.cs          # WebSocket message model
│   ├── JoinResponse.cs           # Registration response model
│   ├── ServerEvent.cs            # Server event model
│   └── Result.cs                 # API result model
├── Strategy/
│   ├── IGameStrategy.cs          # Strategy interface
│   └── SimpleStrategy.cs         # Default strategy
├── build.sh / build.cmd          # Build scripts
├── run.sh / run.cmd              # Run scripts
└── Tests/
    └── StrategyTests.cs          # Unit tests (excluded from build, no test project)
```
