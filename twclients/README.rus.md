# Territory Wars — External Game Clients

Готовые клиенты для подключения к серверу Territory Wars на Python, C# и Go.

## Назначение

Эти клиенты предоставляют студентам рабочую основу для разработки собственных игровых алгоритмов. Каждый клиент:

- Подключается к серверу по HTTP API и WebSocket
- Регистрирует игрока и получает токен сессии
- Получает состояние игры (Observation)
- Отправляет действия (Actions) через WebSocket
- Работает автономно до завершения матча

## Общая архитектура

Каждый клиент состоит из одинаковых логических компонентов:

| Компонент | Описание |
|-----------|----------|
| **HTTP Client** | Взаимодействие с REST API (регистрация, получение состояния) |
| **WebSocket Client** | Отправка действий и получение событий в реальном времени |
| **API Models** | Структуры данных, соответствующие контракту сервера |
| **Game Agent** | Алгоритм принятия решений (Strategy) |
| **Game Loop** | Основной игровой цикл: observe → decide → submit → repeat |
| **Configuration** | Параметры подключения, имя игрока, таймауты |
| **Logging** | Информирование о ходе игры |
| **Error Handling** | Обработка сетевых ошибок, таймаутов, некорректных ответов |

## HTTP-протокол

### Регистрация
```
POST /api/v1/join
Content-Type: application/json

{"name": "PlayerName"}

Response: {"player_id": "p01", "token": "...", "websocket_url": "/ws"}
```

### Получение состояния игры
```
GET /api/v1/state
Authorization: Bearer TOKEN

Response: Observation JSON
```

### Получение таблицы лидеров
```
GET /api/v1/leaderboard

Response: {"status": {...}, "leaderboard": [...], "results": [...]}
```

### Отправка действий
Действия отправляются через WebSocket:
```
ws://localhost:8080/ws?token=YOUR_TOKEN

Message: {"type": "ACTION", "turn": 0, "actions": [{"type": "expand", "target": {"x": 20, "y": 4}}]}
```

### Observation (состояние игры)
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
  "players": [{"player_id": "p01", "x": 20, "y": 20, "score": 0, "connected": true, "eliminated": false}],
  "available_actions": ["expand", "attack", "fortify", "pass"]
}
```

### Action (действие)
```json
{"type": "expand", "target": {"x": 20, "y": 4}}
```

Допустимые типы действий:
- **expand** — захватить соседнюю нейтральную клетку (стоит 1 энергию)
- **attack** — атаковать соседнюю клетку врага (стоит 2 энергии)
- **fortify** — укрепить свою клетку (+2 здоровья, стоит 2 энергии)
- **pass** — пропустить ход (бесплатно)

## Установка и запуск

### Python Client

**Требования:** Python 3.12+, [uv](https://github.com/astral-sh/uv)

```bash
cd twclients/python
uv sync
uv run python main.py --name PythonClient --config config.json
```

Или с аргументами командной строки:
```bash
uv run python main.py --server http://localhost:8080 --name "MyBot"
```

### C# Client

**Требования:** .NET 10 SDK

```bash
cd twclients/csharp
dotnet restore
dotnet run -- --server http://localhost:8080 --name "MyBot"
```

Или с конфигурационным файлом:
```bash
dotnet run -- --config config.json
```

### Go Client

**Требования:** Go 1.22+

```bash
cd twclients/golang
go mod tidy
go build -o client ./cmd/client/
./client --name MyBot --config config.json
```

Или с аргументами командной строки:
```bash
./client --server http://localhost:8080 --name "MyBot" --log-level DEBUG
```

## Запуск всех клиентов

### Скрипт `run.sh`

```bash
# Запустить сервер + все 15 клиентов + 10 ботов (по умолчанию)
./run.sh

# Запустить сервер + 15 клиентов (без ботов)
./run.sh --no-bots

# Пропустить конкретный язык
./run.sh --no-go --no-python
```

### Запуск 5 копий каждого клиента

```bash
# Go
cd twclients/golang
for i in 1 2 3 4 5; do
  nohup ./bin/client --name "GoClient$i" > /tmp/go_client_$i.log 2>&1 &
done

# Python
cd twclients/python
for i in 1 2 3 4 5; do
  uv run python main.py --name "PythonClient$i" > /tmp/python_client_$i.log 2>&1 &
done

# C#
cd twclients/csharp
for i in 1 2 3 4 5; do
  dotnet bin/publish/TerritoryWars.Client.dll --name "CSharpClient$i" > /tmp/csharp_client_$i.log 2>&1 &
done
```

## Стратегии

### Simple — базовая стратегия

Все три клиента включают стратегию `simple`, которая:

1. **Анализирует территорию** — определяет свои клетки и соседние нейтральные/вражеские
2. **Оценивает ресурсы** — проверяет уровень энергии
3. **Выбирает действие** по приоритету:
   - **Expand** — если есть энергия и соседние нейтральные клетки
   - **Attack** — если есть энергия и соседние вражеские клетки
   - **Fortify** — если есть энергия и свои клетки с низким здоровьем (≤3)
   - **Pass** — если нет допустимых действий

Примечание: Только стратегия `simple` реализована во всех клиентах.

## Как написать собственную стратегию

### Python

```python
from client.strategy import IGameStrategy
from client.models import Observation, Action

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Ваш алгоритм здесь
        return Action(type="expand", target={"x": 10, "y": 10})
```

Зарегистрируйте в `build_strategy()`:
```python
def build_strategy(name: str) -> IGameStrategy:
    strategies = {
        "simple": SimpleStrategy,
        "my_strategy": MyStrategy,
    }
    return strategies[name]()
```

### C#

```csharp
using TerritoryWars.Strategy;

public class MyStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        // Ваш алгоритм здесь
        return new Action { Type = "expand", Target = new Point { X = 10, Y = 10 } };
    }
}
```

Зарегистрируйте в `Program.cs`:
```csharp
IGameStrategy strategy = strategyName.ToLower() switch
{
    "simple" => new SimpleStrategy(),
    "my_strategy" => new MyStrategy(),
    _ => throw new ArgumentException(...)
};
```

### Go

```go
package strategy

import "twclient/internal/models"

type MyStrategy struct{}

func (s *MyStrategy) Decide(observation *models.Observation) *models.Action {
    // Ваш алгоритм здесь
    return &models.Action{Type: "expand", Target: &models.Point{X: 10, Y: 10}}
}
```

Зарегистрируйте в `cmd/client/main.go`:
```go
func createStrategy(strategyName string, logger *slog.Logger) strategy.Strategy {
    switch strings.ToLower(strategyName) {
    case "simple":
        return strategy.NewSimpleStrategy(logger)
    case "my_strategy":
        return NewMyStrategy(logger)
    default:
        return strategy.NewSimpleStrategy(logger)
    }
}
```

## Игровая механика

### Карта

- Размер: 40 × 40
- **Тороидальная** — координаты wrap-around: `(x + 1) % Width`, `(y + 1) % Height`
- 20 позиций спавна
- Типы клеток: `base` (база), `normal` (обычная), `resource` (ресурсная)

### Правила

- Каждый игрок начинает с 5 энергии и одной базы
- Одно действие на ход
- Все действия разрешаются одновременно в конце раунда
- Пропущенное действие становится `pass` по таймауту
- Матч заканчивается после `MAX_TURNS` (по умолчанию 200)

### Энергия

| Действие | Стоимость |
|----------|-----------|
| `expand` | 1 |
| `attack` | 2 |
| `fortify` | 2 |
| `pass` | 0 |

### Здоровье клеток

- Обычные клетки: старт 3, максимум 7
- Базы: старт 5, максимум 7

## Диагностика распространённых ошибок

### Connection refused
```
ERROR: Failed to connect to server: Connection refused
```
**Решение:** Убедитесь, что сервер запущен: `./bin/server -config config.json`

### Invalid token
```
ERROR: Unauthorized — invalid token
```
**Решение:** Токен истёк или неверный. Перезапустите клиента.

### Stale turn
```
ERROR: Stale turn: current turn is 5
```
**Решение:** Клиент отправляет действие для старого хода. Проверьте синхронизацию.

### Not enough energy
```
ERROR: Action rejected: not enough energy
```
**Решение:** Стратегия пытается действие без достаточной энергии. Используйте `pass`.

### Target outside map
```
ERROR: Action rejected: target is outside the map
```
**Решение:** Координаты за пределами карты (0..39 для 40x40). Используйте `% Width` / `% Height` для toroidal wrap-around.

## Интеграционное тестирование

Для проверки запустите сервер и подключите всех трёх клиентов одновременно:

```bash
# Терминал 1: сервер
cd twserver
./bin/server -config config.json

# Терминал 2: Python клиент
cd twclients/python
uv run python main.py --server http://localhost:8080 --name "PythonBot"

# Терминал 3: C# клиент
cd twclients/csharp
dotnet run -- --server http://localhost:8080 --name "CSBot"

# Терминал 4: Go клиент
cd twclients/golang
./client --server http://localhost:8080 --name "GoBot"
```

Все три клиента должны зарегистрироваться, получать состояние игры, отправлять действия и дождаться завершения матча.

## Структура репозитория

```
twclients/
├── README.md                    # English version
├── README.rus.md                # Русская версия (этот файл)
├── run.sh                       # Скрипт запуска сервера + клиентов
├── python/
│   ├── README.md
│   ├── pyproject.toml
│   ├── config.json
│   ├── main.py
│   ├── client/
│   │   ├── __init__.py
│   │   ├── api.py
│   │   ├── models.py
│   │   ├── strategy.py
│   │   └── game_loop.py
│   └── tests/
│       ├── test_models.py
│       ├── test_strategy.py
│       └── test_api.py
├── csharp/
│   ├── README.md
│   ├── TerritoryWars.Client.csproj
│   ├── config.json
│   ├── Program.cs
│   ├── Models/
│   │   ├── Observation.cs
│   │   ├── Cell.cs
│   │   ├── PublicPlayer.cs
│   │   ├── Action.cs
│   │   ├── ActionMessage.cs
│   │   ├── JoinResponse.cs
│   │   ├── ServerEvent.cs
│   │   └── Result.cs
│   ├── Api/
│   │   └── GameApiClient.cs
│   ├── Strategy/
│   │   ├── IGameStrategy.cs
│   │   └── SimpleStrategy.cs
│   ├── Game/
│   │   └── GameLoop.cs
│   └── Tests/
│       └── StrategyTests.cs
└── golang/
    ├── README.md
    ├── go.mod
    ├── go.sum
    ├── config.json
    ├── cmd/client/main.go
    └── internal/
        ├── api/
        │   └── client.go
        ├── models/
        │   └── models.go
        ├── strategy/
        │   ├── strategy.go
        │   └── simple.go
        ├── game/
        │   └── loop.go
        └── config/
            └── config.go
```

## Совместимость

Все три клиента:
- Используют один и тот же HTTP-протокол
- Имеют одинаковые JSON-поля и типы данных
- Работают с одним сервером без изменений
- Поддерживают кроссплатформенный запуск (Windows, Linux, macOS)
- Не требуют модификации серверной части
