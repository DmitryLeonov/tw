# Territory Wars — Правила разработки внешних клиентов

## 1. Структура клиентов

Каждый клиент состоит из одинаковых логических компонентов:

| Компонент | Описание |
|-----------|----------|
| **HTTP Client** | REST API (регистрация, опрос состояния) |
| **WebSocket Client** | Отправка действий, приём событий |
| **API Models** | Структуры данных, соответствующие контракту сервера |
| **Game Agent** | Алгоритм принятия решений (Strategy) |
| **Game Loop** | Основной цикл: observe → decide → submit → repeat |
| **Configuration** | Параметры подключения, имя игрока, таймауты |
| **Logging** | Дублирование вывода в stdout + log file |
| **Error Handling** | Сетевые ошибки, таймауты, невалидные ответы |

### 1.1 Go-клиент

```
twclients/golang/
├── cmd/client/main.go           # Точка входа
├── internal/api/client.go       # HTTP + WebSocket API
├── internal/config/config.go    # Загрузка конфигурации
├── internal/game/loop.go        # Игровой цикл
├── internal/models/models.go    # Модели данных
├── internal/strategy/strategy.go # Интерфейс Strategy
├── internal/strategy/simple.go  # Простая стратегия
├── config.json
└── go.mod / go.sum
```

### 1.2 Python-клиент

```
twclients/python/
├── main.py                      # Точка входа
├── client/
│   ├── __init__.py
│   ├── api.py                   # HTTP + WebSocket API
│   ├── models.py                # Модели данных
│   ├── strategy.py              # IGameStrategy + стратегии
│   └── game_loop.py             # Игровой цикл
├── config.json
├── pyproject.toml
└── tests/
    ├── test_api.py
    ├── test_models.py
    └── test_strategy.py
```

### 1.3 C#-клиент

```
twclients/csharp/
├── Program.cs                   # Точка входа
├── TerritoryWars.Client.csproj
├── Logger.cs                    # Dual writer (Console + file)
├── Api/GameApiClient.cs         # HTTP + WebSocket API
├── Game/GameLoop.cs             # Игровой цикл
├── Models/                      # Models (Observation, Cell, Action, etc.)
├── Strategy/
│   ├── IGameStrategy.cs         # Интерфейс стратегии
│   └── SimpleStrategy.cs        # Простая стратегия
├── Tests/StrategyTests.cs       # Тесты стратегии
├── config.json
├── build.sh / build.cmd         # Скрипты сборки
├── run.sh / run.cmd             # Скрипты запуска
└── README.md / README.rus.md
```

## 2. HTTP-протокол

### 2.1 Регистрация (Join)

```
POST /api/v1/join
Content-Type: application/json
Body: {"name": "PlayerName"}

Response (201):
{
  "player_id": "p01",
  "token": "abc123...",
  "websocket_url": "/ws"
}
```

**Правило:** Токен — 48-символьный hex. Используется для всех последующих запросов.

### 2.2 Наблюдение (State)

```
GET /api/v1/state
Authorization: Bearer TOKEN

Response (200): Observation (см. раздел 3.1)
```

**Правило:** HTTP-опрос используется Go-клиентом для получения состояния. Python-клиент получает OBSERVATION через WebSocket. C#-клиент опрашивает через HTTP.

### 2.3 Таблица лидеров (Leaderboard)

```
GET /api/v1/leaderboard

Response (200):
{
  "status": {...},
  "leaderboard": [
    {"player_id": "p01", "score": 100},
    ...
  ]
}
```

### 2.4 WebSocket

```
ws://localhost:8080/ws?token=YOUR_TOKEN
```

Токен передаётся через query parameter.

**Правило:** Все три клиента используют один и тот же WebSocket-протокол.

## 3. Модели данных

### 3.1 Observation

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
  "players": [{"player_id": "p01", "x": 0, "y": 0, "score": 0, "connected": true, "eliminated": false}],
  "available_actions": ["expand", "attack", "fortify", "pass"]
}
```

**Правило:** Все три клиента используют одинаковые JSON-поля. Go и C# используют snake_case в JSON (через теги), Python — snake_case в dataclass-полях.

### 3.2 Action

```json
{"type": "expand", "target": {"x": 20, "y": 4}}
```

Допустимые типы: `expand` (1 energy), `attack` (2 energy), `fortify` (2 energy), `pass` (0).

### 3.3 ActionMessage (WebSocket envelope)

```json
{"type": "ACTION", "turn": 5, "actions": [{"type": "expand", "target": {"x": 20, "y": 4}}]}
```

### 3.4 Event (WebSocket event)

```json
{"type": "OBSERVATION", "turn": 5, "observation": {...}}
{"type": "ACTION_ACCEPTED", "turn": 5}
{"type": "TURN_RESOLVED", "turn": 5, "results": [...]}
{"type": "GAME_OVER", "turn": 200, "message": "Game over"}
{"type": "ERROR", "turn": 5, "message": "Error description"}
```

### 3.5 JoinResponse

```json
{"player_id": "p01", "token": "abc...", "websocket_url": "/ws"}
```

## 4. Игровой цикл (Game Loop)

### 4.1 Go-клиент: HTTP polling + WebSocket submit

Go-клиент использует **HTTP-опрос** для получения состояния и **WebSocket** только для отправки действий:

1. `Join` → регистрация, получение токена.
2. `GetObservation` (HTTP) → начальное состояние.
3. `ConnectWebSocket` → подключение к WS.
4. Цикл:
   - `GetObservation` (HTTP) → текущее состояние.
   - `strategy.Decide(obs)` → решение.
   - `SubmitActionWebSocket(conn, msg)` → отправка через WS.
   - HTTP polling (500ms) до изменения `turn`.
5. Reconnect: при потере WS → `GetObservation` → проверка `game_over` → `Join` (re-join) → `ConnectWebSocket`.

**Особенность:** Go-клиент не читает события из WebSocket (только отправляет действия). Состояние всегда берётся через HTTP.

### 4.2 Python-клиент: WebSocket events

Python-клиент использует **WebSocket events** для получения состояния:

1. `GameApiClient.register()` → регистрация.
2. `api_client.connect()` → подключение к WS.
3. `_wait_for_observation()` → ожидание первого OBSERVATION.
4. Цикл:
   - `strategy.decide(observation)` → решение.
   - `api_client.submit_action(msg)` → отправка через WS.
   - `_wait_for_observation()` → ожидание TURN_RESOLVED + OBSERVATION.
5. Reconnect: при потере WS → `api_client.connect()` → `_wait_for_observation()`.

**Особенность:** Python-клиент читает все события из WS (ACTION_ACCEPTED, TURN_RESOLVED, OBSERVATION, GAME_OVER, ERROR, STILL_ALIVE, CONNECTED).

### 4.3 C#-клиент: HTTP polling + WebSocket submit + response queue

C#-клиент использует **HTTP-опрос** для состояния, **WebSocket** для отправки действий и **очередь сообщений** для ответа сервера:

1. `JoinAsync` → регистрация.
2. `ConnectWebSocketAsync` → подключение к WS + запуск background read loop.
3. `GetObservationAsync` (HTTP) → начальное состояние.
4. Цикл:
   - `GetObservationAsync` (HTTP) → текущее состояние.
   - `strategy.Decide(observation)` → решение.
   - `SubmitActionAsync(message)` → отправка через WS.
   - `WaitForActionResponseAsync(turn)` → ожидание ACTION_ACCEPTED/ERROR из очереди.
   - HTTP polling (500ms) до изменения `turn`.
5. Reconnect: при потере WS → `ConnectWebSocketAsync` → `JoinAsync` (re-join) → `ConnectWebSocketAsync`.

**Особенность:** C#-клиент имеет background read loop, который буферизует все WS-сообщения в `ConcurrentQueue`. `WaitForActionResponseAsync` читает из очереди, ожидая ACTION_ACCEPTED или ERROR.

## 5. Интерфейсы стратегий

### 5.1 Go

```go
type Strategy interface {
    Decide(observation *models.Observation) *models.Action
}
```

Реализация: `SimpleStrategy` в `internal/strategy/simple.go`.

**Правило:** `Decide` возвращает `*models.Action` или `nil` (pass).

### 5.2 Python

```python
class IGameStrategy(ABC):
    @abstractmethod
    def decide(self, observation: Observation) -> Action:
        ...
```

Реализации: `SimpleStrategy`, `AggressiveStrategy` в `client/strategy.py`.

**Правило:** `decide` возвращает `Action` (type + optional target).

### 5.3 C#

```csharp
public interface IGameStrategy
{
    TerritoryWars.Models.Action Decide(Observation observation);
}
```

Реализация: `SimpleStrategy` в `Strategy/SimpleStrategy.cs`.

**Правило:** `Decide` возвращает `Action` или `pass`.

## 6. Конфигурация

### 6.1 Go-клиент

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

CLI-флаги: `--server`, `--name`, `--config`, `--log-level`.

### 6.2 Python-клиент

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

CLI-аргументы: `--server`, `--name`, `--config`, `--log-level`.

### 6.3 C#-клиент

```json
{
  "ServerUrl": "http://localhost:8080",
  "PlayerName": "StudentBot",
  "HttpTimeoutSeconds": 10,
  "Strategy": "Simple",
  "MaxRetries": 10,
  "LogsDir": "logs"
}
```

CLI-аргументы: `--config`, `--server`, `--name`, `--strategy`.

Env-переменные: `TERRITORYWARS_*` (префикс).

### 6.4 Приоритет конфигурации

**Правило:** CLI-флаги > config.json > значения по умолчанию.

## 7. Логирование

### 7.1 Go-клиент

`multiWriter` — дублирует `slog` вывод в stdout и log file.

Log file: `logs_dir/player_name.log`.

### 7.2 Python-клиент

`logging` module — file handler + basicConfig.

Log file: `logs_dir/player_name.log`.

### 7.3 C#-клиент

`Logger` (static class) — `StreamWriter` + `Console.WriteLine`.

Log file: `logs_dir/player_name.log`.

**Правило:** Все три клиента дублируют вывод в stdout и log file.

## 8. Обработка ошибок и переподключение

### 8.1 Общие принципы

1. Все три клиента поддерживают переподключение при потере WS-соединения.
2. При потере WS → проверка `game_over` через HTTP → re-join (новый токен) → reconnect.
3. `max_retries` (по умолчанию 10) — максимальное количество переподключений.
4. После re-join клиент продолжает игру с новым токеном.

### 8.2 Re-join

При `StopGame` сервер инвалидирует все токены. Клиент должен:

1. Заметить ошибку WS.
2. Проверить `game_over` через HTTP.
3. Если не game_over → `Join` с новым токеном.
4. Переподключиться к WS с новым токеном.

**Правило:** Все три клиента реализуют re-join. Go и C# проверяют строки ошибок (`registration is closed`, `game stopped`, `match is full`) для graceful exit.

### 8.3 WebSocket ping/pong

- Сервер отправляет `websocket.PingMessage` каждые 25 секунд.
- Go-клиент: `conn.SetPingHandler()` → `conn.WriteMessage(websocket.PongMessage, ...)`.
- Python-клиент: `websockets.connect(ping_interval=20, ping_timeout=10)`.
- C#-клиент: `ClientWebSocket` auto-responds to pings (но нужен background read loop).

**Правило:** Без pong-ответов сервер закроет соединение.

## 9. Тестирование

### 9.1 Python

```bash
cd twclients/python && uv run pytest tests/ -v
```

Тесты:
- `test_api.py` — URL construction (http→ws, https→wss, trailing slash, port).
- `test_models.py` — serialization/deserialization (Point, Cell, PublicPlayer, Observation, Action, ActionMessage, JoinResponse, Event).
- `test_strategy.py` — SimpleStrategy (empty cells, attack, expand, fortify, pass, defensive fallback).

### 9.2 C#

```bash
cd twclients/csharp && dotnet test
```

Тесты:
- `Tests/StrategyTests.cs` — Decide returns valid action type, handles empty cells, attack/expand/fortify/pass, defensive fallback, energy constraints.

### 9.3 Go

Go-клиент не имеет отдельных тестов. Стратегия `SimpleStrategy` тестируется косвенно через серверные интеграционные тесты.

**Правило:** Любое изменение стратегии должно сопровождаться тестами.

## 10. Кроссплатформенный запуск

### 10.1 Общие скрипты

- `run.sh` / `run.cmd` — запуск сервера + ботов + всех клиентов.
- `--no-go` / `--no-python` / `--no-csharp` — пропуск конкретных клиентов.
- `--no-bots` — сервер без ботов.
- `--force-start` — старт игры после регистрации.

### 10.2 Windows (run.cmd)

Все клиенты запускаются через `start "Name" /I cmd.exe /c "cd /d dir && command"`.

**Правило:** Все три клиента используют одинаковый паттерн запуска на Windows.

### 10.3 Linux/macOS (run.sh)

- Go: `go run ./cmd/client/ --config config.json --name Name`.
- Python: `uv run python main.py --config config.json --name Name`.
- C#: `dotnet run --project . -- --config config.json --name Name`.

## 11. Расхождения между клиентами

### 11.1 Получение состояния

| Клиент | Метод |
|--------|-------|
| Go | HTTP polling (`GetObservation`) |
| Python | WebSocket events (`_wait_for_observation`) |
| C# | HTTP polling (`GetObservationAsync`) |

**Правило:** Разные подходы к получению состояния — это фактическая архитектура, а не баг. Каждый клиент реализован по-своему.

### 11.2 Стратегии

| Клиент | Доступные стратегии |
|--------|---------------------|
| Go | `simple` (единственная) |
| Python | `simple`, `aggressive` |
| C# | `Simple` (единственная) |

**Примечание:** Python имеет `AggressiveStrategy`, но Go и C# — только `SimpleStrategy`.

### 11.3 Приоритет действий в SimpleStrategy

| Клиент | Приоритет |
|--------|-----------|
| Go | attack → expand → fortify → pass |
| Python | attack → expand → fortify → pass |
| C# | attack → expand → fortify → pass |

**Примечание:** Все три клиента имеют одинаковый приоритет в SimpleStrategy.

## 12. Известные ограничения

1. Go-клиент не читает WebSocket-события (только отправляет действия).
2. C#-клиент имеет background read loop, который может накапливать сообщения в очереди.
3. Python-клиент использует `websockets` библиотеку с `ping_interval=20`, что отличается от серверных 25 секунд.
4. C#-клиент использует `CamelCase` для JSON-сериализации (через `JsonNamingPolicy.CamelCase`), но сервер ожидает `snake_case`. Это работает, потому что C#-клиент отправляет `ActionMessage` с `CamelCase`, а сервер парсит его без учёта регистра.
5. Go-клиент использует `gorilla/websocket`, Python — `websockets`, C# — `System.Net.WebSockets.ClientWebSocket`. Разные библиотеки, разные API.
6. C#-клиент не имеет `AggressiveStrategy` (только `SimpleStrategy`).
7. Python-клиент имеет `AggressiveStrategy`, но она не доступна в Go и C# клиентах.
