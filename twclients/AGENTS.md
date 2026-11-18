# Territory Wars — Правила разработки внешних клиентов

## 1. Назначение

Каталог содержит эталонные внешние клиенты Territory Wars.

Поддерживаемые языки:
- Python;
- C#;
- Go.

Клиенты запускаются на компьютерах студентов и взаимодействуют с игровым сервером через HTTP API.

Основная задача — предоставить понятную, надёжную и расширяемую основу для реализации игровых стратегий.

## 2. Общие требования

Все клиенты должны:
- использовать один и тот же серверный API;
- реализовывать одинаковый игровой протокол;
- корректно обрабатывать сетевые ошибки;
- поддерживать таймауты;
- корректно завершаться;
- иметь независимую от сетевого взаимодействия стратегию;
- работать на Windows и Linux.

Не создавать собственные игровые правила.

Не изменять серверный протокол ради удобства отдельного языка.

## 3. Структура клиентов

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

### 3.1 Go-клиент

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

### 3.2 Python-клиент

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

### 3.3 C#-клиент

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

## 4. Единый контракт API

Единственным источником истины для HTTP API является серверная реализация и актуальная спецификация (`docs/api-spec.md`).

Все клиенты должны одинаково трактовать:
- JSON-поля;
- типы данных;
- координаты;
- игровые действия;
- ресурсы;
- идентификаторы;
- статусы матча;
- ошибки;
- временные метки.

Особое внимание уделять:
- отсутствующим JSON-полям;
- null;
- нулевым значениям;
- enum;
- целочисленным значениям;
- массивам;
- вложенным структурам.

Не допускать различий в трактовке одного и того же Observation.

## 5. Разделение ответственности

Каждый клиент должен разделять следующие компоненты:

1. **API Client** — HTTP-взаимодействие.
2. **Models** — игровые модели и сериализация.
3. **Strategy** — принятие решений.
4. **Game Loop** — управление игровым циклом.
5. **Configuration** — параметры запуска.
6. **Logging** — диагностика.

**Правило:** Стратегия не должна зависеть от HTTP-клиента. Сетевой код не должен содержать игровые эвристики.

## 6. Игровой цикл (Game Loop)

### 6.1 Go-клиент: HTTP polling + WebSocket submit

Go-клиент использует **HTTP-опрос** для получения состояния и **WebSocket** только для отправки действий.

**Правило:** Go-клиент получает состояние только через HTTP. Из WebSocket он читает лишь ping (отвечая pong) и событие `GAME_OVER`; игровые события для принятия решений не использует.

1. `Join` → регистрация, получение токена.
2. `GetObservation` (HTTP) → начальное состояние.
3. `ConnectWebSocket` → подключение к WS.
4. Цикл:
   - `GetObservation` (HTTP) → текущее состояние.
   - `strategy.Decide(obs)` → решение.
   - `SubmitActionWebSocket(conn, msg)` → отправка через WS.
   - HTTP polling (500ms) до изменения `turn`.
5. Reconnect: при потере WS → `GetObservation` → проверка `game_over` → `Join` (re-join) → `ConnectWebSocket`.

### 6.2 Python-клиент: WebSocket events

Python-клиент использует **WebSocket events** для получения состояния.

**Правило:** Python-клиент читает все события из WS (ACTION_ACCEPTED, TURN_RESOLVED, OBSERVATION, GAME_OVER, ERROR, STILL_ALIVE, CONNECTED).

1. `GameApiClient.register()` → регистрация.
2. `api_client.connect()` → подключение к WS.
3. `_wait_for_observation()` → ожидание первого OBSERVATION.
4. Цикл:
   - `strategy.decide(observation)` → решение.
   - `api_client.submit_action(msg)` → отправка через WS.
   - `_wait_for_observation()` → ожидание TURN_RESOLVED + OBSERVATION.
5. Reconnect: при потере WS → `api_client.connect()` → `_wait_for_observation()`.

### 6.3 C#-клиент: HTTP polling + WebSocket submit + response queue

C#-клиент использует **HTTP-опрос** для состояния, **WebSocket** для отправки действий и **очередь сообщений** для ответа сервера.

**Правило:** C#-клиент имеет background read loop, который буферизует все WS-сообщения в `ConcurrentQueue`. `WaitForActionResponseAsync` читает из очереди, ожидая ACTION_ACCEPTED или ERROR.

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

## 7. Интерфейс стратегии

Во всех реализациях должна существовать отдельная точка расширения для алгоритма принятия решений.

Логический контракт:

```text
Decide(Observation) -> Action
```

Стратегия:
- получает Observation;
- анализирует доступную информацию;
- возвращает Action;
- не выполняет HTTP-запросы;
- не изменяет Observation;
- не получает доступ к внутреннему состоянию сервера.

Базовая стратегия должна быть понятной студенту. Не перегружать учебный пример сложными паттернами проектирования.

### 7.1 Go

```go
type Strategy interface {
    Decide(observation *models.Observation) *models.Action
}
```

Реализация: `SimpleStrategy` в `internal/strategy/simple.go`.

**Правило:** `Decide` возвращает `*models.Action` или `nil` (pass).

### 7.2 Python

```python
class IGameStrategy(ABC):
    @abstractmethod
    def decide(self, observation: Observation) -> Action:
        ...
```

Реализации: `SimpleStrategy`, `AggressiveStrategy` в `client/strategy.py`.

**Правило:** `decide` возвращает `Action` (type + optional target).

### 7.3 C#

```csharp
public interface IGameStrategy
{
    TerritoryWars.Models.Action Decide(Observation observation);
}
```

Реализация: `SimpleStrategy` в `Strategy/SimpleStrategy.cs`.

**Правило:** `Decide` возвращает `Action` или `pass`.

## 8. Ограничения доступной информации

Стратегия может использовать исключительно информацию, полученную от сервера в Observation.

Запрещено:
- получать скрытые данные;
- пытаться восстановить полное состояние сервера;
- обращаться к внутренним API;
- использовать данные визуализатора;
- использовать информацию о других участниках, недоступную через Observation.

Не закладывать в эталонные стратегии нечестные преимущества.

## 9. Работа с HTTP

Все запросы должны:
- использовать таймауты;
- проверять HTTP status code;
- корректно обрабатывать ошибки;
- проверять формат ответа;
- ограничивать повторные попытки.

Повторять запросы можно только тогда, когда это безопасно с учётом семантики конкретного endpoint.

Особенно осторожно обращаться с отправкой Action: повторная отправка может привести к конфликту или дублированию действий.

**Правило:** Не создавать новый HTTP-клиент для каждого запроса. Не выводить секреты в лог.

## 10. Языковые требования

### 10.1 Python

- Поддерживать Python 3.12+.
- Использовать type hints.
- Предпочитать стандартные средства языка и минимальный набор зависимостей.
- Использовать отдельные модели данных (dataclasses / Pydantic).
- Использовать `logging` module.
- Обрабатывать сетевые ошибки и исключения.
- Корректно завершать работу при Ctrl+C.

### 10.2 C#

- Поддерживать .NET 10+.
- Использовать HttpClient.
- Применять async/await.
- Использовать CancellationToken.
- Использовать System.Text.Json.
- Не создавать HttpClient на каждый запрос.
- Корректно обрабатывать отмену и исключения.

### 10.3 Go

- Поддерживать Go 1.22+.
- Использовать net/http.
- Использовать encoding/json.
- Использовать context.Context.
- Переиспользовать http.Client.
- Проверять и закрывать HTTP response body.
- Не игнорировать ошибки.
- Соблюдать идиоматичный стиль Go.

## 11. Конфигурация

### 11.1 Go-клиент

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

### 11.2 Python-клиент

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

### 11.3 C#-клиент

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

### 11.4 Приоритет конфигурации

**Правило:** CLI-флаги > config.json > значения по умолчанию.

## 12. Логирование

### 12.1 Go-клиент

`multiWriter` — дублирует `slog` вывод в stdout и log file.

Log file: `logs_dir/player_name.log`.

### 12.2 Python-клиент

`logging` module — file handler + basicConfig.

Log file: `logs_dir/player_name.log`.

### 12.3 C#-клиент

`Logger` (static class) — `StreamWriter` + `Console.WriteLine`.

Log file: `logs_dir/player_name.log`.

**Правило:** Все три клиента дублируют вывод в stdout и log file. Не выводить токены в лог.

## 13. Обработка ошибок и переподключение

### 13.1 Общие принципы

1. Все три клиента поддерживают переподключение при потере WS-соединения.
2. При потере WS → проверка `game_over` через HTTP → re-join (новый токен) → reconnect.
3. `max_retries` (по умолчанию 10) — максимальное количество переподключений.
4. После re-join клиент продолжает игру с новым токеном.

### 13.2 Re-join

При `StopGame` сервер инвалидирует все токены. Клиент должен:

1. Заметить ошибку WS.
2. Проверить `game_over` через HTTP.
3. Если не game_over → `Join` с новым токеном.
4. Переподключиться к WS с новым токеном.

**Правило:** Все три клиента реализуют re-join. Go и C# проверяют строки ошибок (`registration is closed`, `game stopped`, `match is full`) для graceful exit.

### 13.3 WebSocket ping/pong

- Сервер отправляет `websocket.PingMessage` каждые 25 секунд.
- Go-клиент: `conn.SetPingHandler()` → `conn.WriteMessage(websocket.PongMessage, ...)`.
- Python-клиент: `websockets.connect(ping_interval=20, ping_timeout=10)`.
- C#-клиент: `ClientWebSocket` auto-responds to pings (но нужен background read loop).

**Правило:** Без pong-ответов сервер закроет соединение.

## 14. Тестирование

Каждая реализация должна иметь собственные unit-тесты.

Обязательно тестировать:
- JSON-десериализацию;
- JSON-сериализацию;
- выбор действий;
- граничные условия;
- отсутствие ресурсов;
- некорректные Observation;
- сетевые ошибки;
- таймауты;
- завершение игрового цикла.

Для HTTP-тестов использовать локальные mock-серверы или эквивалентные средства соответствующего языка. Не использовать работающий production-сервер для unit-тестов.

### 14.1 Python

```bash
cd twclients/python && uv run pytest tests/ -v
```

Тесты:
- `test_api.py` — URL construction (http→ws, https→wss, trailing slash, port).
- `test_models.py` — serialization/deserialization (Point, Cell, PublicPlayer, Observation, Action, ActionMessage, JoinResponse, Event).
- `test_strategy.py` — SimpleStrategy (empty cells, attack, expand, fortify, pass, defensive fallback).

### 14.2 C#

Тесты:
- `Tests/StrategyTests.cs` — Decide возвращает допустимый тип действия, пустые клетки, attack/expand/fortify/pass, оборонительный режим, ограничения по энергии.

**Ограничение:** тестового проекта нет — файлы `Tests/**` исключены из сборки
(`<Compile Remove="Tests\**" />`), пакет тестового фреймворка не подключён.
Команда `dotnet test` не выполняет ничего. Чтобы тесты начали выполняться, нужен
отдельный проект с xUnit.

```bash
cd twclients/csharp && dotnet build && dotnet run -- --config config.json
```

### 14.3 Go

Go-клиент имеет тесты загрузки конфигурации: `internal/config/config_test.go`
(значения по умолчанию, разбор `http_timeout`/`max_retries`, подмена ненулевых значений).
Стратегия `SimpleStrategy` отдельных тестов не имеет и тестируется косвенно через
серверные интеграционные тесты.

```bash
cd twclients/golang && go test ./...
```

**Правило:** Любое изменение стратегии должно сопровождаться тестами.

## 15. Согласованность реализаций

Python, C# и Go клиенты должны иметь одинаковую функциональность.

При изменении моделей:
1. Проверить серверный контракт.
2. Обновить модели всех языков.
3. Проверить сериализацию.
4. Обновить тесты.
5. Проверить интеграционную совместимость.

По возможности использовать общие JSON test fixtures для проверки всех трёх реализаций.

## 16. Учебная направленность

Код предназначен для изучения студентами.

Поэтому:
- сохранять простую структуру;
- давать понятные имена;
- комментировать нетривиальную логику;
- избегать избыточных абстракций;
- отделять обязательный инфраструктурный код от алгоритма;
- документировать точки расширения.

Студент должен иметь возможность заменить стратегию без изменения API Client и Game Loop.

## 17. Кроссплатформенный запуск

### 17.1 Общие скрипты

- `run.sh` / `run.cmd` — запуск сервера + ботов + всех клиентов.
- `--no-go` / `--no-python` / `--no-csharp` — пропуск конкретных клиентов.
- `--no-bots` — сервер без ботов.
- `--force-start` — старт игры после регистрации.

### 17.2 Windows (run.cmd)

Все клиенты запускаются через `start "Name" /I cmd.exe /c "cd /d dir && command"`.

**Правило:** Все три клиента используют одинаковый паттерн запуска на Windows.

### 17.3 Linux/macOS (run.sh)

- Go: `go run ./cmd/client/ --config config.json --name Name`.
- Python: `uv run python main.py --config config.json --name Name`.
- C#: `dotnet run --project . -- --config config.json --name Name`.

## 18. Расхождения между клиентами

### 18.1 Получение состояния

| Клиент | Метод |
|--------|-------|
| Go | HTTP polling (`GetObservation`) |
| Python | WebSocket events (`_wait_for_observation`) |
| C# | HTTP polling (`GetObservationAsync`) |

**Правило:** Разные подходы к получению состояния — это фактическая архитектура, а не баг. Каждый клиент реализован по-своему.

### 18.2 Стратегии

| Клиент | Доступные стратегии |
|--------|---------------------|
| Go | `simple` (единственная) |
| Python | `simple`, `aggressive` |
| C# | `Simple` (единственная) |

**Примечание:** Python имеет `AggressiveStrategy`, но Go и C# — только `SimpleStrategy`.

### 18.3 Приоритет действий в SimpleStrategy

| Клиент | Приоритет |
|--------|-----------|
| Go | attack → expand → fortify → pass |
| Python | attack → expand → fortify → pass |
| C# | attack → expand → fortify → pass |

**Примечание:** Все три клиента имеют одинаковый приоритет в SimpleStrategy.

## 19. Известные ограничения

1. Go-клиент читает из WebSocket только ping/pong и `GAME_OVER`; игровые события (`OBSERVATION`, `TURN_RESOLVED`) для логики не используются — состояние берётся из HTTP.
2. C#-клиент имеет background read loop, который может накапливать сообщения в очереди.
3. Python-клиент использует `websockets` библиотеку с `ping_interval=20`, что отличается от серверных 25 секунд.
4. C#-клиент использует `CamelCase` для JSON-сериализации (через `JsonNamingPolicy.CamelCase`), но сервер ожидает `snake_case`. Это работает, потому что C#-клиент отправляет `ActionMessage` с `CamelCase`, а сервер парсит его без учёта регистра.
5. Go-клиент использует `gorilla/websocket`, Python — `websockets`, C# — `System.Net.WebSockets.ClientWebSocket`. Разные библиотеки, разные API.
6. C#-клиент не имеет `AggressiveStrategy` (только `SimpleStrategy`).
7. Python-клиент имеет `AggressiveStrategy`, но она не доступна в Go и C# клиентах.

## 20. Завершение работы

После изменения клиента:
- выполнить форматирование;
- выполнить unit-тесты;
- проверить сборку;
- проверить запуск;
- обновить README при необходимости.

При изменении общего контракта обязательно проверить все три языка.

В итоговом отчёте указывать реальные результаты проверок.
