# Territory Wars Python Client

Python-клиент для игрового сервера [Territory Wars](https://github.com/example/territory-wars). Регистрация, наблюдение за состоянием игры и отправка действий через WebSocket.

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
  │                       build_strategy()
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
  "strategy": "simple"
}
```

Доступные стратегии:
- **`simple`** — сначала expand, затем attack, затем fortify, затем pass
- **`aggressive`** — сначала attack, затем expand, затем fortify, затем pass

## Как написать собственную стратегию

Наследуйте `IGameStrategy` и реализуйте метод `decide`:

```python
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Пример: всегда expand в первую доступную нейтральную клетку
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

Затем зарегистрируйте в `build_strategy()`:

```python
# В strategy.py
def build_strategy(name: str) -> IGameStrategy:
    strategies = {
        "simple": SimpleStrategy,
        "my_strategy": MyStrategy,
    }
    return strategies[name]()
```

Или используйте напрямую в коде:

```python
from client.api import GameApiClient
from client.game_loop import GameLoop
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        return Action(type="pass")  # заглушка

api = GameApiClient("http://localhost:8080", "your-token")
loop = GameLoop(api, MyStrategy())
loop.run()
```

## Справочник API

### GameApiClient

```python
# Регистрация (статический метод)
join = GameApiClient.register("http://localhost:8080", "MyBot")

# Методы экземпляра
client = GameApiClient(base_url="http://localhost:8080", token=join.token)
obs = client.get_observation()          # GET /api/v1/state
data = client.get_leaderboard()         # GET /api/v1/leaderboard
client.connect()                        # WebSocket подключение
client.submit_action(action_message)    # Отправка через WebSocket
event = client.receive_event()          # Получение из WebSocket
client.close()                          # Очистка
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
client.submit_action(msg)
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

**Решение:** Перерегистрируйтесь. Токены одноразовые на подключение.

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
