# Territory Wars

Сервер пошаговой игры с одновременными действиями по контролю территории с клиентами на Python, C# и Go.

## Обзор

Territory Wars — многопользовательская игра, где игроки захватывают территорию на тороидальной сетке 40×40. Сервер написан на Go; клиенты доступны на Python, C# и Go. Студенты разрабатывают ИИ-стратегии для конкуренции друг с другом и встроенными ботами.

## Быстрый старт

### Требования

- **Go 1.22+** — сервер
- **Python 3.12+** с [uv](https://github.com/astral-sh/uv) — клиент Python
- **.NET 10 SDK** — клиент C#
- **curl** — для вызовов API

### Запуск одной командой

```bash
# Сервер + все 15 клиентов, боты заполняют оставшиеся слоты (по умолчанию)
./run.sh

# Сервер + все 15 клиентов, без ботов
./run.sh --no-bots

# Сервер + боты, пропустить Go-клиентов
./run.sh --no-go

# Сервер + боты + клиенты, запустить игру после регистрации
./run.sh --force-start
```

### Ручной запуск

```bash
# Сборка и запуск сервера
cd twserver
go build -o bin/server ./cmd/server/
./bin/server -config config.json

# Запуск клиентов в отдельных терминалах
cd ../twclients/golang && go build -o bin/client ./cmd/client/ && ./bin/client --name GoClient --config config.json
cd ../twclients/python && uv run python main.py --name PythonClient --config config.json
cd ../twclients/csharp && dotnet exec bin/Release/net10.0/publish/TerritoryWars.Client.dll --name CSharpClient --config config.json --strategy simple
```

## Архитектура

```
tw/
├── run.sh                    # Скрипт запуска (Linux/macOS)
├── run.cmd                   # Скрипт запуска (Windows)
├── README.md                 # English version
├── README.rus.md             # Этот файл
├── twserver/                 # Go-сервер игры
│   ├── README.md
│   ├── README.rus.md
│   ├── config.json           # Конфиг сервера (боты, лимиты, размер карты)
│   ├── cmd/server/main.go    # Точка входа
│   ├── internal/
│   │   ├── server/           # HTTP-хендлеры, WebSocket, Round Manager, загрузка конфига
│   │   └── game/             # Игровой движок (состояние, действия, разрешение)
│   └── bin/server            # Собранный бинарник
└── twclients/                # Внешние клиенты игры
    ├── README.md
    ├── README.rus.md
    ├── golang/               # Go-клиент
    │   ├── README.md
    │   ├── README.rus.md
    │   ├── cmd/client/main.go
    │   ├── internal/
    │   └── config.json
    ├── python/               # Python-клиент
    │   ├── README.md
    │   ├── README.rus.md
    │   ├── main.py
    │   ├── client/
    │   │   ├── api.py
    │   │   ├── models.py
    │   │   ├── strategy.py
    │   │   └── game_loop.py
    │   ├── pyproject.toml
    │   └── config.json
    └── csharp/               # C#-клиент
        ├── README.md
        ├── README.rus.md
        ├── Program.cs
        ├── TerritoryWars.Client.csproj
        ├── Api/
        ├── Game/
        ├── Models/
        ├── Strategy/
        └── config.json
```

## Игровая механика

### Карта

- **Размер:** 40 × 40
- **Тороидальная:** координаты wrap-around через `% Width` и `% Height`
- **30 позиций спавна** (до max_players)
- **Типы клеток:** `base` (база), `normal` (обычная), `resource` (ресурсная)

### Действия

| Действие | Стоимость | Цель | Описание |
|----------|-----------|------|----------|
| `expand` | 1 энергия | Нейтральная клетка | Захватить соседнюю нейтральную клетку |
| `attack` | 2 энергии | Вражеская клетка | Атаковать соседнюю вражескую клетку |
| `fortify` | 2 энергии | Своя клетка | Укрепить свою клетку (+2 здоровья, макс 7) |
| `pass` | 0 | — | Пропустить ход |

### Правила

- Каждый игрок начинает с **5 энергии** и одной **базы**
- **Одно действие за ход**
- Все действия разрешаются **одновременно** в границе раунда
- Пропущенное действие становится `pass` по таймауту
- Матч заканчивается после **MAX_TURNS** (по умолчанию 200)

### Энергия и здоровье

- Энергия: старт 5, +1 за ход (больше с ресурсными клетками), макс 20
- Здоровье клеток: обычные стартуют с 3, базы с 5, макс 7

## API

### Эндпоинты

| Метод | Путь | Описание |
|-------|------|----------|
| `GET` | `/health` | Проверка здоровья |
| `POST` | `/api/v1/join` | Регистрация игрока (`{"name":"..."}`) |
| `GET` | `/api/v1/state` | Получить наблюдение (`Authorization: Bearer TOKEN`) |
| `GET` | `/api/v1/leaderboard` | Получить таблицу лидеров и статус игры |
| `GET` | `/api/v1/results` | Получить финальные результаты (после окончания) |
| `POST` | `/api/v1/start` | Запустить игру |
| `POST` | `/api/v1/stop` | Остановить игру, сбросить состояние, отключить всех клиентов |
| `POST` | `/api/v1/pause` | Поставить игру на паузу (остановить обработку ходов) |
| `POST` | `/api/v1/resume` | Продолжить игру (возобновить обработку ходов) |

### WebSocket

```
ws://localhost:8080/ws?token=ВАШ_ТОКЕН
```

События: `CONNECTED`, `OBSERVATION`, `ACTION_ACCEPTED`, `TURN_RESOLVED`, `GAME_OVER`, `ERROR`, `STILL_ALIVE`

Формат действия:
```json
{
  "type": "ACTION",
  "turn": 0,
  "actions": [{"type": "expand", "target": {"x": 20, "y": 4}}]
}
```

## Запуск нескольких клиентов

### С помощью `run.sh`

```bash
./run.sh              # Сервер + все 15 клиентов (+ боты заполняют остаток)
./run.sh --no-bots    # Сервер + 15 клиентов (без ботов)
./run.sh --no-go      # Пропустить Go-клиентов
./run.sh --no-python  # Пропустить Python-клиентов
./run.sh --no-csharp  # Пропустить C#-клиентов
./run.sh --force-start # Запустить игру после регистрации клиентов
```

При старте игры сервер заполняет оставшиеся слоты встроенными ботами:
`число_ботов = max_players − зарегистрированные_клиенты`. При значении по
умолчанию `max_players: 30` и 15 клиентах это 15 ботов.

### Ручной запуск (по 5 копий каждого)

```bash
# Go
cd twclients/golang
for i in 1 2 3 4 5; do
  nohup ./bin/client --name "GoClient$i" &
done

# Python
cd twclients/python
for i in 1 2 3 4 5; do
  uv run python main.py --name "PythonClient$i" &
done

# C#
cd twclients/csharp
for i in 1 2 3 4 5; do
  dotnet exec bin/Release/net10.0/publish/TerritoryWars.Client.dll --name "CSharpClient$i" --strategy simple &
done
```

## Интерактивная консоль

`run.sh` предоставляет интерактивную консоль после запуска всех процессов:

```
=== Server Console ===
  1) start  — запустить игру
  2) stop   — остановить игру
  3) pause  — поставить на паузу
  4) resume — продолжить игру
  5) state  — показать состояние игры
  6) leaderboard — показать таблицу лидеров
  q) quit   — остановить все процессы
```

Команды отправляются через REST API к серверу.

## Viewer

Реалтайм-вьюер карты: `http://localhost:8080/viewer`

Использует Server-Sent Events (SSE) на `/api/v1/viewer/events`.

## Боты

Сервер автоматически создаёт ботов при `bots_enabled: true` в `twserver/config.json`. Боты создаются с ротацией стратегий и уникальными seed (base=42).

```json
{
  "port": 8080,
  "max_players": 30,
  "max_turns": 200,
  "width": 40,
  "height": 40,
  "turn_delay": "1s",
  "bots_enabled": true
}
```

Поддерживаемые стратегии: `simple`, `aggressive`, `defensive`, `expansion`, `random`.

## Написание собственной стратегии

### Python

```python
from client.strategy import IGameStrategy
from client.models import Action, Observation

class MyStrategy(IGameStrategy):
    def decide(self, observation: Observation) -> Action:
        # Ваш алгоритм здесь
        return Action(type="expand", target=...)
```

### C#

```csharp
using TerritoryWars.Models;
using TerritoryWars.Strategy;

public class MyStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        return new Action { Type = "expand", Target = new Point { X = 10, Y = 10 } };
    }
}
```

### Go

```go
package strategy

import "twclient/internal/models"

type MyStrategy struct{}

func (s *MyStrategy) Decide(obs *models.Observation) *models.Action {
    return &models.Action{Type: "expand", Target: &models.Point{X: 10, Y: 10}}
}
```

## Диагностика

| Ошибка | Решение |
|--------|---------|
| `Connection refused` | Сервер не запущен: `./bin/server -config config.json` |
| `401 Unauthorized` | Токен истёк. Перезапустите клиента. |
| `Stale turn` | Клиент слишком медленный. Нормально при нагрузке. |
| `Not enough energy` | Стратегия пытается действие без энергии. Используйте `pass`. |
| `Target outside map` | Координаты вне границ. Используйте `% Width` / `% Height` для wrap-around. |

## Логи

- Сервер: `logs/server.log` (поле `logs_dir` в `config.json`)
- Клиенты: stdout + log file (`logs_dir/player_name.log`)

Чтобы изменить папку логов, установите `logs_dir` в `config.json`:
```json
{ "logs_dir": "logs" }
```

## Совместимость

- Все клиенты используют один HTTP-протокол и JSON-контракт
- Кроссплатформенный запуск: Windows, Linux, macOS
- Новые клиенты не требуют модификации сервера
