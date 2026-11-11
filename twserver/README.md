# Territory Wars server (MVP)

A small Go server for a turn-based, simultaneous-action territory game. Students run their own Python/C# bots and connect to the teacher's server over WebSocket.

## Requirements

- Go 1.22+
- Network access from student machines to the server

## Run

```bash
go mod tidy
go run ./cmd/server
```

Environment variables:

- `PORT` (default `8080`)
- `MAX_PLAYERS` (default `20`)
- `MAX_TURNS` (default `200`)
- `TURN_TIMEOUT_SECONDS` (default `15`)

## Register a player

```bash
curl -X POST http://localhost:8080/api/v1/join \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice"}'
```

The response contains `player_id`, `token`, and `websocket_url`. Keep the token private.

Connect:

```text
ws://localhost:8080/ws?token=YOUR_TOKEN
```

The server sends `CONNECTED`, then one `OBSERVATION` for the current turn. Submit one action per turn:

```json
{
  "type": "ACTION",
  "turn": 0,
  "actions": [
    {"type": "expand", "target": {"x": 20, "y": 4}}
  ]
}
```

After submitting an action, the server replies with `ACTION_ACCEPTED`. When the round
resolves, every connected client receives `TURN_RESOLVED` (including the round results),
followed by exactly one `OBSERVATION` for the next turn. A missing action becomes `pass`
when the round timeout expires. Reconnecting clients receive the current observation.

Supported actions:

- `expand`: claim an adjacent neutral cell, costs 1 energy
- `attack`: damage an adjacent enemy cell, costs 2 energy
- `fortify`: add 2 health to one of your cells, costs 2 energy
- `pass`: do nothing

## HTTP endpoints

- `GET /health`
- `POST /api/v1/join` with `{"name":"..."}`
- `GET /api/v1/state` with `Authorization: Bearer TOKEN`
- `GET /api/v1/leaderboard`

## Game rules in this prototype

- Map: 40 x 40, 20 spawn positions.
- Each player starts with 5 energy and a base.
- One action per player per turn.
- All submitted actions are resolved together at the round boundary.
- A missing action becomes `pass` when the turn timeout expires.
- A match ends after `MAX_TURNS`.

## Architecture

### Round Manager

A single `RoundManager` instance coordinates all rounds. It is the sole bridge between
WebSocket handlers and the game engine.

```
WebSocket handler ──► SubmitAction(actionCh) ──► run() event loop
                                                        │
                                          engine.Submit()
                                          engine.TurnSignal()
                                                        │
                                          enqueue(TURN_RESOLVED)
                                          enqueue(OBSERVATION)
                                                        ▼
                                              all connected clients
```

**Event-loop architecture:** One `run()` goroutine serialises all action processing and
round resolution. Each player has a per-player action channel; the WS handler writes
actions to this channel instead of calling the engine directly. This prevents data races
and eliminates double-resolution bugs.

### Event lifecycle

Every turn follows a strict ordering:

1. **ACTION_ACCEPTED** — sent to the player who submitted, immediately after acceptance.
2. **TURN_RESOLVED** — broadcast to all connected clients when all players have submitted
   (or the timeout expires). Includes per-player results.
3. **OBSERVATION** — sent to each client after TURN_RESOLVED, showing the board state for
   the next turn.

The invariant `ACTION_ACCEPTED → TURN_RESOLVED → OBSERVATION` is guaranteed by the
single-threaded event loop.

### Reconnect safety

Each player's connection has a monotonically increasing `generation` counter. When a
player reconnects:

1. The old connection is marked with the current generation.
2. The new connection gets `generation + 1`.
3. The event loop validates the generation before processing actions from a connection.
4. Stale actions from the old connection are silently dropped.

This prevents a reconnecting player from submitting actions that were already processed
by their previous connection.

### Bounded outbound queues

Each player has a bounded outbound channel (capacity 32). If the queue fills (the client
is slow to consume events), the connection is gracefully disconnected. The client can
reconnect and receive the current observation.

### Graceful shutdown

The server handles `SIGINT` and `SIGTERM`:

1. `http.Server.Shutdown()` stops accepting new connections and drains existing ones.
2. `RoundManager.Shutdown()` closes the context, broadcasts `GAME_OVER` to all clients,
   and terminates the event loop.

## Testing

Run all tests with the race detector:

```bash
go test -race -v ./...
```

Test categories:

| Package | Tests |
|---------|-------|
| `internal/game` | Engine unit tests — join, authenticate, submit, resolve, timeout, observation, leaderboard, game-over |
| `internal/server` | Round Manager tests — connect, disconnect, generation tracking, action submission, event ordering, shutdown, bounded queues, concurrent submits |
| `internal/server` | Integration tests — real WebSocket clients via `httptest.Server`, action flow, timeout resolution, reconnect, event ordering |

## Known MVP limitations

This is a starter implementation, not a production exam system:

1. Registration is still open and returns tokens publicly. For an exam, pre-create the roster or protect registration with an admin key.
2. `CheckOrigin` and CORS are permissive for local development. Restrict allowed origins before exposing the server.
3. There is no persistence, TLS termination, rate limiting, replay storage, or admin authentication yet.
4. The current map observation exposes all cells. Add fog-of-war filtering if hidden information is part of the game.
5. This first ruleset is deliberately small. Balance and scoring need playtesting.
6. The round coordinator uses a bounded outbound queue per connected client. A client that stops consuming events is disconnected; it can reconnect and receive the current observation.

The WebSocket coordination layer is now centralized: handlers no longer poll the engine or independently send observations. The engine signals round transitions, and the manager broadcasts `TURN_RESOLVED` plus one next-turn observation to each connected client.
