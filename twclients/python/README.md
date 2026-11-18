# Territory Wars Python Client

A Python client for the [Territory Wars](../../README.md) game server. Register, observe the game state, and submit actions via WebSocket.

## Purpose

This client connects to a Territory Wars game server, registers a player, and plays the game using a configurable strategy. It demonstrates:

- HTTP API interaction (registration, state polling, leaderboard)
- WebSocket communication (action submission, event reception)
- Strategy pattern for game AI
- Robust error handling with retry logic

## Architecture

```
main.py
  │
  ├── client/api.py      GameApiClient — HTTP + WebSocket API
  │       ├── register()     POST /api/v1/join
  │       ├── get_observation()  GET /api/v1/state
  │       ├── get_leaderboard()  GET /api/v1/leaderboard
  │       ├── connect()      WebSocket connect
  │       ├── submit_action()  Send ActionMessage via WS
  │       └── receive_event()  Receive Event from WS
  │
  ├── client/models.py   Dataclasses: Cell, PublicPlayer, Observation,
  │                       Action, ActionMessage, JoinResponse, Event
  │
  ├── client/strategy.py IGameStrategy (ABC), SimpleStrategy,
  │                       AggressiveStrategy, build_strategy()
  │
  └── client/game_loop.py GameLoop — main game loop orchestrator
```

### Data Flow

```
1. Register → JoinResponse(player_id, token)
2. Create GameApiClient(token)
3. GameLoop.run():
   a. Connect WebSocket
   b. Receive initial OBSERVATION
   c. Loop:
      - strategy.decide(observation) → Action
      - submit_action(ActionMessage)
      - wait for TURN_RESOLVED + OBSERVATION
   d. Print final state
```

## Installation

### Prerequisites

- Python 3.12+
- [uv](https://github.com/astral-sh/uv) — fast Python package installer
- A running Territory Wars server (default: `http://localhost:8080`)

### Install Dependencies

```bash
uv sync
```

This installs:
- **httpx** — Async-capable HTTP client with retry support
- **websockets** — WebSocket client for action submission
- **pytest** (dev group) — runs the unit tests in `tests/`

### Running the Tests

```bash
uv run pytest tests/ -v
```

## Usage

### Quick Start

```bash
# Register and play with default settings
uv run python main.py --server http://localhost:8080 --name MyBot

# Use a config file
uv run python main.py --config config.json

# Debug mode
uv run python main.py --config config.json --log-level DEBUG
```

### Command-Line Arguments

| Argument    | Description                        | Default              |
|-------------|------------------------------------|----------------------|
| `--server`  | Server URL (e.g. `http://localhost:8080`) | `http://localhost:8080` |
| `--name`    | Player name                       | `StudentBot`         |
| `--config`  | Path to JSON config file          | (none)               |
| `--log-level` | Logging level: DEBUG, INFO, WARNING, ERROR | `INFO` |

### Configuration File

The `config.json` file already contains an example configuration:

```json
{
  "server": "http://localhost:8080",
  "player_name": "StudentBot",
  "http_timeout": 10,
  "log_level": "INFO",
  "strategy": "simple",
  "max_retries": 10,
  "logs_dir": "logs"
}
```

| Key | Default | Description |
|-----|---------|-------------|
| `server` | `http://localhost:8080` | Server base URL |
| `player_name` | `StudentBot` | Player name for registration |
| `http_timeout` | `10` | HTTP timeout in seconds |
| `log_level` | `INFO` | DEBUG, INFO, WARNING, ERROR |
| `strategy` | `simple` | Strategy name (see below) |
| `max_retries` | `10` | Maximum WebSocket reconnects before the client exits |
| `logs_dir` | `""` (no file) | Directory for `logs_dir/player_name.log` |

Output goes to stdout and, when `logs_dir` is set, to `logs_dir/<player_name>.log`.

Available strategies:
- **`simple`** — defensive mode when the weakest owned cell has health ≤ 3 (fortify → expand → pass); otherwise attack (cost 2) → expand (cost 1) → fortify (cost 2) → pass
- **`aggressive`** — always attack (cost 2) when an adjacent enemy exists, then expand, then fortify, then pass; no defensive mode

Note: Python has both `simple` and `aggressive`; the Go and C# clients implement `simple` only.

## How to Write Custom Strategies

Subclass `IGameStrategy` and implement the `decide` method:

```python
from client.strategy import IGameStrategy
from client.models import Action, Observation, Point

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Example: expand into the first adjacent neutral cell
        owned = observation.get_owned_cells()
        for cell in owned:
            for dx, dy in [(-1, 0), (1, 0), (0, -1), (0, 1)]:
                # The map is toroidal — wrap coordinates around.
                nx = (cell.x + dx) % observation.width
                ny = (cell.y + dy) % observation.height
                target_cell = next(
                    (c for c in observation.cells if c.x == nx and c.y == ny),
                    None,
                )
                if target_cell and target_cell.owner == "" and observation.energy >= 1:
                    return Action(type="expand", target=Point(x=nx, y=ny))
        return Action(type="pass")
```

Then register it in `build_strategy()`:

```python
# In strategy.py
def build_strategy(name: str) -> IGameStrategy:
    strategies = {
        "simple": SimpleStrategy,
        "aggressive": AggressiveStrategy,
        "my_strategy": MyStrategy,
    }
    cls = strategies.get(name.lower())
    if cls is None:
        raise ValueError(f"Unknown strategy: {name}. Available: {list(strategies.keys())}")
    return cls()
```

Or use it directly in code (`GameLoop.run()` is a coroutine, so it must be awaited):

```python
import asyncio

from client.api import GameApiClient
from client.game_loop import GameLoop
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        return Action(type="pass")  # placeholder

async def main() -> None:
    join = GameApiClient.register("http://localhost:8080", "MyBot")
    api = GameApiClient(base_url="http://localhost:8080", token=join.token)
    loop = GameLoop(api_client=api, strategy=MyStrategy(), max_retries=10)
    try:
        await loop.run()
    finally:
        await api.close()

asyncio.run(main())
```

## API Reference

### GameApiClient

```python
# Registration (static method, synchronous HTTP)
join = GameApiClient.register("http://localhost:8080", "MyBot")

# Instance construction
client = GameApiClient(base_url="http://localhost:8080", token=join.token)

# Synchronous HTTP methods (httpx.Client)
obs = client.get_observation()             # GET /api/v1/state
data = client.get_leaderboard()            # GET /api/v1/leaderboard

# Coroutines (WebSocket) — must be awaited
await client.connect()                     # open the WebSocket
await client.submit_action(action_message) # send an ActionMessage
event = await client.receive_event()       # one event (default 30 s timeout)
await client.close()                       # close WebSocket + HTTP client
```

### Observation Model

```python
obs = client.get_observation()
print(f"Turn: {obs.turn}")
print(f"Score: {obs.score}")
print(f"Energy: {obs.energy}")
print(f"Cells: {len(obs.cells)}")
print(f"Owned: {len(obs.get_owned_cells())}")
print(f"Valid coord (5,5): {obs.is_valid_coord(5, 5)}")
```

### ActionMessage

```python
from client.models import Action, ActionMessage

msg = ActionMessage(
    type="ACTION",
    turn=obs.turn,
    actions=[Action(type="expand", target=Point(x=10, y=20))],
)
await client.submit_action(msg)
```

## Troubleshooting

### Connection Refused

```
Connection error: Connection refused
```

**Fix**: Ensure the server is running:
```bash
cd /apps/tw/twserver
go build -o bin/server ./cmd/server/
./bin/server -config config.json
```

### Token Unauthorized

```
Unauthorized: token invalid or expired
```

**Fix**: Re-register with `POST /api/v1/join` to get a new token. A token stays
valid across WebSocket reconnects, but `POST /api/v1/stop` invalidates every
token, so all clients must join again afterwards.

### Action Rejected

Check the server logs for rejection reasons:
- `"target must be adjacent to your territory"` — expand/attack target must be next to your cell
- `"not enough energy"` — expand costs 1, attack/fortify costs 2
- `"target is outside the map"` — coordinates must be within [0, width) × [0, height)
- `"exactly one action is required per turn"` — send exactly one action per turn

### Timeout Waiting for Events

```
Timeout waiting for event
```

**Fix**: Check that the WebSocket connection is alive. The client auto-reconnects on failure.

### Strategy Not Found

```
Unknown strategy: foo
```

**Fix**: Use `simple` or `aggressive`, or add your strategy to `build_strategy()`.

## File Structure

```
twclients/python/
├── README.md
├── pyproject.toml
├── config.json
├── main.py
├── client/
│   ├── __init__.py
│   ├── api.py
│   ├── game_loop.py
│   ├── models.py
│   └── strategy.py
└── tests/
    ├── __init__.py
    ├── test_models.py
    ├── test_strategy.py
    └── test_api.py
```

## License

This client is provided as-is for educational purposes.
