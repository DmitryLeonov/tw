# Territory Wars — Правила серверной части

## 1. Структура сервера

```
twserver/
├── cmd/server/main.go           # Точка входа, HTTP-маршруты, multiWriter
├── internal/game/
│   ├── engine.go                # Игровой движок (state, actions, resolution)
│   ├── engine_test.go           # Юнит-тесты движка
│   └── types.go                 # Типы: Config, Player, Cell, Observation, Action, Result
├── internal/server/
│   ├── server.go                # HTTP handlers, WebSocket, Server struct
│   ├── round_manager.go         # Event loop, action processing, round coordination
│   ├── round_manager_test.go    # Тесты RoundManager
│   ├── bot.go                   # Встроенные боты: random, aggressive, defensive, expansion
│   ├── bot_test.go              # Тесты ботов
│   ├── bot_manager.go           # Управление жизненным циклом ботов
│   ├── bot_manager_test.go      # Тесты BotManager
│   ├── bot_config.go            # GameConfig, BotStrategy, LoadConfig, BuildBotList
│   ├── bot_config_test.go       # Тесты конфигурации
│   ├── console.go               # Интерактивная консоль (stdin)
│   ├── viewer.go                # HTTP viewer + SSE
│   ├── viewer.html              # Встроенный HTML viewer (embed)
│   ├── viewer_test.go           # Тесты viewer
│   └── integration_test.go      # Интеграционные тесты (httptest.Server + WS)
├── config.json                  # Конфигурация сервера
├── build.sh / build.cmd         # Скрипты сборки
└── run.sh / run.cmd             # Скрипты запуска
```

## 2. Игровой движок (Engine)

### 2.1 Ответственность

Engine — единственный источник истины о состоянии игры. Он не знает о HTTP, WebSocket или клиентах.

### 2.2 Основные методы

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
| `TurnSignal() <-chan struct{}` | Сигнал о завершении хода |
| `GameOver() bool` | Проверка окончания игры |
| `Stopped() bool` | Проверка остановки |
| `ResetStopped()` | Сброс флага остановки |
| `ResetGameStarted()` | Сброс gameStarted флага |

### 2.3 Типы данных

**Config:** `MaxPlayers`, `MaxTurns`, `TurnDelay`, `Width`, `Height`, `MarkRate`, `MarkThreshold`.

**Player:** `ID`, `Name`, `Token`, `X`, `Y`, `Energy`, `Score`, `Connected`, `Submitted`, `Eliminated`.

**Cell:** `X`, `Y`, `Owner`, `Health`, `Type` (`base`, `normal`, `resource`).

**Observation:** `MatchID`, `PlayerID`, `Turn`, `GameOver`, `Width`, `Height`, `Energy`, `Score`, `Cells`, `Players`, `AvailableActions`.

**Action:** `Type` (`expand`, `attack`, `fortify`, `pass`), `Target` (`Point`).

**Result:** `Turn`, `PlayerID`, `Action`, `Accepted`, `Reason`.

**Event:** `Type` (`CONNECTED`, `OBSERVATION`, `ACTION_ACCEPTED`, `TURN_RESOLVED`, `GAME_OVER`, `ERROR`), `Turn`, `Observation`, `Results`, `Message`.

**ActionMessage:** `Type` (`ACTION`), `Turn`, `Actions` ([]Action).

### 2.4 Правила Engine

1. Все поля защищены `sync.RWMutex`.
2. `Token` — 48-символьный hex (randomHex(24) → crypto/rand).
3. `tokenToPlayer` map связывает токен с playerID.
4. `pending` map — действия, ожидающие разрешения.
5. `gameOver` и `stopped` флаги управляют жизненным циклом.
6. `turnChanged` channel пересоздаётся после каждого разрешения хода.

## 3. RoundManager

### 3.1 Архитектура

Один `run()` goroutine сериализует всю обработку действий и разрешение раундов. Это предотвращает data races и устраняет баги двойного разрешения.

### 3.2 Основные методы

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

### 3.3 Жизненный цикл события

Каждый ход проходит строгую последовательность:

1. **ACTION_ACCEPTED** — отправляется игроку, который отправил действие, сразу после принятия.
2. **TURN_RESOLVED** — транслируется всем подключённым клиентам при разрешении раунда (все игроки отправили действия или таймаут истёк). Включает per-player результаты.
3. **OBSERVATION** — отправляется каждому клиенту после TURN_RESOLVED, показывает состояние доски для следующего хода.

Инвариант: `ACTION_ACCEPTED → TURN_RESOLVED → OBSERVATION`.

### 3.4 Правила RoundManager

1. Один `run()` goroutine — единственный обработчик действий.
2. Per-player action channels (`actionCh`) — WS-хендлер пишет в канал, `run()` читает.
3. Generation counter — предотвращает обработку действий от старых соединений.
4. Bounded outbound queue (capacity 256) — переполнение → disconnect.
5. `cleanupAll()` — закрывает все WS-соединения при `StopGame`.

## 4. Встроенные боты

### 4.1 Интерфейс PlayerAgent

```go
type PlayerAgent interface {
    Decide(ctx context.Context, obs *game.Observation) (*game.Action, error)
    Name() string
    Strategy() string
}
```

### 4.2 Стратегии

| Константа | Значение | Описание |
|-----------|----------|----------|
| `StrategyRandom` | `"random"` | Случайное действие |
| `StrategyAggressive` | `"aggressive"` | Приоритет атаки |
| `StrategyDefensive` | `"defensive"` | Приоритет защиты |
| `StrategyExpansion` | `"expansion"` | Приоритет расширения |

**Примечание:** `StrategySimple` определён в `bot_config.go` но не используется в `BuildBotList`.

### 4.3 BotManager

- `NewBotManager(engine, rm, agents)` — создание с предсобранным списком агентов.
- `StartBotsOnDemand(count)` — запуск ботов по требованию (не автоматически).
- `Shutdown()` — остановка всех ботов.
- `ResetContext()` — сброс контекста для нового раунда.
- `Count()` — количество запущенных ботов.
- `HumanPlayerCount()` — количество внешних игроков.
- `LogEntries()` — записи истории ботов для Viewer.

### 4.4 Правила ботов

1. Боты НЕ запускаются автоматически при старте сервера.
2. Боты запускаются через `StartBotsOnDemand()` при вызове `StartGame()`.
3. `BuildBotList(enabled, maxPlayers)` — создаёт агентов с ротацией стратегий и уникальными seed (base=42).
4. Имена ботов: `Bot-aggressive-1`, `Bot-defensive-2`, `Bot-expansion-3`, `Bot-random-4`, `Bot-simple-5`.
5. Боты реализуют `PlayerAgent`, а не `IGameStrategy`.

## 5. HTTP-сервер

### 5.1 Маршруты

Все маршруты регистрируются в `cmd/server/main.go`:

```go
mux.HandleFunc("/health", srv.Health)
mux.HandleFunc("/api/v1/join", srv.Join)
mux.HandleFunc("/api/v1/state", srv.State)
mux.HandleFunc("/api/v1/leaderboard", srv.Leaderboard)
mux.HandleFunc("/api/v1/results", srv.Results)
mux.HandleFunc("/api/v1/start", srv.Start)
mux.HandleFunc("/api/v1/stop", srv.Stop)
mux.HandleFunc("/api/v1/pause", srv.PauseHTTP)
mux.HandleFunc("/api/v1/resume", srv.ResumeHTTP)
mux.HandleFunc("/ws", srv.WebSocket)
mux.HandleFunc("/", viewer.ServeViewerHandler)
mux.HandleFunc("/viewer", viewer.ServeViewerHandler)
mux.HandleFunc("/api/v1/viewer/", viewer.ServeViewerHandler)
```

### 5.2 WebSocket

```
ws://localhost:8080/ws?token=YOUR_TOKEN
```

Токен передаётся через query parameter или `Authorization: Bearer` header.

WS-хендлер:
1. Аутентифицирует токен.
2. Upgrades соединение.
3. Устанавливает `Connected = true`.
4. Подключает к RoundManager.
5. Запускает goroutine для отправки событий (с ping каждые 25 секунд).
6. Читает JSON-действия и передаёт в RoundManager.

### 5.3 CORS

Сервер устанавливает `Access-Control-Allow-Origin: *` для всех запросов. Это допустимо только для локальной разработки.

### 5.4 Правила HTTP

1. Все POST-эндпоинты проверяют `r.Method != http.MethodPost`.
2. `writeJSON()` — единая функция для JSON-ответов.
3. `bearer(r)` — извлечение токена из `Authorization` header.
4. `http.MaxBytesReader(w, r.Body, 4096)` — ограничение размера тела запроса.
5. `signal.Ignore(syscall.SIGINT, syscall.SIGTERM)` — сервер игнорирует сигналы для выживания при kill harness.

## 6. Viewer

### 6.1 Архитектура

Viewer — HTTP-сервис с встроенным HTML-интерфейсом (через `//go:embed viewer.html`).

- `ServeViewerHandler` — обслуживает `/`, `/viewer`, `/api/v1/viewer/*`.
- `ServeState` — `GET /api/v1/viewer/state` — текущее состояние игры.
- `ServeHistory` — `GET /api/v1/viewer/history` — история действий ботов.
- `ServeEvents` — `GET /api/v1/viewer/events` — SSE-стриминг (`text/event-stream`).

### 6.2 SSE-события

| Событие | Описание |
|---------|----------|
| `state` | Полный снимок состояния игры |
| `history` | История действий ботов |
| `error` | Ошибка подключения |
| `stale` | Устаревшие данные |

### 6.3 Правила Viewer

1. `viewer.html` встраивается через `//go:embed viewer.html`.
2. SSE использует `EventSource` в браузере.
3. `StartMonitoring(ctx)` — фоновый goroutine, транслирующий обновления.
4. Viewer не требует аутентификации.
5. `buildViewerState()` собирает данные из Engine и BotManager.

## 7. Конфигурация

### 7.1 GameConfig

```go
type GameConfig struct {
    Port          int    `json:"port"`
    MaxPlayers    int    `json:"max_players"`
    MaxTurns      int    `json:"max_turns"`
    Width         int    `json:"width"`
    Height        int    `json:"height"`
    TurnDelay     string `json:"turn_delay"`
    ResultsDir    string `json:"results_dir"`
    LogsDir       string `json:"logs_dir"`
    MarkRate      int    `json:"mark_rate"`
    MarkThreshold int    `json:"mark_threshold"`
    BotsEnabled   bool   `json:"bots_enabled"`
}
```

### 7.2 LoadConfig

- Загружает JSON из файла.
- Применяет значения по умолчанию: `port=8080`, `max_players=20`, `max_turns=200`.
- Отрицательные значения заменяются на значения по умолчанию.
- Относительные пути (`results_dir`, `logs_dir`) конвертируются в абсолютные относительно config file.
- Абсолютные пути остаются без изменений.

### 7.3 Env-переменные

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `PORT` | `8080` | HTTP порт |
| `MAX_PLAYERS` | `20` | Максимум игроков |
| `MAX_TURNS` | `200` | Максимум ходов |
| `TURN_TIMEOUT_SECONDS` | `15` | Таймаут хода (не используется, turn_delay в config) |

**Правило:** Env-переменные переопределяют `config.json`. CLI-флаг `-config` переопределяет использование env.

## 8. Жизненный цикл игры

### 8.1 Состояния

1. **Stopped** — сервер запущен, регистрация открыта, игра не началась.
2. **Started** — `StartGame()` вызван, боты запущены, ходы идут.
3. **Paused** — `Pause()` вызван, event loop заблокирован.
4. **GameOver** — `MAX_TURNS` достигнут или все игроки eliminated.
5. **Reset** — `StopGame()` вызван, состояние сброшено, регистрация открыта.

### 8.2 StopGame

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

### 8.3 Правила жизненного цикла

1. Игра НЕ начинается автоматически.
2. `StartGame()` запускает ботов для заполнения оставшихся слотов.
3. `StopGame()` сбрасывает ВСЁ: токены, соединения, состояние, ботов.
4. После `StopGame` клиенты должны заново пройти `Join` для получения нового токена.
5. `gameOverHandler` — после окончания игры сервер ждёт Enter для остановки.

## 9. Тестирование сервера

### 9.1 Запуск

```bash
cd twserver && go test -race -v ./...
```

### 9.2 Категории тестов

| Файл | Что тестирует |
|------|--------------|
| `engine_test.go` | Join, Authenticate, Submit, Resolve, Timeout, Observation, Leaderboard, GameOver |
| `round_manager_test.go` | Connect, Disconnect, Generation, Action submission, Event ordering, Shutdown, Bounded queues, Concurrent submits |
| `integration_test.go` | Реальные WS-клиенты через `httptest.Server`, action flow, timeout resolution, reconnect, event ordering |
| `bot_test.go` | CreateBot, все стратегии, BuildBotList, seed uniqueness, decision timeout |
| `bot_config_test.go` | LoadConfig, defaults, overrides, invalid JSON, file not found, negative values, relative/absolute paths |
| `bot_manager_test.go` | StartBotsOnDemand, exceeds max players, Shutdown, LogEntries, concurrent start/shutdown |
| `viewer_test.go` | ServeViewer, ServeState, ServeHistory, ServeEvents (SSE), routing, buildViewerState, client removal |

### 9.3 Правила тестирования

1. Все тесты должны проходить с `-race`.
2. Интеграционные тесты используют `httptest.Server` — без реального HTTP-сервера.
3. Тесты RoundManager не используют реальные WS-соединения (conn=nil).
4. Тесты ботов проверяют все стратегии и edge cases.
5. Тесты конфигурации проверяют defaults, overrides, invalid input, path resolution.

## 10. Логирование

- `multiWriter` в `main.go` — дублирует вывод в stdout и log file.
- Log file: `logs_dir/server.log` (относительный путь из `config.json`).
- `os.MkdirAll(cfg.LogsDir, 0755)` — создание директории логов.
- `log.SetOutput(mw)` — перенаправление стандартного logger.

**Правило:** Все логи сервера идут через `log.Printf` / `log.Fatalf`. Не используйте `fmt.Println` для логов.

## 11. Известные ограничения MVP

1. Регистрация открыта и возвращает токены публично. Для экзамена — предсоздать список или защитить admin key.
2. `CheckOrigin` и CORS пермиссивны для локальной разработки.
3. Нет persistence, TLS, rate limiting, replay storage, admin authentication.
4. Наблюдение раскрывает все клетки. Добавить fog-of-war при необходимости.
5. Текущий ruleset намеренно мал. Нужен playtesting для баланса.
6. Round coordinator использует bounded outbound queue. Клиент, который не потребляет события, disconnects.
