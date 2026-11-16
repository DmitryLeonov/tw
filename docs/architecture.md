# Territory Wars — Архитектура системы

## 1. Обзор

Territory Wars — многопользовательская пошаговая игра за контроль территории на торической сетке 40×40. Сервер написан на Go; внешние клиенты доступны на Python, C# и Go. Студенты разрабатывают AI-стратегии для конкуренции друг с другом и встроенными ботами.

## 2. Структура проекта

```
tw/
├── AGENTS.md                    # Общие правила разработки
├── run.sh / run.cmd             # Скрипты запуска (сервер + боты + клиенты)
├── twserver/                    # Go-сервер
│   ├── AGENTS.md                # Правила серверной части
│   ├── cmd/server/main.go       # Точка входа
│   ├── internal/game/           # Игровой движок
│   ├── internal/server/         # HTTP, WS, RoundManager, боты, viewer
│   ├── config.json              # Конфигурация сервера
│   ├── build.sh / build.cmd     # Скрипты сборки
│   └── run.sh / run.cmd         # Скрипты запуска сервера
└── twclients/                   # Внешние клиенты
    ├── AGENTS.md                # Правила разработки клиентов
    ├── golang/                  # Go-клиент
    ├── python/                  # Python-клиент
    └── csharp/                  # C#-клиент
```

## 3. Архитектурные компоненты

### 3.1 Игровой движок (Engine)

**Файл:** `twserver/internal/game/engine.go`

Engine — единственный источник истины о состоянии игры. Он не знает о HTTP, WebSocket или клиентах. Все изменения состояния проходят через методы Engine.

**Основные методы:**

| Метод | Описание |
|-------|----------|
| `NewEngine(cfg Config)` | Создание нового движка |
| `Join(name string) (*Player, error)` | Регистрация игрока, возвращает ID + Token |
| `Authenticate(token string) (*Player, bool)` | Идентификация по сессии |
| `Submit(playerID string, action Action) error` | Приём действия |
| `ResolveTurn() ([]Result, error)` | Одновременное разрешение всех действий |
| `Observation(playerID string) (*Observation, error)` | Снимок состояния |
| `Leaderboard() []Result` | Таблица лидеров |
| `Results() []Result` | Результаты игры |
| `Status() map[string]any` | Статус игры |
| `Reset()` | Сброс состояния |
| `ClearTokens()` | Инвалидация всех токенов |
| `ClearPlayers()` | Очистка игроков |
| `SetConnected(playerID string, connected bool)` | Обновление статуса подключения |
| `TurnSignal() (<-chan struct{}, <-chan struct{})` | Сигнал о завершении хода |
| `GameOver() bool` | Проверка окончания игры |
| `Stopped() bool` | Проверка остановки |
| `ResetStopped()` | Сброс флага остановки |
| `ResetGameStarted()` | Сброс gameStarted флага |
| `Players() []Player` | Список всех игроков |

**Типы данных:**

- **Config:** `MaxPlayers`, `MaxTurns`, `TurnDelay`, `Width`, `Height`, `MarkRate`, `MarkThreshold`
- **Player:** `ID`, `Name`, `Token`, `X`, `Y`, `Energy`, `Score`, `Connected`, `Submitted`, `Eliminated`
- **Cell:** `X`, `Y`, `Owner`, `Health`, `Type` (`base`, `normal`, `resource`)
- **Observation:** `MatchID`, `PlayerID`, `Turn`, `GameOver`, `Width`, `Height`, `Energy`, `Score`, `Cells`, `Players`, `AvailableActions`, `ExpandTargets`, `AttackTargets`, `FortifyTargets`
- **Action:** `Type` (`expand`, `attack`, `fortify`, `pass`), `Target` (`Point`)
- **Point:** `X`, `Y`
- **Result:** `Turn`, `PlayerID`, `Action`, `Accepted`, `Reason`
- **PublicPlayer:** `ID`, `Name`, `X`, `Y`, `Score`, `Energy`, `Territory`, `Connected`, `Eliminated`

**Правила Engine:**

1. Все поля защищены `sync.RWMutex`.
2. `Token` — 48-символьный hex (randomHex(24) → crypto/rand).
3. `tokenToPlayer` map связывает токен с playerID.
4. `pending` map — действия, ожидающие разрешения.
5. `gameOver` и `stopped` флаги управляют жизненным циклом.
6. `turnChanged` channel пересоздаётся после каждого разрешения хода.

### 3.2 RoundManager

**Файл:** `twserver/internal/server/round_manager.go`

RoundManager — единственный координатор раундов. Один `run()` goroutine сериализует всю обработку действий и разрешение раундов.

**Основные методы:**

| Метод | Описание |
|-------|----------|
| `NewRoundManager(engine, gameStarted)` | Создание RoundManager |
| `Connect(conn, playerID) <-chan Event` | Подключение игрока, возвращает outbound channel |
| `Disconnect(playerID, generation)` | Отключение с проверкой generation |
| `Generation(playerID) int64` | Текущий generation игрока |
| `SubmitAction(playerID, msg, outbound)` | Отправка действия в event loop |
| `Pause()` / `Resume()` | Пауза / возобновление event loop |
| `Shutdown()` / `Stop()` / `Restart()` | Управление жизненным циклом |
| `Ready() <-chan struct{}` | Канал готовности event loop |
| `SetGameStarted(ch)` | Установка gameStarted канала |
| `SetResultsDir(dir)` | Директория для результатов |
| `SetGameOverHandler(fn)` | Обработчик окончания игры |

**Жизненный цикл события:**

1. **ACTION_ACCEPTED** — отправляется игроку, который отправил действие, сразу после принятия.
2. **TURN_RESOLVED** — транслируется всем подключённым клиентам при разрешении раунда.
3. **OBSERVATION** — отправляется каждому клиенту после TURN_RESOLVED.

Инвариант: `ACTION_ACCEPTED → TURN_RESOLVED → OBSERVATION`.

**Правила RoundManager:**

1. Один `run()` goroutine — единственный обработчик действий.
2. Per-player action channels (`actionCh`) — WS-хендлер пишет в канал, `run()` читает.
3. Generation counter — предотвращает обработку действий от старых соединений.
4. Bounded outbound queue (capacity 256) — переполнение → disconnect.
5. `cleanupAll()` — закрывает все WS-соединения при `StopGame`.

### 3.3 HTTP-сервер

**Файл:** `twserver/internal/server/server.go`

Сервер предоставляет REST API и WebSocket для взаимодействия с клиентами.

**Маршруты (регистрируются в `cmd/server/main.go`):**

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| `GET` | `/health` | `Health` | Нет |
| `POST` | `/api/v1/join` | `Join` | Нет |
| `GET` | `/api/v1/state` | `State` | `Bearer TOKEN` |
| `GET` | `/api/v1/leaderboard` | `Leaderboard` | Нет |
| `GET` | `/api/v1/results` | `Results` | Нет |
| `POST` | `/api/v1/start` | `Start` | Нет |
| `POST` | `/api/v1/stop` | `Stop` | Нет |
| `POST` | `/api/v1/pause` | `PauseHTTP` | Нет |
| `POST` | `/api/v1/resume` | `ResumeHTTP` | Нет |
| `GET` | `/ws` | `WebSocket` | `Bearer TOKEN` (query/header) |
| `GET` | `/`, `/viewer` | `ServeViewerHandler` | Нет |
| `GET` | `/api/v1/viewer/state` | `ServeState` | Нет |
| `GET` | `/api/v1/viewer/events` | `ServeEvents` | Нет |
| `GET` | `/api/v1/viewer/history` | `ServeHistory` | Нет |

**WebSocket:**

```
ws://localhost:8080/ws?token=YOUR_TOKEN
```

Токен передаётся через query parameter или `Authorization: Bearer` header.

WS-хендлер:
1. Аутентифицирует токен.
2. Upgrades соединение.
3. Устанавливает `Connected = true`.
4. Подключает к RoundManager.
5. Запускает goroutine для отправки событий (ping каждые 25 секунд).
6. Читает JSON-действия и передаёт в RoundManager.

**CORS:** Сервер устанавливает `Access-Control-Allow-Origin: *` для всех запросов.

### 3.4 Встроенные боты

**Файлы:** `twserver/internal/server/bot.go`, `bot_manager.go`, `bot_config.go`

**Интерфейс PlayerAgent:**

```go
type PlayerAgent interface {
    Decide(ctx context.Context, obs *game.Observation) (*game.Action, error)
    Name() string
    Strategy() string
}
```

**Стратегии:**

| Константа | Значение | Описание |
|-----------|----------|----------|
| `StrategyRandom` | `"random"` | Случайное действие |
| `StrategyAggressive` | `"aggressive"` | Приоритет атаки |
| `StrategyDefensive` | `"defensive"` | Приоритет защиты |
| `StrategyExpansion` | `"expansion"` | Приоритет расширения |
| `StrategySimple` | `"simple"` | Простая эвристика (определён, но не используется в BuildBotList) |

**BotManager:**

- `NewBotManager(engine, rm, agents)` — создание с предсобранным списком агентов.
- `StartBotsOnDemand(count)` — запуск ботов по требованию (не автоматически).
- `Shutdown()` — остановка всех ботов.
- `ResetContext()` — сброс контекста для нового раунда.
- `Count()` — количество запущенных ботов.
- `HumanPlayerCount()` — количество внешних игроков.
- `LogEntries()` — записи истории ботов для Viewer.
- `BotIDs()` — идентификаторы ботов.

**Правила ботов:**

1. Боты НЕ запускаются автоматически при старте сервера.
2. Боты запускаются через `StartBotsOnDemand()` при вызове `StartGame()`.
3. `BuildBotList(enabled, maxPlayers)` — создаёт агентов с ротацией стратегий и уникальными seed (base=42).
4. Имена ботов: `Bot-aggressive-1`, `Bot-defensive-2`, `Bot-expansion-3`, `Bot-random-4`, `Bot-simple-5`.
5. Боты реализуют `PlayerAgent`, а не `IGameStrategy`.

### 3.5 Viewer (HTTP-визуализация)

**Файл:** `twserver/internal/server/viewer.go`

Viewer — HTTP-сервис с встроенным HTML-интерфейсом (через `//go:embed viewer.html`).

**Эндпоинты:**

| Path | Описание |
|------|----------|
| `/`, `/viewer` | HTML-страница |
| `/api/v1/viewer/state` | Текущее состояние игры (JSON) |
| `/api/v1/viewer/events` | SSE-стриминг (`text/event-stream`) |
| `/api/v1/viewer/history` | История действий ботов (JSON) |

**SSE-события:**

| Событие | Описание |
|---------|----------|
| `state` | Полный снимок состояния игры |
| `history` | История действий ботов |

**Правила Viewer:**

1. `viewer.html` встраивается через `//go:embed viewer.html`.
2. SSE использует `EventSource` в браузере.
3. `StartMonitoring(ctx)` — фоновый goroutine, транслирующий обновления каждые 5 секунд.
4. Viewer не требует аутентификации.
5. `buildViewerState()` собирает данные из Engine и BotManager.

## 4. Внешние клиенты

**Директория:** `twclients/`

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

### 4.1 Go-клиент

**Структура:** `twclients/golang/`

- `cmd/client/main.go` — точка входа
- `internal/api/client.go` — HTTP + WebSocket API
- `internal/config/config.go` — загрузка конфигурации
- `internal/game/loop.go` — игровой цикл
- `internal/models/models.go` — модели данных
- `internal/strategy/strategy.go` — интерфейс Strategy
- `internal/strategy/simple.go` — простая стратегия

**Особенность:** Go-клиент использует **HTTP-опрос** для получения состояния и **WebSocket** только для отправки действий. Не читает события из WebSocket.

### 4.2 Python-клиент

**Структура:** `twclients/python/`

- `main.py` — точка входа
- `client/api.py` — HTTP + WebSocket API
- `client/models.py` — модели данных
- `client/strategy.py` — IGameStrategy + стратегии
- `client/game_loop.py` — игровой цикл

**Особенность:** Python-клиент использует **WebSocket events** для получения состояния. Читает все события из WS (ACTION_ACCEPTED, TURN_RESOLVED, OBSERVATION, GAME_OVER, ERROR, STILL_ALIVE, CONNECTED).

### 4.3 C#-клиент

**Структура:** `twclients/csharp/`

- `Program.cs` — точка входа
- `Api/GameApiClient.cs` — HTTP + WebSocket API
- `Game/GameLoop.cs` — игровой цикл
- `Models/` — модели данных
- `Strategy/IGameStrategy.cs` — интерфейс стратегии
- `Strategy/SimpleStrategy.cs` — простая стратегия
- `Logger.cs` — dual writer (Console + file)

**Особенность:** C#-клиент имеет background read loop, который буферизует все WS-сообщения в `ConcurrentQueue`. `WaitForActionResponseAsync` читает из очереди, ожидая ACTION_ACCEPTED или ERROR.

## 5. Жизненный цикл игры

### 5.1 Состояния

1. **Stopped** — сервер запущен, регистрация открыта, игра не началась.
2. **Started** — `StartGame()` вызван, боты запущены, ходы идут.
3. **Paused** — `Pause()` вызван, event loop заблокирован.
4. **GameOver** — `MAX_TURNS` достигнут или все игроки eliminated.
5. **Reset** — `StopGame()` вызван, состояние сброшено, регистрация открыта.

### 5.2 StopGame

Полная последовательность остановки:

1. `engine.ClearTokens()` — инвалидация всех токенов.
2. `rounds.cleanupAll()` — закрытие всех WS-соединений.
3. `shutdownCancel()` — остановка Viewer monitoring.
4. `rounds.Stop()` — остановка event loop.
5. `engine.Reset()` — сброс состояния.
6. `engine.ClearPlayers()` — очистка игроков.
7. `ResetGameStarted()` — пересоздание gameStarted канала.
8. `botManager.Shutdown()` — остановка ботов.
9. `rounds.Restart()` — перезапуск event loop.
10. `engine.ResetStopped()` — разрешение новой регистрации.
11. Пересоздание shutdown context + restart Viewer monitoring.

### 5.3 Правила жизненного цикла

1. Игра НЕ начинается автоматически.
2. `StartGame()` запускает ботов для заполнения оставшихся слотов.
3. `StopGame()` сбрасывает ВСЁ: токены, соединения, состояние, ботов.
4. После `StopGame` клиенты должны заново пройти `Join` для получения нового токена.
5. `gameOverHandler` — после окончания игры сервер ждёт Enter для остановки.

## 6. Конфигурация

### 6.1 Сервер (GameConfig)

```json
{
  "port": 8080,
  "max_players": 30,
  "max_turns": 200,
  "width": 40,
  "height": 40,
  "turn_delay": "1s",
  "results_dir": "results",
  "logs_dir": "logs",
  "mark_rate": 10,
  "mark_threshold": 5,
  "bots_enabled": true
}
```

**Env-переменные:** `PORT`, `MAX_PLAYERS`, `MAX_TURNS`, `TURN_TIMEOUT_SECONDS` (не используется).

### 6.2 Клиенты

**Go/Python:**

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

**C#:**

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

**Приоритет:** CLI-флаги > config.json > значения по умолчанию.

## 7. Логирование

- Сервер: `logs/server.log` (контролируется `logs_dir` в `config.json`).
- Клиенты: stdout + log file в `logs_dir/player_name.log`.
- `logs/` — единственная директория для логов (не `.logs`).
- Директория `logs/` создаётся автоматически при запуске.

## 8. Игровая механика

- Карта: 40×40, торическая (координаты wrap-around через `% Width` / `% Height`).
- 30 позиций спавна (до `max_players`).
- Типы клеток: `base` (старт), `normal`, `resource`.
- Каждый игрок стартует с 5 energy и одной `base`.
- Одно действие за ход. Все действия разрешаются одновременно.
- Пропущенное действие → `pass` по таймауту.
- Матч заканчивается после `MAX_TURNS` (по умолчанию 200).
- Energy: старт 5, +1 за ход (больше с resource-клетками), макс 20.
- Health клеток: normal старт 3, bases старт 5, макс 7.

## 9. Тестирование

### 9.1 Сервер (Go)

```bash
cd twserver && go test -race -v ./...
```

| Файл | Что тестирует |
|------|--------------|
| `engine_test.go` | Join, Authenticate, Submit, Resolve, Timeout, Observation, Leaderboard, GameOver |
| `round_manager_test.go` | Connect, Disconnect, Generation, Action submission, Event ordering, Shutdown, Bounded queues, Concurrent submits |
| `integration_test.go` | Реальные WS-клиенты через `httptest.Server`, action flow, timeout resolution, reconnect, event ordering |
| `bot_test.go` | CreateBot, все стратегии, BuildBotList, seed uniqueness, decision timeout |
| `bot_config_test.go` | LoadConfig, defaults, overrides, invalid JSON, file not found, negative values, relative/absolute paths |
| `bot_manager_test.go` | StartBotsOnDemand, exceeds max players, Shutdown, LogEntries, concurrent start/shutdown |
| `viewer_test.go` | ServeViewer, ServeState, ServeHistory, ServeEvents (SSE), routing, buildViewerState, client removal |

### 9.2 Клиенты (Python)

```bash
cd twclients/python && uv run pytest tests/ -v
```

| Файл | Что тестирует |
|------|--------------|
| `test_api.py` | URL construction (http→ws, https→wss, trailing slash, port) |
| `test_models.py` | Serialization/deserialization (Point, Cell, PublicPlayer, Observation, Action, ActionMessage, JoinResponse, Event) |
| `test_strategy.py` | SimpleStrategy (empty cells, attack, expand, fortify, pass, defensive fallback) |

### 9.3 Клиенты (C#)

```bash
cd twclients/csharp && dotnet test
```

| Файл | Что тестирует |
|------|--------------|
| `Tests/StrategyTests.cs` | Decide returns valid action type, handles empty cells, attack/expand/fortify/pass, defensive fallback, energy constraints |

## 10. Расхождения между клиентами

### 10.1 Получение состояния

| Клиент | Метод |
|--------|-------|
| Go | HTTP polling (`GetObservation`) |
| Python | WebSocket events (`_wait_for_observation`) |
| C# | HTTP polling (`GetObservationAsync`) |

### 10.2 Стратегии

| Клиент | Доступные стратегии |
|--------|---------------------|
| Go | `simple` (единственная) |
| Python | `simple`, `aggressive` |
| C# | `Simple` (единственная) |

### 10.3 Приоритет действий в SimpleStrategy

Все три клиента имеют одинаковый приоритет: attack → expand → fortify → pass (с defensive fallback при weakest_health <= 3).

## 11. Известные ограничения MVP

1. Регистрация открыта и возвращает токены публично. Для экзамена — предсоздать список или защитить admin key.
2. `CheckOrigin` и CORS пермиссивны для локальной разработки.
3. Нет persistence, TLS, rate limiting, replay storage, admin authentication.
4. Наблюдение раскрывает все клетки. Добавить fog-of-war при необходимости.
5. Текущий ruleset намеренно мал. Нужен playtesting для баланса.
6. Round coordinator использует bounded outbound queue. Клиент, который не потребляет события, disconnects.
