# Territory Wars C# Client

A C# client for the Territory Wars game server, implementing an AI bot that plays the turn-based territory control game.

## Purpose

This client connects to a Territory Wars game server, receives game state observations, and submits strategic actions via WebSocket. It demonstrates how to build an AI agent for the game using C# and .NET 8.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                    Program.cs                    │
│  (Entry point, CLI args, DI setup, game loop)   │
├─────────────────────────────────────────────────┤
│                  GameLoop.cs                     │
│  (Main game loop: observe → decide → submit)    │
├──────────────┬──────────────────────────────────┤
│  Api/        │  Strategy/                        │
│ GameApiClient│ IGameStrategy (interface)         │
│              │ SimpleStrategy (default impl)     │
├──────────────┴──────────────────────────────────┤
│                  Models/                         │
│  Observation, Cell, PublicPlayer, Action, etc.  │
├─────────────────────────────────────────────────┤
│                  Tests/                          │
│  StrategyTests (unit tests for SimpleStrategy)  │
└─────────────────────────────────────────────────┘
```

### Key Components

- **Program.cs** — Entry point. Parses CLI arguments, builds configuration, sets up dependency injection, and starts the game loop with Ctrl+C handling.
- **GameLoop.cs** — Core game loop. Fetches observations, delegates to the strategy, and submits actions.
- **GameApiClient.cs** — HTTP and WebSocket client. Handles registration, state polling, leaderboard queries, and action submission.
- **IGameStrategy.cs** — Strategy interface. Defines how a bot decides its action each turn.
- **SimpleStrategy.cs** — Default strategy implementation with basic territory expansion logic.
- **Models/** — Data classes matching the server's JSON API contract.
- **Tests/** — Unit tests for strategy logic.

## Installation

### Prerequisites

- [.NET 8 SDK](https://dotnet.microsoft.com/download/dotnet/8.0) (8.0.100 or later)

### Build

```bash
cd /apps/tw/twclients/csharp
dotnet restore
dotnet build
```

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

### Run Tests

```bash
dotnet test
```

## Usage Examples

### Basic Usage

```bash
dotnet run -- --server http://localhost:8080 --name TerritoryBot
```

### With Configuration File

1. Copy the example config:
   ```bash
   # config.json уже содержит пример конфигурации
   ```

2. Отредактируйте `config.json` с вашими настройками:
   ```json
   {
     "ServerUrl": "http://game-server.example.com:8080",
     "PlayerName": "MyAwesomeBot",
     "HttpTimeoutSeconds": 15,
     "LogLevel": "Debug",
     "Strategy": "Simple"
   }
   ```

3. Запуск:
   ```bash
   dotnet run -- --config config.json
   ```

### Command-Line Arguments

| Argument   | Description                        | Default              |
|------------|------------------------------------|----------------------|
| `--server` | Game server HTTP URL               | From config file     |
| `--name`   | Player name for registration       | From config file     |
| `--config` | Путь к JSON файлу конфигурации     | `config.json`        |
| `--strategy`| Strategy implementation to use   | `Simple`             |

## Configuration

### config.json

| Key                  | Type    | Description                          | Default          |
|----------------------|---------|--------------------------------------|------------------|
| `ServerUrl`          | string  | Game server base URL                 | `http://localhost:8080` |
| `PlayerName`         | string  | Display name for your bot            | `StudentBot`     |
| `HttpTimeoutSeconds` | int     | HTTP request timeout in seconds      | `10`             |
| `LogLevel`           | string  | Logging level (Debug, Information, Warning, Error) | `Information` |
| `Strategy`           | string  | Strategy to use (`Simple` or custom) | `Simple`         |

### Environment Variables

Configuration values can also be overridden via environment variables:

```bash
export TERRITORYWARS__SERVERURL="http://localhost:8080"
export TERRITORYWARS__PLAYERNAME="EnvBot"
```

## How to Write Custom Strategies

### Step 1: Implement IGameStrategy

Create a new class in the `Strategy/` folder that implements `IGameStrategy`:

```csharp
using TerritoryWars.Models;
using TerritoryWars.Strategy;

namespace TerritoryWars.Strategy;

public class AggressiveStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        var myPlayer = observation.Players.FirstOrDefault(p => p.PlayerId == observation.PlayerId);
        if (myPlayer == null || myPlayer.Eliminated)
            return new Action { Type = "pass" };

        // Find an adjacent enemy cell to attack
        foreach (var cell in observation.Cells)
        {
            if (cell.Owner != observation.PlayerId && cell.Owner != "")
            {
                // Check if we have a cell adjacent to this enemy
                var adjacentOwn = observation.Cells.FirstOrDefault(c =>
                    c.Owner == observation.PlayerId &&
                    Math.Abs(c.X - cell.X) + Math.Abs(c.Y - cell.Y) == 1);

                if (adjacentOwn != null && observation.Energy >= 1)
                {
                    return new Action
                    {
                        Type = "attack",
                        Target = new Point { X = cell.X, Y = cell.Y }
                    };
                }
            }
        }

        // Fall back to expanding into neutral territory
        foreach (var cell in observation.Cells)
        {
            if (cell.Owner == "" && observation.Energy >= 1)
            {
                var adjacentOwn = observation.Cells.FirstOrDefault(c =>
                    c.Owner == observation.PlayerId &&
                    Math.Abs(c.X - cell.X) + Math.Abs(c.Y - cell.Y) == 1);

                if (adjacentOwn != null)
                {
                    return new Action
                    {
                        Type = "expand",
                        Target = new Point { X = cell.X, Y = cell.Y }
                    };
                }
            }
        }

        return new Action { Type = "pass" };
    }
}
```

### Step 2: Register Your Strategy

In `Program.cs`, register your strategy:

```csharp
if (strategyName == "Aggressive")
{
    services.AddSingleton<IGameStrategy, AggressiveStrategy>();
}
```

### Step 3: Run with Your Strategy

```bash
dotnet run -- --strategy Aggressive
```

## Game Mechanics Reference

### Action Types

| Action   | Description                              | Energy Cost |
|----------|------------------------------------------|-------------|
| `expand` | Claim a neutral cell adjacent to yours   | 1           |
| `attack` | Attack an enemy cell adjacent to yours   | 1           |
| `fortify` | Reinforce one of your own cells        | 1           |
| `pass`   | Skip your turn                           | 0           |

### Cell Types

| Type   | Description                          |
|--------|--------------------------------------|
| `base` | Starting position, high health       |
| `normal` | Regular territory cell             |

### Movement Rules

- Actions can only target cells **adjacent** (Manhattan distance = 1) to your owned cells.
- You need sufficient **energy** to perform actions.
- Each turn you submit **one action**.

## Troubleshooting

### Connection Refused

```
WebSocket connection failed: Connection refused
```

- Verify the server is running: `curl http://localhost:8080/api/v1/state`
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

- Ensure .NET 8 SDK is installed: `dotnet --version`
- Run `dotnet restore` to download packages.

### Tests Failing

```
Test Run Failed
```

- Run with verbose output: `dotnet test --verbosity normal`
- Check that the strategy logic handles all edge cases in the test suite.

## Project Structure

```
twclients/csharp/
├── TerritoryWars.Client.csproj
├── README.md
├── config.json
├── Program.cs                    # Entry point
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
│   └── JoinResponse.cs           # Registration response model
├── Strategy/
│   ├── IGameStrategy.cs          # Strategy interface
│   └── SimpleStrategy.cs         # Default strategy
└── Tests/
    └── StrategyTests.cs          # Unit tests
```
