# Territory Wars Go Client

Go-клиент для сервера игры Territory Wars. Этот клиент подключается к серверу, регистрирует игрока и играет в игру с помощью настраиваемой стратегии.

## Назначение

Этот клиент демонстрирует взаимодействие с игровым сервером Territory Wars на Go. Он обрабатывает:

- **Регистрация игрока** через HTTP POST на `/api/v1/join`
- **Опрос состояния** через HTTP GET на `/api/v1/state`
- **Отправка действий** через WebSocket на `/ws`
- **Запросы таблицы лидеров** через HTTP GET на `/api/v1/leaderboard`
- **Корректное завершение** при SIGINT/SIGTERM

## Архитектура

```
┌─────────────────────────────────────────────────┐
│                  cmd/client                     │
│  Точка входа: парсинг флагов, загрузка конфига, │
│  обработка сигналов, оркестрация игрового цикла  │
├─────────────────────────────────────────────────┤
│              internal/config                    │
│  Загрузка JSON-конфига с настройками по умолчанию│
│  и переопределением через CLI-флаги              │
├─────────────────────────────────────────────────┤
│               internal/models                   │
│  Типы данных: Observation, Action, Cell,        │
│  PublicPlayer, ActionMessage, JoinResponse      │
├─────────────────────────────────────────────────┤
│                internal/api                     │
│  HTTP-клиент для state/leaderboard,             │
│  WebSocket-клиент для отправки действий         │
├─────────────────────────────────────────────────┤
│             internal/strategy                   │
│  Интерфейс стратегии + SimpleStrategy           │
│  (эвристики expand, attack, fortify, pass)      │
├─────────────────────────────────────────────────┤
│               internal/game                     │
│  Игровой цикл: connect → observe → decide →     │
│  submit → poll → повтор до конца игры           │
└─────────────────────────────────────────────────┘
```

### Ответственность компонентов

| Пакет | Ответственность |
|-------|-----------------|
| `cmd/client` | CLI-точка входа, загрузка конфига, обработка сигналов |
| `config` | Парсинг JSON-конфига с настройками по умолчанию |
| `models` | JSON-сериализуемые игровые типы данных |
| `api` | HTTP-запросы и WebSocket-соединения |
| `strategy` | Логика принятия решений (плагиновая) |
| `game` | Оркестрация основного игрового цикла |

## Установка

### Требования

- **Go 1.22+** ([скачать](https://go.dev/dl/))
- Запущенный игровой сервер Territory Wars

### Настройка

```bash
# Клонируйте или скопируйте клиент в рабочую область
cd /apps/tw/twclients/golang

# Скачайте зависимости
go mod tidy

# Соберите клиент
go build -o bin/client ./cmd/client/

# Запустите его
./bin/client
```

## Использование

### Базовое использование

```bash
# Использовать конфиг по умолчанию (config.json)
./bin/client

# Указать URL сервера
./bin/client --server http://localhost:8080

# Указать имя игрока
./bin/client --name MyBot

# Указать файл конфига
./bin/client --config my-config.json

# Установить уровень логирования
./bin/client --log-level DEBUG
```

### Все флаги

```
./bin/client --help

Flags:
  --config string    Путь к JSON-файлу конфига (default "config.json")
  --log-level string Уровень логирования: DEBUG, INFO, WARN, ERROR
  --name string      Имя игрока
  --server string    URL игрового сервера (например, http://localhost:8080)
```

### Использование файла конфига

Отредактируйте `config.json`:

```json
{
  "server": "http://localhost:8080",
  "player_name": "MyAwesomeBot",
  "http_timeout": 10,
  "log_level": "INFO",
  "logs_dir": "logs",
  "strategy": "simple",
  "max_retries": 10
}
```

## Конфигурация

### Параметры config.json

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `server` | string | `http://localhost:8080` | URL игрового сервера |
| `player_name` | string | `StudentBot` | Отображаемое имя игрока |
| `http_timeout` | int | `10` | Таймаут HTTP-запроса и WebSocket-хендшейка в секундах |
| `log_level` | string | `INFO` | Уровень логирования: DEBUG, INFO, WARN, ERROR |
| `logs_dir` | string | `""` (без файла) | Каталог для `logs_dir/имя_игрока.log` |
| `strategy` | string | `simple` | Используемая стратегия (сейчас: `simple`) |
| `max_retries` | int | `10` | Максимум переподключений WebSocket перед завершением клиента |

### Приоритет CLI-флагов

Флаги командной строки переопределяют значения из файла конфига:

```bash
# Это переопределит player_name из config.json
./bin/client --name OverrideName --log-level DEBUG
```

## Как написать собственную стратегию

### Интерфейс стратегии

Все стратегии реализуют этот интерфейс:

```go
package strategy

import "twclient/internal/models"

type Strategy interface {
    Decide(observation *models.Observation) *models.Action
}
```

### Пошаговое: создание новой стратегии

1. **Создайте новый файл** в `internal/strategy/`:

```go
// internal/strategy/my_strategy.go
package strategy

import (
    "log/slog"
    "twclient/internal/models"
)

type MyStrategy struct{}

func NewMyStrategy(logger *slog.Logger) *MyStrategy {
    return &MyStrategy{}
}

func (s *MyStrategy) Decide(obs *models.Observation) *models.Action {
    // Ваша логика здесь
    return nil // nil означает "pass"
}
```

2. **Проанализируйте наблюдение**:

```go
func (s *MyStrategy) Decide(obs *models.Observation) *models.Action {
    playerID := obs.PlayerID

    // Найдите нейтральные клетки рядом с вашей территорией
    for _, cell := range obs.Cells {
        if cell.Owner == "" && obs.Energy >= 1 {
            return &models.Action{
                Type:   "expand",
                Target: &models.Point{X: cell.X, Y: cell.Y},
            }
        }
    }

    // Найдите вражеские клетки рядом с вашей территорией
    for _, cell := range obs.Cells {
        if cell.Owner != "" && cell.Owner != playerID {
            // Это вражеская клетка — рассмотрите атаку
            if cell.Health <= 2 && obs.Energy >= 2 {
                return &models.Action{
                    Type:   "attack",
                    Target: &models.Point{X: cell.X, Y: cell.Y},
                }
            }
        }
    }

    // Нечего делать
    return nil
}
```

3. **Зарегистрируйте стратегию** в `cmd/client/main.go`:

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

4. **Обновите конфиг**:

```json
{
  "strategy": "my_strategy"
}
```

### Справочник типов действий

| Действие | Стоимость энергии | Требуется цель | Описание |
|----------|-------------------|----------------|----------|
| `expand` | 1 | Да (нейтральная клетка) | Захватить нейтральную клетку рядом с вашей территорией |
| `attack` | 2 | Да (вражеская клетка) | Атаковать вражескую клетку рядом с вашей территорией |
| `fortify` | 2 | Да (ваша клетка) | Увеличить здоровье вашей клетки (макс 7) |
| `pass` | 0 | Нет | Пропустить ход |

### Ключевые игровые механики

- **Энергия**: старт 5, увеличивается на 1 за ход (больше с ресурсными клетками), макс 20
- **Здоровье клеток**: старт 3 для обычных, 5 для баз, макс 7
- **Соседство**: цель должна быть соседней (вверх/вниз/влево/вправо) к вашей территории
- **Лимит ходов**: по умолчанию 200
- **Очки**: +1 за expand, +3 за attack (завоевание), -2 за атакованного владельца клетки
- **Тороидальная карта**: координаты wrap-around через `% Width` и `% Height`

## Диагностика

### Connection refused

```
Error: join request failed: Post "http://localhost:8080/api/v1/join": dial tcp 127.0.0.1:8080: connect: connection refused
```

**Решение:** Убедитесь, что сервер запущен:

```bash
cd /apps/tw/twserver
go build -o bin/server ./cmd/server/
./bin/server -config config.json
```

### Unauthorized Error

```
Error: observation failed with status 401: unauthorized
```

**Решение:** Токен мог истечь или быть невалидным. Перезапустите клиент.

### Stale Turn Error

```
stale turn: current turn is 5
```

**Решение:** Сервер отклонил действие, потому что ход уже сменился. Действия
отправляются по WebSocket, поэтому такая ошибка приходит как WebSocket-событие
`ERROR`, а не как HTTP-код. Go-клиент не реагирует на `ERROR` — он заново читает
состояние через `GET /api/v1/state` и отправляет новое действие для текущего хода.
Это нормальная ситуация при высокой задержке.

### Strategy Not Found

```
Warning: unknown strategy, falling back to simple
```

**Решение:** Проверьте поле `strategy` в `config.json`. Сейчас поддерживается: `simple`.

### High Memory Usage

Клиент загружает всё состояние игры (все клетки) в память каждый ход. Для карты 40×40 это 1600 клеток на наблюдение, что ничтожно. Если видите проблемы, проверьте утечки горутин в пользовательских стратегиях.

### Отладка

Запустите с `--log-level DEBUG` для подробного вывода:

```bash
./bin/client --log-level DEBUG
```

Это показывает каждое решение, отправку действия и разрешение хода.

## Лицензия

Этот клиент является частью проекта Territory Wars.
