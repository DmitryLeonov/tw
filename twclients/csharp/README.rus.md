# Territory Wars C# Client

C#-клиент для игрового сервера Territory Wars, реализующий ИИ-бота, играющего в пошаговую игру контроля территории.

## Назначение

Этот клиент подключается к игровому серверу Territory Wars, получает наблюдения за состоянием игры и отправляет стратегические действия через WebSocket. Он демонстрирует, как построить ИИ-агента для игры на C# и .NET 10.

## Архитектура

```
┌─────────────────────────────────────────────────┐
│                    Program.cs                    │
│  (Точка входа, аргументы CLI, DI, игровой цикл) │
├─────────────────────────────────────────────────┤
│                  GameLoop.cs                     │
│  (Основной игровой цикл: observe → decide → submit) │
├──────────────┬──────────────────────────────────┤
│  Api/        │  Strategy/                        │
│ GameApiClient│ IGameStrategy (интерфейс)         │
│              │ SimpleStrategy (реализация по ум.) │
├──────────────┴──────────────────────────────────┤
│                  Models/                         │
│  Observation, Cell, PublicPlayer, Action и др.  │
├─────────────────────────────────────────────────┤
│                  Tests/                          │
│  StrategyTests (юнит-тесты для SimpleStrategy)  │
└─────────────────────────────────────────────────┘
```

### Ключевые компоненты

- **Program.cs** — Точка входа. Парсит аргументы CLI, строит конфигурацию, настраивает DI и запускает игровой цикл с обработкой Ctrl+C.
- **GameLoop.cs** — Основной игровой цикл. Запрашивает наблюдения, делегирует стратегии и отправляет действия.
- **GameApiClient.cs** — HTTP и WebSocket-клиент. Обрабатывает регистрацию, опрос состояния, запросы таблицы лидеров и отправку действий.
- **IGameStrategy.cs** — Интерфейс стратегии. Определяет, как бот принимает решение о действии каждый ход.
- **SimpleStrategy.cs** — Реализация стратегии по умолчанию с базовой логикой расширения территории.
- **Models/** — Классы данных, соответствующие JSON-контракту сервера.
- **Tests/** — Юнит-тесты для логики стратегии.

## Установка

### Требования

- [.NET 10 SDK](https://dotnet.microsoft.com/download/dotnet/10.0) (10.0.0 или новее)
- `dotnet` в PATH (например, `/opt/dotnet`)

### Сборка

```bash
cd /apps/tw/twclients/csharp
dotnet restore
dotnet publish --configuration Release
```

Вывод: `bin/publish/TerritoryWars.Client.dll`

### Запуск

```bash
# Использовать конфиг по умолчанию (StudentBot на localhost:8080)
dotnet run

# Переопределить URL сервера и имя игрока
dotnet run -- --server http://localhost:8080 --name MyBot

# Использовать конкретный файл конфига
dotnet run -- --config config.json

# Указать стратегию
dotnet run -- --strategy Simple
```

### Запуск всех клиентов (Go + Python + C#)

```bash
# Go-клиенты
cd /apps/tw/twclients/golang
for i in 1 2 3 4 5; do
  nohup ./client --name "GoClient$i" > .logs/go_client_$i.log 2>&1 &
done

# Python-клиенты
cd /apps/tw/twclients/python
export PATH="/root/.local/bin:$PATH"
for i in 1 2 3 4 5; do
  nohup uv run python main.py --name "PythonClient$i" > .logs/python_client_$i.log 2>&1 &
done

# C#-клиенты
cd /apps/tw/twclients/csharp
export PATH="/opt/dotnet:$PATH"
for i in 1 2 3 4 5; do
  nohup dotnet bin/publish/TerritoryWars.Client.dll --name "CSharpClient$i" > .logs/csharp_client_$i.log 2>&1 &
done
```

## Примеры использования

### Базовое использование

```bash
dotnet run -- --server http://localhost:8080 --name TerritoryBot
```

### С файлом конфигурации

1. Отредактируйте `config.json`:

```json
{
  "ServerUrl": "http://game-server.example.com:8080",
  "PlayerName": "MyAwesomeBot",
  "HttpTimeoutSeconds": 15,
  "LogLevel": "Debug",
  "Strategy": "Simple"
}
```

2. Запуск:

```bash
dotnet run -- --config config.json
```

### Аргументы командной строки

| Аргумент | Описание | По умолчанию |
|----------|----------|--------------|
| `--server` | URL HTTP-сервера игры | `http://localhost:8080` |
| `--name` | Имя игрока для регистрации | `StudentBot` |
| `--config` | Путь к JSON-файлу конфигурации | `config.json` |
| `--strategy` | Используемая реализация стратегии | `Simple` |

## Конфигурация

### config.json

| Ключ | Тип | Описание | По умолчанию |
|------|-----|----------|--------------|
| `ServerUrl` | string | Базовый URL игрового сервера | `http://localhost:8080` |
| `PlayerName` | string | Отображаемое имя вашего бота | `StudentBot` |
| `HttpTimeoutSeconds` | int | Таймаут HTTP-запроса в секундах | `10` |
| `LogLevel` | string | Уровень логирования (Debug, Information, Warning, Error) | `Information` |
| `Strategy` | string | Используемая стратегия (`Simple`) | `Simple` |

### Переменные окружения

Значения конфигурации также можно переопределить через переменные окружения:

```bash
export TERRITORYWARS__SERVERURL="http://localhost:8080"
export TERRITORYWARS__PLAYERNAME="EnvBot"
```

## Как написать собственную стратегию

### Шаг 1: Реализуйте IGameStrategy

Создайте новый класс в папке `Strategy/`, реализующий `IGameStrategy`:

```csharp
using TerritoryWars.Models;
using TerritoryWars.Strategy;

namespace TerritoryWars.Strategy;

public class MyStrategy : IGameStrategy
{
    public Action Decide(Observation observation)
    {
        var owned = observation.GetOwnedCells();
        if (owned == null || owned.Count == 0)
            return new Action { Type = "pass" };

        // Expand в нейтральную территорию, затем атака, затем fortify
        foreach (var cell in owned)
        {
            foreach (var neighbor in GetNeighbors(cell, observation.Width, observation.Height))
            {
                var target = observation.Cells.FirstOrDefault(c => c.X == neighbor.X && c.Y == neighbor.Y);
                if (target != null && target.Owner == "" && observation.Energy >= 1)
                {
                    return new Action
                    {
                        Type = "expand",
                        Target = new Point { X = neighbor.X, Y = neighbor.Y }
                    };
                }
            }
        }

        return new Action { Type = "pass" };
    }

    private static IEnumerable<Point> GetNeighbors(Cell cell, int width, int height)
    {
        yield return new Point { X = (cell.X - 1 + width) % width, Y = cell.Y };
        yield return new Point { X = (cell.X + 1 + width) % width, Y = cell.Y };
        yield return new Point { X = cell.X, Y = (cell.Y - 1 + height) % height };
        yield return new Point { X = cell.X, Y = (cell.Y + 1 + height) % height };
    }
}
```

### Шаг 2: Зарегистрируйте стратегию

В `Program.cs` добавьте стратегию в switch-выражение:

```csharp
IGameStrategy strategy = strategyName.ToLower() switch
{
    "simple" => new SimpleStrategy(),
    "my_strategy" => new MyStrategy(),
    _ => new SimpleStrategy() // fallback на simple
};
```

### Шаг 3: Запустите со своей стратегией

```bash
dotnet run -- --strategy MyStrategy
```

## Справочник игровой механики

### Типы действий

| Действие | Описание | Стоимость энергии |
|----------|----------|-------------------|
| `expand` | Захватить нейтральную клетку рядом с вашей | 1 |
| `attack` | Атаковать вражескую клетку рядом с вашей | 2 |
| `fortify` | Укрепить одну из своих клеток | 2 |
| `pass` | Пропустить ход | 0 |

### Типы клеток

| Тип | Описание |
|-----|----------|
| `base` | Стартовая позиция, высокое здоровье |
| `normal` | Обычная клетка территории |
| `resource` | Ресурсная клетка (отмечена визуально) |

### Правила перемещения

- Действия могут targeting только клетки, **соседние** (Манхэттенское расстояние = 1) к вашим клеткам.
- Карта **тороидальная** — координаты wrap-around через `% Width` и `% Height`.
- Вам нужна достаточная **энергия** для выполнения действий.
- Каждый ход вы отправляете **одно действие**.

## Диагностика

### Connection Refused

```
WebSocket connection failed: Connection refused
```

- Убедитесь, что сервер запущен: `curl http://localhost:8080/health`
- Проверьте `ServerUrl` в конфиге на соответствие адресу сервера.
- Убедитесь, что фаервол не блокирует порт.

### Authentication Errors

```
HTTP 401 Unauthorized
```

- Токен мог истечь. Перезапустите клиент для перерегистрации.
- Убедитесь, что сервер принимает новых игроков.

### Strategy Errors

```
NullReferenceException в Decide()
```

- Убедитесь, что стратегия обрабатывает крайние случаи: пустые списки клеток, устранённые игроки, нулевая энергия.
- Проверьте, что возвращённые действия имеют допустимые координаты `Target` в пределах карты.

### WebSocket Disconnections

```
WebSocket closed unexpectedly
```

- Сервер мог таймаутнуть ваше соединение. Перезапустите клиент.
- Проверьте стабильность сети.
- Клиент попытается переподключиться при следующем цикле наблюдений.

### Build Errors

```
error NETSDK1004: File not found
```

- Убедитесь, что .NET 10 SDK установлен: `dotnet --version`
- Запустите `dotnet restore` для скачивания пакетов.

## Структура проекта

```
twclients/csharp/
├── TerritoryWars.Client.csproj
├── README.md
├── README.rus.md
├── config.json
├── Program.cs                    # Точка входа
├── Api/
│   └── GameApiClient.cs          # HTTP + WebSocket-клиент
├── Game/
│   └── GameLoop.cs               # Основной игровой цикл
├── Models/
│   ├── Observation.cs            # Модель состояния игры
│   ├── Cell.cs                   # Модель клетки
│   ├── PublicPlayer.cs           # Модель информации об игроке
│   ├── Action.cs                 # Модель действия
│   ├── ActionMessage.cs          # Модель сообщения WebSocket
│   ├── JoinResponse.cs           # Модель ответа регистрации
│   ├── ServerEvent.cs            # Модель события сервера
│   └── Result.cs                 # Модель результата API
├── Strategy/
│   ├── IGameStrategy.cs          # Интерфейс стратегии
│   └── SimpleStrategy.cs         # Стратегия по умолчанию
└── Tests/
    └── StrategyTests.cs          # Юнит-тесты (исключены из билда)
```
