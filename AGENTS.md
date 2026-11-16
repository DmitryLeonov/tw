# Territory Wars — Общие правила разработки

## 1. Архитектурные принципы

### 1.1 Единый игровой движок

Игровой движок (`twserver/internal/game/engine.go`) — единственный источник истины о состоянии игры. Все изменения состояния проходят через методы Engine:

- `Join(name)` — регистрация игрока, возвращает `Player` с уникальным `ID` и `Token`.
- `Authenticate(token)` — идентификация по сессии (один `Join` → токен → все последующие запросы).
- `Submit(playerID, action)` — приём действия от игрока.
- `ResolveTurn()` — одновременное разрешение всех действий раунда.
- `Observation(playerID)` — получение снимка состояния для игрока.
- `Reset()` — сброс состояния после `StopGame`.

**Правило:** Движок не знает о HTTP, WebSocket или клиентах. Он оперирует только типами `game.Config`, `game.Player`, `game.Cell`, `game.Observation`, `game.Action`, `game.Result`.

### 1.2 Единый механизм разрешения ходов

`RoundManager` (`twserver/internal/server/round_manager.go`) — единственный координатор раундов. Архитектура:

- Один `run()` goroutine сериализует всю обработку действий и разрешение раундов.
- Каждый игрок имеет свой per-player action channel (`actionCh`).
- WS-хендлер пишет действия в этот канал вместо прямого вызова Engine.
- `run()` читает действия, вызывает `engine.Submit()`, очередь `ACTION_ACCEPTED`, мониторит `engine.TurnSignal()`.

**Правило:** Никогда не вызывайте `engine.Submit()` или `engine.ResolveTurn()` напрямую из HTTP-хендлеров или WS-хендлеров. Всегда используйте `RoundManager.SubmitAction()`.

### 1.3 Различия между встроенными и внешними ботами

| Характеристика | Встроенные боты | Внешние клиенты |
|---|---|---|
| Язык | Go (в `twserver/internal/server/bot.go`) | Python, C#, Go |
| Связь | `PlayerAgent` интерфейс (без HTTP) | HTTP + WebSocket |
| Создание | `BotManager.StartBotsOnDemand()` | `POST /api/v1/join` |
| Стратегии | `random`, `aggressive`, `defensive`, `expansion` | `simple`, `aggressive` (Python), `Simple` (C#) |
| Интерфейс | `Decide(ctx, *game.Observation)` | `IGameStrategy.Decide(observation)` |
| Логирование | `botLogger` → `Viewer.history` | `Logger` → stdout + log file |

**Правило:** Встроенные боты реализуют `PlayerAgent` интерфейс в сервере. Внешние клиенты реализуют `IGameStrategy` (Python/C#/Go). Это разные интерфейсы с разными сигнатурами.

### 1.4 Контракт REST API

Все маршруты префиксированы `/api/v1/`:

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/health` | Health check | Нет |
| `POST` | `/api/v1/join` | Регистрация (`{"name":"..."}`) | Нет |
| `GET` | `/api/v1/state` | Наблюдение | `Bearer TOKEN` |
| `GET` | `/api/v1/leaderboard` | Таблица лидеров | Нет |
| `GET` | `/api/v1/results` | Результаты (после game over) | Нет |
| `POST` | `/api/v1/start` | Старт игры | Нет |
| `POST` | `/api/v1/stop` | Стоп, сброс, отключение | Нет |
| `POST` | `/api/v1/pause` | Пауза | Нет |
| `POST` | `/api/v1/resume` | Возобновление | Нет |

**Правило:** Все JSON-поля используют `snake_case`. Ответы сервера — `application/json`.

### 1.5 Модели Observation и Action

**Observation** (полный снимок состояния):
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

**Action** (одно действие за ход):
```json
{"type": "expand", "target": {"x": 20, "y": 4}}
```

Допустимые типы: `expand` (1 energy), `attack` (2 energy), `fortify` (2 energy), `pass` (0).

**Правило:** Все три клиента (Go, Python, C#) должны использовать одинаковые JSON-поля. Изменения в `types.go` требуют синхронного обновления всех клиентов.

### 1.6 Синхронизация и конкурентность

- **Engine:** `sync.RWMutex` защищает все поля состояния.
- **RoundManager:** Один `run()` goroutine сериализует обработку. Per-player action channels.
- **WS-хендлер:** `defer` закрывает соединение, вызывает `Disconnect()` с generation-счётчиком.
- **Generation:** Монотонно увеличивается при каждом переподключении. Старые соединения игнорируются.
- **Outbound queue:** Бounded channel (capacity 256). Переполнение → disconnect.
- **Пинги:** Сервер отправляет `websocket.PingMessage` каждые 25 секунд. Клиент должен отвечать `PongMessage`.

**Правило:** Не добавляйте мьютексы в Engine без крайней необходимости. Все изменения состояния должны проходить через `run()` RoundManager.

### 1.7 Обратная совместимость клиентов

- Все три клиента используют один и тот же HTTP-протокол и JSON-контракт.
- Клиенты не требуют изменений на сервере для добавления новых стратегий.
- При `StopGame` все токены инвалидируются (`ClearTokens()`). Клиенты переподключаются через новый `Join`.
- `generation` counter предотвращает обработку действий от старых соединений.

**Правило:** Новые клиенты (новый язык) должны:
1. Реализовать `Join` → `GetObservation` → `SubmitAction` через WebSocket.
2. Использовать `Bearer TOKEN` для `GET /api/v1/state`.
3. Обрабатывать события: `CONNECTED`, `OBSERVATION`, `ACTION_ACCEPTED`, `TURN_RESOLVED`, `GAME_OVER`.

### 1.8 Тестирование

**Сервер (Go):**
```bash
cd twserver && go test -race -v ./...
```

Тесты охватывают:
- `internal/game/engine_test.go` — Join, Authenticate, Submit, Resolve, Timeout, Observation, Leaderboard, GameOver.
- `internal/server/round_manager_test.go` — Connect, Disconnect, Generation, Action submission, Event ordering, Shutdown, Bounded queues, Concurrent submits.
- `internal/server/integration_test.go` — Реальные WS-клиенты через `httptest.Server`, action flow, timeout resolution, reconnect.
- `internal/server/bot_test.go` — CreateBot, все стратегии, BuildBotList.
- `internal/server/bot_config_test.go` — LoadConfig, defaults, overrides, invalid JSON.
- `internal/server/bot_manager_test.go` — StartBotsOnDemand, Shutdown, LogEntries.
- `internal/server/viewer_test.go` — ServeViewer, ServeState, ServeHistory, ServeEvents (SSE).

**Клиенты (Python):**
```bash
cd twclients/python && uv run pytest tests/ -v
```

**Клиенты (C#):**
```bash
cd twclients/csharp && dotnet test
```

**Правило:** Любое изменение серверной логики должно сопровождаться тестами. Интеграционные тесты проверяют полный цикл: Join → WS connect → Submit action → TURN_RESOLVED → OBSERVATION.

### 1.9 Правила изменения документации

- `README.md` / `README.rus.md` — пользовательская документация (API, запуск, механика).
- `AGENTS.md` — инструкции для разработчиков (архитектура, правила, контракты).
- Документация должна отражать **фактическое** состояние кода, а не предполагаемое.
- При изменении API, типов или маршрутов — обновлять все соответствующие README и AGENTS файлы.
- Русские версии (`*.rus.md`) должны синхронизироваться с английскими.

## 2. Структура проекта

```
tw/
├── AGENTS.md                    # Общие правила (этот файл)
├── run.sh                       # Запуск (Linux/macOS)
├── run.cmd                      # Запуск (Windows)
├── twserver/                    # Go-сервер
│   ├── AGENTS.md                # Правила серверной части
│   ├── cmd/server/main.go       # Точка входа
│   ├── internal/game/           # Игровой движок
│   ├── internal/server/         # HTTP, WS, RoundManager, боты, viewer
│   └── config.json              # Конфигурация сервера
└── twclients/                   # Внешние клиенты
    ├── AGENTS.md                # Правила разработки клиентов
    ├── golang/                  # Go-клиент
    ├── python/                  # Python-клиент
    └── csharp/                  # C#-клиент
```

## 3. Запуск и сборка

- `run.sh` / `run.cmd` — НЕ собирают код автоматически. Сборка отдельная через `build.sh` / `build.cmd`.
- `run.sh` запускает сервер + боты + всех внешних клиентов.
- `run.sh --no-bots` — сервер без ботов (только внешние клиенты).
- `run.sh --no-go` / `--no-python` / `--no-csharp` — пропуск конкретных клиентов.
- `run.sh --force-start` — старт игры после регистрации клиентов.

## 4. Логирование

- Сервер: `logs/server.log` (контролируется `logs_dir` в `config.json`).
- Клиенты: stdout + log file в `logs_dir/player_name.log`.
- `logs/` — единственная директория для логов (не `.logs`).
- Директория `logs/` создаётся автоматически при запуске.

## 5. Конфигурация

- Сервер: `twserver/config.json` — `port`, `max_players`, `max_turns`, `width`, `height`, `turn_delay`, `results_dir`, `logs_dir`, `mark_rate`, `mark_threshold`, `bots_enabled`.
- Клиенты: `config.json` — `server`/`ServerUrl`, `player_name`/`PlayerName`, `http_timeout`/`HttpTimeoutSeconds`, `strategy`/`Strategy`, `max_retries`/`MaxRetries`, `logs_dir`/`LogsDir`.
- CLI-флаги переопределяют значения из `config.json`.
- Env-переменные (`PORT`, `MAX_PLAYERS`, `MAX_TURNS`) переопределяют `config.json` на сервере.

## 6. Игровая механика

- Карта: 40×40, торическая (координаты wrap-around через `% Width` / `% Height`).
- 30 позиций спавна (до `max_players`).
- Типы клеток: `base` (старт), `normal`, `resource`.
- Каждый игрок стартует с 5 energy и одной `base`.
- Одно действие за ход. Все действия разрешаются одновременно.
- Пропущенное действие → `pass` по таймауту.
- Матч заканчивается после `MAX_TURNS` (по умолчанию 200).
- Energy: старт 5, +1 за ход (больше с resource-клетками), макс 20.
- Health клеток: normal старт 3, bases старт 5, макс 7.
