# Territory Wars — Правила серверной части

## 1. Игровой движок (Engine)

**Правила Engine:**

1. Все поля защищены `sync.RWMutex`.
2. `Token` — 48-символьный hex (randomHex(24) → crypto/rand).
3. `tokenToPlayer` map связывает токен с playerID.
4. `pending` map — действия, ожидающие разрешения.
5. `gameOver` и `stopped` флаги управляют жизненным циклом.
6. `turnChanged` channel пересоздаётся после каждого разрешения хода.

## 2. RoundManager

**Правила RoundManager:**

1. Один `run()` goroutine — единственный обработчик действий.
2. Per-player action channels (`actionCh`) — WS-хендлер пишет в канал, `run()` читает.
3. Generation counter — предотвращает обработку действий от старых соединений.
4. Bounded outbound queue (capacity 256) — переполнение → disconnect.
5. `cleanupAll()` — закрывает все WS-соединения при `StopGame`.

## 3. Встроенные боты

**Правила ботов:**

1. Боты НЕ запускаются автоматически при старте сервера.
2. Боты запускаются через `StartBotsOnDemand()` при вызове `StartGame()`.
3. `BuildBotList(enabled, maxPlayers)` — создаёт агентов с ротацией стратегий и уникальными seed (base=42).
4. Имена ботов: `Bot-aggressive-1`, `Bot-defensive-2`, `Bot-expansion-3`, `Bot-random-4`, `Bot-simple-5`.
5. Боты реализуют `PlayerAgent`, а не `IGameStrategy`.
6. `StrategySimple` определён в `bot_config.go` но не используется в `BuildBotList`.

## 4. HTTP-сервер

**Правила HTTP:**

1. Все POST-эндпоинты проверяют `r.Method != http.MethodPost`.
2. `writeJSON()` — единая функция для JSON-ответов.
3. `bearer(r)` — извлечение токена из `Authorization` header.
4. `http.MaxBytesReader(w, r.Body, 4096)` — ограничение размера тела запроса.
5. `signal.Ignore(syscall.SIGINT, syscall.SIGTERM)` — сервер игнорирует сигналы для выживания при kill harness.
6. CORS `Access-Control-Allow-Origin: *` — только для локальной разработки.

## 5. Viewer

**Правила Viewer:**

1. `viewer.html` встраивается через `//go:embed viewer.html`.
2. SSE использует `EventSource` в браузере.
3. `StartMonitoring(ctx)` — фоновый goroutine, транслирующий обновления каждые 5 секунд.
4. Viewer не требует аутентификации.
5. `buildViewerState()` собирает данные из Engine и BotManager.

## 6. Жизненный цикл игры

**Правила жизненного цикла:**

1. Игра НЕ начинается автоматически.
2. `StartGame()` запускает ботов для заполнения оставшихся слотов.
3. `StopGame()` сбрасывает ВСЁ: токены, соединения, состояние, ботов.
4. После `StopGame` клиенты должны заново пройти `Join` для получения нового токена.
5. `gameOverHandler` — после окончания игры сервер ждёт Enter для остановки.

## 7. Тестирование

**Правила тестирования:**

1. Все тесты должны проходить с `-race`.
2. Интеграционные тесты используют `httptest.Server` — без реального HTTP-сервера.
3. Тесты RoundManager не используют реальные WS-соединения (conn=nil).
4. Тесты ботов проверяют все стратегии и edge cases.
5. Тесты конфигурации проверяют defaults, overrides, invalid input, path resolution.

## 8. Логирование

**Правило:** Все логи сервера идут через `log.Printf` / `log.Fatalf`. Не используйте `fmt.Println` для логов.

## 9. Известные ограничения MVP

1. Регистрация открыта и возвращает токены публично. Для экзамена — предсоздать список или защитить admin key.
2. `CheckOrigin` и CORS пермиссивны для локальной разработки.
3. Нет persistence, TLS, rate limiting, replay storage, admin authentication.
4. Наблюдение раскрывает все клетки. Добавить fog-of-war при необходимости.
5. Текущий ruleset намеренно мал. Нужен playtesting для баланса.
6. Round coordinator использует bounded outbound queue. Клиент, который не потребляет события, disconnects.
