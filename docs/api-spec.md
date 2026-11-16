# Territory Wars — HTTP API Contract

## 1. Общие правила

- Все маршруты префиксированы `/api/v1/` (кроме `/health` и `/ws`).
- Все JSON-поля используют `snake_case`.
- Ответы сервера — `application/json`.
- Токен передаётся через `Authorization: Bearer <token>` header или query parameter `?token=`.
- Размер тела запроса ограничен 4096 байт (`http.MaxBytesReader`).

## 2. Health Check

```
GET /health
```

**Ответ (200 OK):**

```json
{
  "status": "ok"
}
```

**Аутентификация:** не требуется.

## 3. Регистрация игрока (Join)

```
POST /api/v1/join
Content-Type: application/json
Body: {"name": "PlayerName"}
```

**Ограничения:**
- `name` — не более 4096 байт.
- `name` trim-ится (пробелы по краям удаляются).
- Регистрация закрыта после `StartGame()` или `StopGame()` (до следующего `Join`).
- Максимум `max_players` игроков (по умолчанию 20).

**Ответ (201 Created):**

```json
{
  "player_id": "p01",
  "token": "a1b2c3d4e5f6...",
  "websocket_url": "/ws"
}
```

- `player_id` — уникальный идентификатор игрока (строка).
- `token` — 48-символьный hex (randomHex(24) → crypto/rand). Используется для всех последующих запросов.
- `websocket_url` — всегда `/ws`.

**Ответ (409 Conflict):**

```json
{
  "error": "registration is closed"
}
```

или

```json
{
  "error": "match is full"
}
```

**Ответ (400 Bad Request):**

```
invalid JSON
```

## 4. Наблюдение (State)

```
GET /api/v1/state
Authorization: Bearer <token>
```

**Ответ (200 OK):**

```json
{
  "match_id": "string",
  "player_id": "p01",
  "turn": 5,
  "game_over": false,
  "width": 40,
  "height": 40,
  "energy": 5,
  "score": 0,
  "cells": [
    {
      "x": 0,
      "y": 0,
      "owner": "p01",
      "health": 5,
      "type": "base"
    }
  ],
  "players": [
    {
      "player_id": "p01",
      "name": "PlayerName",
      "x": 0,
      "y": 0,
      "score": 0,
      "energy": 5,
      "territory": 1,
      "connected": true,
      "eliminated": false
    }
  ],
  "available_actions": ["expand", "attack", "fortify", "pass"],
  "expand_targets": [{"x": 1, "y": 0}],
  "attack_targets": [],
  "fortify_targets": []
}
```

**Поля Observation:**

| Поле | Тип | Описание |
|------|-----|----------|
| `match_id` | string | Уникальный идентификатор матча |
| `player_id` | string | ID текущего игрока |
| `turn` | int | Текущий ход (0 = ожидание старта) |
| `game_over` | bool | true, если игра окончена |
| `width` | int | Ширина карты |
| `height` | int | Высота карты |
| `energy` | int | Текущая энергия игрока |
| `score` | int | Текущий счёт игрока |
| `cells` | Cell[] | Все клетки на карте |
| `players` | PublicPlayer[] | Все игроки |
| `available_actions` | string[] | Доступные типы действий |
| `expand_targets` | Point[] | Координаты соседних нейтральных клеток (опционально) |
| `attack_targets` | Point[] | Координаты соседних вражеских клеток (опционально) |
| `fortify_targets` | Point[] | Координаты соседних своих клеток с health <= 3 (опционально) |

**Поля Cell:**

| Поле | Тип | Описание |
|------|-----|----------|
| `x` | int | Координата X |
| `y` | int | Координата Y |
| `owner` | string | ID владельца (пустая строка = нейтральная) |
| `health` | int | Здоровье клетки (normal=3, base=5, макс=7) |
| `type` | string | Тип: `base`, `normal`, `resource` |

**Поля PublicPlayer:**

| Поле | Тип | Описание |
|------|-----|----------|
| `player_id` | string | ID игрока |
| `name` | string | Имя игрока |
| `x` | int | Координата X |
| `y` | int | Координата Y |
| `score` | int | Счёт |
| `energy` | int | Энергия |
| `territory` | int | Количество клеток под контролем |
| `connected` | bool | Подключён ли игрок |
| `eliminated` | bool | Исключён ли игрок |

**Ответ (401 Unauthorized):**

```
unauthorized
```

**Ответ (400 Bad Request):**

```
<error message>
```

## 5. Таблица лидеров (Leaderboard)

```
GET /api/v1/leaderboard
```

**Ответ (200 OK):**

```json
{
  "status": {
    "match_id": "string",
    "turn": 5,
    "max_turns": 200,
    "game_over": false,
    "stopped": false
  },
  "leaderboard": [
    {
      "turn": 5,
      "player_id": "p01",
      "action": {
        "type": "expand",
        "target": {"x": 1, "y": 0}
      },
      "accepted": true,
      "reason": ""
    }
  ],
  "results": [
    {
      "turn": 5,
      "player_id": "p01",
      "action": {
        "type": "expand",
        "target": {"x": 1, "y": 0}
      },
      "accepted": true,
      "reason": ""
    }
  ]
}
```

**Аутентификация:** не требуется.

## 6. Результаты (Results)

```
GET /api/v1/results
```

**Ответ (200 OK):**

```json
{
  "turn": 5,
  "leaderboard": [...],
  "results": [...]
}
```

- `turn` — счёт первого игрока в leaderboard (фактически текущий ход).
- `leaderboard` — список игроков, отсортированный по score (убывание).
- `results` — результаты всех действий за все ходы.

**Ответ (405 Method Not Allowed):**

```
method not allowed
```

## 7. Старт игры (Start)

```
POST /api/v1/start
```

**Ответ (200 OK):**

```json
{
  "status": "game started"
}
```

или

```json
{
  "status": "game already started"
}
```

**Поведение:**
- Игра НЕ начинается автоматически.
- `StartGame()` закрывает `gameStarted` channel, разрешая ходы.
- Запускает ботов для заполнения оставшихся слотов (`max_players - human_players`).
- Боты регистрируются через `engine.Join()` и подключаются к RoundManager.

**Ответ (405 Method Not Allowed):**

```
method not allowed
```

## 8. Стоп игры (Stop)

```
POST /api/v1/stop
```

**Ответ (200 OK):**

```json
{
  "status": "game stopped"
}
```

**Поведение (полная последовательность):**

1. `engine.ClearTokens()` — инвалидация всех токенов.
2. `rounds.cleanupAll()` — закрытие всех WS-соединений.
3. `shutdownCancel()` — остановка Viewer monitoring goroutine.
4. `rounds.Stop()` — остановка RoundManager event loop.
5. `engine.Reset()` — сброс состояния движка.
6. `engine.ClearPlayers()` — очистка игроков.
7. `ResetGameStarted()` — пересоздание `gameStarted` channel.
8. `botManager.Shutdown()` — остановка ботов.
9. `rounds.Restart()` — перезапуск RoundManager event loop.
10. `engine.ResetStopped()` — разрешение новой регистрации.
11. Пересоздание shutdown context + restart Viewer monitoring.

**После StopGame:**
- Все токены инвалидированы. Клиенты получают `unauthorized` при попытке подключения.
- После `max_retries` переподключений клиенты выходят.
- Регистрация снова открыта.

**Ответ (405 Method Not Allowed):**

```
method not allowed
```

## 9. Пауза (Pause)

```
POST /api/v1/pause
```

**Ответ (200 OK):**

```json
{
  "status": "game paused"
}
```

**Поведение:** Останавливает RoundManager event loop от обработки действий и продвижения ходов.

**Ответ (405 Method Not Allowed):**

```json
{
  "error": "POST required"
}
```

## 10. Возобновление (Resume)

```
POST /api/v1/resume
```

**Ответ (200 OK):**

```json
{
  "status": "game resumed"
}
```

**Поведение:** Снимает блокировку RoundManager event loop.

**Ответ (405 Method Not Allowed):**

```json
{
  "error": "POST required"
}
```

## 11. WebSocket

```
ws://localhost:8080/ws?token=<token>
```

или

```
wss://localhost:8080/ws?token=<token>
```

Токен передаётся через query parameter `?token=` или `Authorization: Bearer` header.

**Подключение:**

1. Клиент открывает WS-соединение с токеном.
2. Сервер аутентифицирует токен.
3. Сервер устанавливает `Connected = true` для игрока.
4. Сервер подключает клиента к RoundManager.
5. Сервер отправляет `CONNECTED` и начальное `OBSERVATION`.
6. Сервер запускает goroutine для отправки событий (ping каждые 25 секунд).

**Отправка действий (клиент → сервер):**

```json
{
  "type": "ACTION",
  "turn": 5,
  "actions": [
    {
      "type": "expand",
      "target": {"x": 1, "y": 0}
    }
  ]
}
```

**Поля ActionMessage:**

| Поле | Тип | Описание |
|------|-----|----------|
| `type` | string | Всегда `"ACTION"` |
| `turn` | int | Текущий ход |
| `actions` | Action[] | Список действий (одно действие за ход) |

**Приём событий (сервер → клиент):**

```json
{
  "type": "OBSERVATION",
  "turn": 5,
  "observation": {...}
}
```

**Типы событий:**

| Тип | Описание |
|-----|----------|
| `CONNECTED` | Подключение успешно |
| `OBSERVATION` | Снимок состояния игры |
| `ACTION_ACCEPTED` | Действие принято (отправляется игроку, который отправил действие) |
| `TURN_RESOLVED` | Раунд разрешён (отправляется всем подключённым) |
| `GAME_OVER` | Игра окончена |
| `ERROR` | Ошибка |

**Поля Event:**

| Поле | Тип | Описание |
|------|-----|----------|
| `type` | string | Тип события |
| `turn` | int | Номер хода |
| `message` | string | Сообщение (для ERROR) |
| `observation` | Observation | Снимок состояния (для OBSERVATION) |
| `results` | Result[] | Результаты хода (для TURN_RESOLVED) |

**Жизненный цикл события:**

1. **ACTION_ACCEPTED** — отправляется игроку, который отправил действие, сразу после принятия.
2. **TURN_RESOLVED** — транслируется всем подключённым клиентам при разрешении раунда.
3. **OBSERVATION** — отправляется каждому клиенту после TURN_RESOLVED.

Инвариант: `ACTION_ACCEPTED → TURN_RESOLVED → OBSERVATION`.

**Ping/Pong:**

- Сервер отправляет `websocket.PingMessage` каждые 25 секунд.
- Клиент должен отвечать `websocket.PongMessage`.
- Без pong-ответов сервер закроет соединение (read deadline 120 секунд).

**Generation:**

- При каждом переподключении generation монотонно увеличивается.
- Старые соединения игнорируются (generation mismatch).
- `Disconnect(playerID, generation)` — отключение с проверкой generation.

**Outbound queue:**

- Bounded channel (capacity 256).
- Переполнение → disconnect.

**Ограничения:**

- `ReadLimit` — 64 KB.
- `ReadBufferSize` / `WriteBufferSize` — 4096 байт.
- `CheckOrigin` — всегда true (CORS для локальной разработки).

## 12. Viewer API

### 12.1 HTML-страница

```
GET /
GET /viewer
```

**Ответ:** HTML-страница (`viewer.html`, встроен через `//go:embed`).

**Headers:**
- `Content-Type: text/html; charset=utf-8`
- `Cache-Control: no-cache, no-store, must-revalidate`
- `Pragma: no-cache`
- `Expires: 0`

### 12.2 Состояние игры (State)

```
GET /api/v1/viewer/state
```

**Ответ (200 OK):**

```json
{
  "match_id": "string",
  "turn": 5,
  "max_turns": 200,
  "game_over": false,
  "width": 40,
  "height": 40,
  "players": [
    {
      "id": "p01",
      "name": "PlayerName",
      "x": 0,
      "y": 0,
      "score": 0,
      "energy": 5,
      "territory": 1,
      "connected": true,
      "eliminated": false,
      "is_bot": false
    }
  ],
  "bot_players": [...],
  "cells": [
    {
      "x": 0,
      "y": 0,
      "owner": "p01",
      "type": "base",
      "health": 5
    }
  ],
  "history": [...]
}
```

**Аутентификация:** не требуется.

### 12.3 SSE-стриминг (Events)

```
GET /api/v1/viewer/events
```

**Ответ:** `text/event-stream`

**Формат SSE:**

```
event: state
data: {"match_id":"...","turn":5,...}

event: history
data: [{"player":"Bot-aggressive-1","action":"expand","turn":1,...}]

```

**Типы событий:**

| Событие | Описание |
|---------|----------|
| `state` | Полный снимок состояния игры |
| `history` | История действий ботов |

**Headers:**
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`
- `Access-Control-Allow-Origin: *`

**Поведение:**
- При подключении отправляется начальное `state` и `history`.
- `state` обновляется каждые 5 секунд или при разрешении хода.
- `history` обновляется при разрешении хода.
- Клиент отключается при закрытии соединения.

### 12.4 История ботов (History)

```
GET /api/v1/viewer/history
```

**Ответ (200 OK):**

```json
[
  {
    "player": "Bot-aggressive-1",
    "action": "expand",
    "turn": 1,
    "x": 1,
    "y": 0,
    "reason": ""
  }
]
```

**Аутентификация:** не требуется.

## 13. Типы действий

| Тип | Стоимость | Описание |
|-----|-----------|----------|
| `expand` | 1 energy | Расширение на соседнюю нейтральную клетку |
| `attack` | 2 energy | Атака на соседнюю вражескую клетку |
| `fortify` | 2 energy | Укрепление своей клетки (health <= 3) |
| `pass` | 0 | Пропуск хода |

**Action:**

```json
{
  "type": "expand",
  "target": {"x": 1, "y": 0}
}
```

- `type` — тип действия.
- `target` — координаты цели (null для `pass`).

## 14. Результаты (Result)

```json
{
  "turn": 5,
  "player_id": "p01",
  "action": {
    "type": "expand",
    "target": {"x": 1, "y": 0}
  },
  "accepted": true,
  "reason": ""
}
```

**Поля Result:**

| Поле | Тип | Описание |
|------|-----|----------|
| `turn` | int | Номер хода |
| `player_id` | string | ID игрока |
| `action` | Action | Действие игрока |
| `accepted` | bool | Принято ли действие |
| `reason` | string | Причина отклонения (если accepted=false) |

## 15. Конфигурация сервера

**Файл:** `twserver/config.json`

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

**Значения по умолчанию:**

| Поле | По умолчанию |
|------|-------------|
| `port` | 8080 |
| `max_players` | 20 |
| `max_turns` | 200 |
| `width` | 40 |
| `height` | 40 |
| `turn_delay` | 15s |
| `mark_rate` | 10 |
| `mark_threshold` | 5 |

**Env-переменные (переопределяют config.json):**

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `PORT` | 8080 | HTTP порт |
| `MAX_PLAYERS` | 20 | Максимум игроков |
| `MAX_TURNS` | 200 | Максимум ходов |
| `TURN_TIMEOUT_SECONDS` | 15 | Таймаут хода (не используется) |

**CLI-флаг:** `-config` — путь к JSON config file.

## 16. Интерактивная консоль

```
> start
> stop
> pause
> resume
> results
> quit
```

**Команды:**

| Команда | Описание |
|---------|----------|
| `start` | Запуск игры (аналог POST /api/v1/start) |
| `stop` | Остановка игры (аналог POST /api/v1/stop) |
| `pause` | Пауза (аналог POST /api/v1/pause) |
| `resume` | Возобновление (аналог POST /api/v1/resume) |
| `results` | Вывод результатов в stdout + сохранение в файл |
| `quit` | Остановка сервера |

**Сохранение результатов:**

Файл: `results_dir/results.yy.mm.dd.hhmm.txt`
