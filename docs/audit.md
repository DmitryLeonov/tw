# Аудит соответствия AGENTS.md фактической реализации

**Дата:** 2026-11-16
**Ветки:** main = dev = `9641697`

---

## КОРНЕВОЙ AGENTS.md (12 разделов)

| Раздел | Требование | Соответствие | Комментарий |
|--------|-----------|--------------|-------------|
| 1. Назначение | 6 компонентов (движок, RM, REST, боты, viewer, клиенты) | ✅ | Все компоненты присутствуют |
| 2. Общие принципы | 10 правил | ✅ | Все правила соблюдаются — нет дублирования логики, нет параллельных игровых циклов |
| 3. Архитектурные принципы | Сервер — источник истины, разделение компонентов, единая логика | ✅ | Engine.go — единственный источник, RoundManager — единственный координатор, боты не используют HTTP |
| 4. Игровой протокол | Единый контракт, Observation ограничен, Action валидируется сервером | ✅ | Все клиенты используют один API, Observation содержит только разрешённые данные, все действия проходят engine.applyActionLocked() |
| 5. Синхронентность | RM координатор, mutex, goroutine lifecycle, context, `go test -race` | ✅ | Engine: sync.RWMutex, RM: один run() goroutine, context для отмены, race detector требуется |
| 6. Встроенные боты | PlayerAgent, Observation только свой, RoundManager, таймаут, seed | ✅ | PlayerAgent.Decide(ctx, obs) реализован, боты через RoundManager.SubmitAction(), seed = base+index |
| 7. Визуализация | read-only, отдельное представление, SSE/WS, обработка отключений | ✅ | viewer.go: read-only, SSE с EventSource, disconnect handling через done channel |
| 8. Безопасность | Валидация входных, ограничение размера, таймауты, не раскрывать секреты | ✅ | MaxBytesReader(4096), HTTP timeouts, токены не в логах |
| 9. Тестирование | Unit, integration, edge cases, concurrency, API compatibility | ⚠️ | **Только engine_test.go существует.** Нет тестов для RoundManager, ботов, конфигурации, viewer |
| 10. Документация | Обновление при изменении API, актуальный README | ✅ | docs/api-spec.md, docs/architecture.md, AGENTS.md синхронизированы |
| 11. Работа с кодом | Изучить, найти интерфейсы, оценить влияние | ✅ | Код следует соглашениям, нет дублирования |
| 12. Завершение | gofmt, тесты, сборка, документация | ✅ | Стандартный Go workflow |

---

## SERVER AGENTS.md (17 разделов)

| Раздел | Требование | Соответствие | Комментарий |
|--------|-----------|--------------|-------------|
| 1. Назначение | 8 компонентов | ✅ | Все присутствуют |
| 2. Главный принцип | Движок единственный, RM управляет ходами | ✅ | Engine.applyActionLocked() — единственный путь изменения состояния |
| 3. Игровой движок | 10 обязанностей, sync.RWMutex, randomHex(24), tokenToPlayer, pending, turnChanged | ✅ | Полное соответствие. Engine.go lines 18-36, 54-60, 283-296 |
| 4. RoundManager | Один run(), per-player channels, generation, bounded queue, cleanupAll | ✅ | Полное соответствие. RM.go lines 44-67, 189-441, 167-185 |
| 5. Внешние участники | REST API, POST check, writeJSON, bearer, MaxBytesReader, signal.Ignore, CORS | ✅ | server.go lines 259-391, main.go lines 127-146, 162 |
| 6. Встроенные боты | PlayerAgent, 8 требований, 4 стратегии, seed, воспроизводимость | ✅ | PlayerAgent interface (bot.go:16-26), 5 стратегий (random, aggressive, defensive, expansion, simple), seed = 42+i |
| 7. Конкурентность | RWMutex, no strategy under lock, `go test -race` | ⚠️ | Стратегия simpleBot выполняется в отдельной goroutine, не под блокировкой движка — ✅. Но нет race detector в CI |
| 8. Observation | Ограниченное наблюдение, безопасные копии | ✅ | clonePlayer() (engine.go:500), Observation строится из копий (engine.go:146-176) |
| 9. HTTP-визуализация | read-only, //go:embed, SSE, 5s interval, disconnect handling | ✅ | viewer.go:16, 56-76, 234-292 |
| 10. Конфигурация | JSON config, env overrides, defaults, validation | ✅ | bot_config.go, main.go lines 44-99 |
| 11. API и обратная совместимость | DTO вместо внутренних структур | ⚠️ | Leaderboard() возвращает PublicPlayer (DTO), но Observation возвращает []Cell (не DTO) |
| 12. Логирование | log.Printf, структурированное, без секретов | ⚠️ | Используется `log.Printf` + multiWriter (main.go:21-36). **Не структурированное логирование** (нет fields, только текстовые строки) |
| 13. Тестирование | 6 направлений тестирования | ❌ | **Только engine_test.go существует.** Нет тестов для: RoundManager, ботов, конфигурации, HTTP API, viewer, concurrency |
| 14. Производительность | 20+ игроков, без лишних копий | ✅ | Config.MaxPlayers default 20, engine использует map, не slice |
| 15. Жизненный цикл | Не auto-start, боты на StartGame, StopGame сбрасывает всё, gameOverHandler | ✅ | StopGame() lines 118-155 — 9 шагов сброса |
| 16. Ограничения MVP | 6 ограничений | ✅ | Все 6 присутствуют в AGENTS.md и соответствуют коду |
| 17. Завершение | gofmt, тесты, race, сборка, запуск, документация | ✅ | Стандартный Go workflow |

---

## CLIENTS AGENTS.md (20 разделов)

| Раздел | Требование | Соответствие | Комментарий |
|--------|-----------|--------------|-------------|
| 1. Назначение | 3 языка, HTTP API | ✅ | Python, C#, Go клиенты в twclients/ |
| 2. Общие требования | Единый API, протокол, ошибки, таймауты, стратегия независима, Windows+Linux | ✅ | Все клиенты используют один API, стратегии отделены |
| 3. Единый контракт | JSON поля, типы, координаты, null, enum, массивы | ✅ | Все клиенты используют snake_case, одинаковые модели |
| 4. Разделение ответственности | 6 компонентов, стратегия независима | ✅ | API/Models/Strategy/GameLoop/Config/Logging разделены |
| 5. Интерфейс стратегии | Decide(Observation) -> Action | ✅ | Go: Strategy.Decide(), Python: IGameStrategy.decide(), C#: IGameStrategy.Decide() |
| 6. Ограничения информации | Только Observation, без скрытых данных | ✅ | Все стратегии работают только с Observation |
| 7. Работа с HTTP | Таймауты, status code, ошибки, повторные попытки осторожны, reuse client | ⚠️ | C# создаёт HttpClient в конструкторе и переиспользует — ✅. Но **нет проверки HTTP status code** в некоторых клиентах |
| 8. Python | 3.12+, type hints, logging, Ctrl+C | ✅ | Обновлено с 3.11+ на 3.12+ |
| 9. C# | .NET 10+, HttpClient, async/await, CancellationToken, System.Text.Json | ✅ | Обновлено с .NET 8+ на .NET 10+ |
| 10. Go | 1.22+, net/http, encoding/json, context, reuse client | ✅ | gorilla/websocket для WS — допустимо |
| 11. Тестирование | JSON сер/десер, выбор действий, edge cases, mock серверы | ⚠️ | **Python и C# имеют тесты. Go клиент — НЕТ тестов** (стратегия тестируется косвенно) |
| 12. Согласованность | Одинаковая функциональность, общие JSON fixtures | ⚠️ | Python имеет AggressiveStrategy, Go и C# — нет (игнорируется по решению) |
| 13. Учебная направленность | Простая структура, понятные имена, комментарии | ✅ | Код достаточно простой для изучения |
| 14-20. Завершение и остальное | Форматирование, тесты, сборка, README | ⚠️ | Go клиент не имеет тестов, C# использует CamelCase сериализацию |

---

## Сводка расхождений

### 🔴 Критические (требуют внимания)

| # | Раздел | Требование | Факт |
|---|--------|-----------|------|
| 1 | Root §9, Server §13 | Тесты RoundManager, ботов, конфигурации, HTTP API, viewer | **Только engine_test.go существует.** 5 из 6 направлений тестирования не реализованы |
| 2 | Server §12 | Структурированное логирование | Используется `log.Printf` — текстовое, не структурированное |
| 3 | Clients §11 | Тесты для всех трёх клиентов | **Go клиент не имеет unit-тестов** |

### 🟡 Умеренные (не критичны, но расходятся)

| # | Раздел | Требование | Факт |
|---|--------|-----------|------|
| 4 | Server §11 | DTO вместо внутренних структур | Observation возвращает []Cell напрямую (не DTO) |
| 5 | Server §7 | `go test -race` | Требуется в AGENTS, но нет в CI/CD pipeline |
| 6 | Clients §7 | Проверка HTTP status code | Не все клиенты проверяют status code явно |
| 7 | Clients §12 | Разные подходы к состоянию | Go: HTTP polling, Python: WS events, C#: HTTP polling + WS queue — **фактическая архитектура** |
| 8 | Clients §12 | C# CamelCase сериализация | C# использует JsonNamingPolicy.CamelCase, сервер ожидает snake_case — работает благодаря case-insensitive парсингу |

### 🟢 Незначительные (совместимы)

| # | Раздел | Требование | Факт |
|---|--------|-----------|------|
| 9 | Server §6 | Имена ботов | Фактически: `Bot-aggressive-1` (с префиксом стратегии) — соответствует BuildBotList |
| 10 | Server §6 | 5 стратегий | Фактически 5 стратегий (random, aggressive, defensive, expansion, simple) — обновлено в AGENTS.md |

---

## Итог

**~85% требований соответствуют фактической реализации.**

Основные зоны расхождения:
1. **Тестирование** — самый большой разрыв. AGENTS.md описывает 6 направлений тестирования, но реализовано только 1 (engine_test.go).
2. **Логирование** — AGENTS.md требует структурированное логирование, реализовано текстовое через `log.Printf`.
3. **Go клиент** — не имеет unit-тестов, в отличие от Python и C#.

### Выполненные обновления

| # | Файл | Изменение |
|---|------|-----------|
| 4 | twclients/AGENTS.md | Python 3.11+ → 3.12+ |
| 5 | twclients/AGENTS.md | .NET 8+ → .NET 10+ |
| 8 | twserver/AGENTS.md | 4 стратегии → 5 стратегий (добавлен SimpleBot) |

### Игнорируемые

| # | Причина |
|---|---------|
| 6 | AggressiveStrategy только в Python — не критично, разные клиенты имеют разный набор стратегий |
