# Territory Wars Python Client

A Python client for the [Territory Wars](https://github.com/example/territory-wars) game server. Register, observe the game state, and submit actions via WebSocket.

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
  │                       build_strategy()
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
  "strategy": "simple"
}
```

Available strategies:
- **`simple`** — Expands first, then attacks, then fortifies weakest cell (health ≤3), then passes

## How to Write Custom Strategies

Subclass `IGameStrategy` and implement the `decide` method:

```python
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Example: always expand into the first available neutral cell
        owned = observation.get_owned_cells()
        for cell in owned:
            for dx, dy in [(-1, 0), (1, 0), (0, -1), (0, 1)]:
                nx, ny = cell.x + dx, cell.y + dy
                if observation.is_valid_coord(nx, ny):
                    target_cell = next(
                        (c for c in observation.cells if c.x == nx and c.y == ny),
                        None,
                    )
                    if target_cell and target_cell.owner == "":
                        return Action(type="expand", target=type('P', (), {'x': nx, 'y': ny})())
        return Action(type="pass")
```

Then register it in `build_strategy()`:

```python
# In strategy.py
def build_strategy(name: str) -> IGameStrategy:
    strategies = {
        "simple": SimpleStrategy,
        "my_strategy": MyStrategy,
    }
    return strategies[name]()
```

Or use it directly in code:

```python
from client.api import GameApiClient
from client.game_loop import GameLoop
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        return Action(type="pass")  # placeholder

api = GameApiClient("http://localhost:8080", "your-token")
loop = GameLoop(api, MyStrategy())
loop.run()
```

## API Reference

### GameApiClient

```python
# Registration (static method)
join = GameApiClient.register("http://localhost:8080", "MyBot")

# Instance methods
client = GameApiClient(base_url="http://localhost:8080", token=join.token)
obs = client.get_observation()          # GET /api/v1/state
data = client.get_leaderboard()         # GET /api/v1/leaderboard
client.connect()                        # WebSocket connect
client.submit_action(action_message)    # Send via WebSocket
event = client.receive_event()          # Receive from WebSocket
client.close()                          # Cleanup
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
client.submit_action(msg)
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

**Fix**: Re-register. Tokens are single-use per connection.

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

**Fix**: Use `simple`, or add your strategy to `build_strategy()`.

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
