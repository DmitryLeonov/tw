# Territory Wars Python Client

Python-клиент для игрового сервера [Territory Wars](../../README.rus.md). Регистрация, наблюдение за состоянием игры и отправка действий через WebSocket.

## Назначение

Этот клиент подключается к игровому серверу Territory Wars, регистрирует игрока и играет в игру с помощью настраиваемой стратегии. Он демонстрирует:

- Взаимодействие с HTTP API (регистрация, опрос состояния, таблица лидеров)
- Коммуникацию через WebSocket (отправка действий, получение событий)
- Паттерн стратегии для игрового ИИ
- Надёжную обработку ошибок с повторными попытками

## Архитектура

```
main.py
  │
  ├── client/api.py      GameApiClient — HTTP + WebSocket API
  │       ├── register()     POST /api/v1/join
  │       ├── get_observation()  GET /api/v1/state
  │       ├── get_leaderboard()  GET /api/v1/leaderboard
  │       ├── connect()      WebSocket подключение
  │       ├── submit_action()  Отправка ActionMessage через WS
  │       └── receive_event()  Получение Event из WS
  │
  ├── client/models.py   Dataclass: Cell, PublicPlayer, Observation,
  │                       Action, ActionMessage, JoinResponse, Event
  │
  ├── client/strategy.py IGameStrategy (ABC), SimpleStrategy,
  │                       AggressiveStrategy, build_strategy()
  │
  └── client/game_loop.py GameLoop — оркестратор основного игрового цикла
```

### Поток данных

```
1. Регистрация → JoinResponse(player_id, token)
2. Создание GameApiClient(token)
3. GameLoop.run():
   a. Подключение к WebSocket
   b. Получение начального OBSERVATION
   c. Цикл:
      - strategy.decide(observation) → Action
      - submit_action(ActionMessage)
      - ожидание TURN_RESOLVED + OBSERVATION
   d. Вывод финального состояния
```

## Установка

### Требования

- Python 3.12+
- [uv](https://github.com/astral-sh/uv) — быстрый установщик пакетов Python

### Установка зависимостей

```bash
uv sync
```

Устанавливает:
- **httpx** — Асинхронный HTTP-клиент с поддержкой повторных попыток
- **websockets** — WebSocket-клиент для отправки действий
- **pytest** (dev-группа) — запуск юнит-тестов из каталога `tests/`

### Запуск тестов

```bash
uv run pytest tests/ -v
```

## Использование

### Быстрый старт

```bash
# Регистрация и игра с настройками по умолчанию
uv run python main.py --server http://localhost:8080 --name MyBot

# Использовать файл конфига
uv run python main.py --config config.json

# Режим отладки
uv run python main.py --config config.json --log-level DEBUG
```

### Аргументы командной строки

| Аргумент | Описание | По умолчанию |
|----------|----------|--------------|
| `--server` | URL сервера (например, `http://localhost:8080`) | `http://localhost:8080` |
| `--name` | Имя игрока | `StudentBot` |
| `--config` | Путь к JSON-файлу конфига | (нет) |
| `--log-level` | Уровень логирования: DEBUG, INFO, WARNING, ERROR | `INFO` |

### Файл конфигурации

Файл `config.json` уже содержит пример конфигурации:

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

| Ключ | По умолчанию | Описание |
|------|--------------|----------|
| `server` | `http://localhost:8080` | Базовый URL сервера |
| `player_name` | `StudentBot` | Имя игрока для регистрации |
| `http_timeout` | `10` | Таймаут HTTP-запроса в секундах |
| `log_level` | `INFO` | DEBUG, INFO, WARNING, ERROR |
| `strategy` | `simple` | Название стратегии (см. ниже) |
| `max_retries` | `10` | Максимум переподключений WebSocket перед завершением |
| `logs_dir` | `""` (без файла) | Каталог для `logs_dir/имя_игрока.log` |

Вывод идёт в stdout и, если задан `logs_dir`, в `logs_dir/<имя_игрока>.log`.

Доступные стратегии:
- **`simple`** — оборонительный режим при здоровье слабойшей своей клетки ≤ 3 (fortify → expand → pass); иначе attack (стоит 2) → expand (стоит 1) → fortify (стоит 2) → pass
- **`aggressive`** — всегда атакует (стоит 2) при наличии соседнего противника, затем expand, затем fortify, затем pass; оборонительного режима нет

Примечание: в Python есть обе стратегии `simple` и `aggressive`; в Go- и C#-клиентах реализована только `simple`.

## Как написать собственную стратегию

Наследуйте `IGameStrategy` и реализуйте метод `decide`:

```python
from client.strategy import IGameStrategy
from client.models import Action, Observation, Point

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Пример: expand в первую соседнюю нейтральную клетку
        owned = observation.get_owned_cells()
        for cell in owned:
            for dx, dy in [(-1, 0), (1, 0), (0, -1), (0, 1)]:
                # Карта тороидальная — координаты заворачиваются.
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

Затем зарегистрируйте в `build_strategy()`:

```python
# В strategy.py
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

Или используйте напрямую в коде (`GameLoop.run()` — корутина, её нужно дождаться):

```python
import asyncio

from client.api import GameApiClient
from client.game_loop import GameLoop
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        return Action(type="pass")  # заглушка

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

## Справочник API

### GameApiClient

```python
# Регистрация (статический метод, синхронный HTTP)
join = GameApiClient.register("http://localhost:8080", "MyBot")

# Создание экземпляра
client = GameApiClient(base_url="http://localhost:8080", token=join.token)

# Синхронные HTTP-методы (httpx.Client)
obs = client.get_observation()             # GET /api/v1/state
data = client.get_leaderboard()            # GET /api/v1/leaderboard

# Корутины (WebSocket) — их нужно дождаться
await client.connect()                     # открыть WebSocket
await client.submit_action(action_message) # отправить ActionMessage
event = await client.receive_event()       # одно событие (таймаут по умолчанию 30 с)
await client.close()                       # закрыть WebSocket + HTTP-клиент
```

### Модель Observation

```python
obs = client.get_observation()
print(f"Ход: {obs.turn}")
print(f"Очки: {obs.score}")
print(f"Энергия: {obs.energy}")
print(f"Клетки: {len(obs.cells)}")
print(f"Свои: {len(obs.get_owned_cells())}")
print(f"Валидная координата (5,5): {obs.is_valid_coord(5, 5)}")
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

## Диагностика

### Connection refused

```
Connection error: Connection refused
```

**Решение:** Убедитесь, что сервер запущен:
```bash
cd /apps/tw/twserver
go build -o bin/server ./cmd/server/
./bin/server -config config.json
```

### Token Unauthorized

```
Unauthorized: token invalid or expired
```

**Решение:** Получите новый токен через `POST /api/v1/join`. Токен остаётся
действительным при переподключении WebSocket, но `POST /api/v1/stop` инвалидирует
все токены — после этого всем клиентам нужно заново зарегистрироваться.

### Action Rejected

Проверьте логи сервера для причин отклонения:
- `"target must be adjacent to your territory"` — цель expand/attack должна быть рядом с вашей клеткой
- `"not enough energy"` — expand стоит 1, attack/fortify стоит 2
- `"target is outside the map"` — координаты должны быть в пределах [0, width) × [0, height)
- `"exactly one action is required per turn"` — отправляйте ровно одно действие за ход

### Timeout Waiting for Events

```
Timeout waiting for event
```

**Решение:** Проверьте, что WebSocket-соединение живо. Клиент автоматически переподключается при ошибке.

### Strategy Not Found

```
Unknown strategy: foo
```

**Решение:** Используйте `simple` или `aggressive`, или добавьте свою стратегию в `build_strategy()`.

## Структура файлов

```
twclients/python/
├── README.md
├── README.rus.md
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

## Лицензия

Этот клиент предоставлен «как есть» в образовательных целях.
