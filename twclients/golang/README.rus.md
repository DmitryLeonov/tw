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
go build -o client ./cmd/client/

# Запустите его
./client
```

## Использование

### Базовое использование

```bash
# Использовать конфиг по умолчанию (config.json)
./client

# Указать URL сервера
./client --server http://localhost:8080

# Указать имя игрока
./client --name MyBot

# Указать файл конфига
./client --config my-config.json

# Установить уровень логирования
./client --log-level DEBUG
```

### Все флаги

```
./client --help

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
  "strategy": "simple"
}
```

## Конфигурация

### Параметры config.json

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `server` | string | `http://localhost:8080` | URL игрового сервера |
| `player_name` | string | `StudentBot` | Отображаемое имя игрока |
| `http_timeout` | duration | `10s` | Таймаут HTTP-запроса |
| `log_level` | string | `INFO` | Уровень логирования: DEBUG, INFO, WARN, ERROR |
| `strategy` | string | `simple` | Используемая стратегия (сейчас: `simple`) |

### Приоритет CLI-флагов

Флаги командной строки переопределяют значения из файла конфига:

```bash
# Это переопределит player_name из config.json
./client --name OverrideName --log-level DEBUG
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
// internal/strategy/aggressive.go
package strategy

import (
    "log/slog"
    "twclient/internal/models"
)

type AggressiveStrategy struct{}

func NewAggressiveStrategy(logger *slog.Logger) *AggressiveStrategy {
    return &AggressiveStrategy{}
}

func (s *AggressiveStrategy) Decide(obs *models.Observation) *models.Action {
    // Ваша логика здесь
    return nil // nil означает "pass"
}
```

2. **Проанализируйте наблюдение**:

```go
func (s *AggressiveStrategy) Decide(obs *models.Observation) *models.Action {
    playerID := obs.PlayerID

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

    // Найдите нейтральные клетки рядом с вашей территорией
    for _, cell := range obs.Cells {
        if cell.Owner == "" && obs.Energy >= 1 {
            return &models.Action{
                Type:   "expand",
                Target: &models.Point{X: cell.X, Y: cell.Y},
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
    case "aggressive":
        return strategy.NewAggressiveStrategy(logger)
    default:
        return strategy.NewSimpleStrategy(logger)
    }
}
```

4. **Обновите конфиг**:

```json
{
  "strategy": "aggressive"
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
Error: action failed with status 400: stale turn: current turn is 5
```

**Решение:** Ваш клиент слишком медленный. Сервер уже перешёл к следующему ходу. Это нормально при высокой задержке. Клиент повторит попытку при следующем опросе.

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
./client --log-level DEBUG
```

Это показывает каждое решение, отправку действия и разрешение хода.

## Лицензия

Этот клиент является частью проекта Territory Wars.
