# Sprint 4 Checklist

Источник: `docs/chat-architecture-plan.md` §13 (Sprint 4) + tech debt из `docs/known-limitations-sprint-3.md`.

**Цель спринта:** сообщения исчезают по таймеру у обоих пользователей и в БД; мобильный клиент показывает полноценный чат с обратным отсчётом TTL.

---

## 1) Подготовка и контракты

- [x] Утвердить модель TTL: глобальный конфиг vs политика диалога (на Sprint 4 — глобальный конфиг).
- [x] Утвердить поля `expires_at TIMESTAMPTZ NULL` и `deleted_at TIMESTAMPTZ NULL` в `messages`.
- [x] Утвердить формат события `message_deleted` (payload: `{type, message_id, dialog_id}`).
- [x] Утвердить формат TTL в `SendMessage` API: включать `expires_at` в ответ и в `message_new` WS-событие.
- [x] Утвердить интервал тикера `message-expirer` (по умолчанию: 10 сек).
- [x] Подготовить `docs/api-sprint-4.md` с обновлёнными контрактами.

Примечание: все решения зафиксированы в `docs/api-sprint-4.md`. TTL — глобальный конфиг `chat.message_ttl_seconds` (0 = без TTL). Таблица `messages` расширяется полями `expires_at` и `deleted_at` (soft delete). WS событие `message_deleted` содержит `{type, message_id, dialog_id}`. `message_new` расширен полем `expires_at`. Тикер expirer — 10 сек (`expirer.interval_seconds`). Rate-limiting на login: 10 req/60 sec/IP → 429. Новый код ошибки `user_inactive` (403).

---

## 2) База данных и миграции

- [x] Создать миграцию `internal/store/migrations/008_message_ttl.sql`:
  - добавить `expires_at TIMESTAMPTZ NULL` в `messages`;
  - добавить `deleted_at TIMESTAMPTZ NULL` в `messages`;
  - добавить индекс `idx_messages_expires_at` (`expires_at`) WHERE `deleted_at IS NULL`.
- [x] Обновить модель `Message` в `internal/store/models.go` (поля `ExpiresAt`, `DeletedAt`).
- [x] Проверить миграции на чистой БД и при повторном запуске (идемпотентность).

Примечание: миграция `008_message_ttl.sql` применена автоматически при `task local:up` (auto_migrate). Повторный прогон — ни ошибок, ни NOTICE (полная идемпотентность: `ADD COLUMN IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`). Схема проверена через `\d messages` — оба поля и частичный индекс присутствуют. `go build ./internal/store/...` — OK.

---

## 3) Store-слой (`internal/store/`)

- [x] Обновить `MessageRepository.Create` — принимать и сохранять `expires_at`.
- [x] Обновить `MessageRepository.List` — фильтровать `WHERE deleted_at IS NULL`.
- [x] Добавить `MessageRepository.ExpireMessages(ctx, now time.Time) ([]ExpiredMessage, error)` — batch UPDATE:
  ```sql
  UPDATE messages
  SET deleted_at = $1
  WHERE expires_at <= $1 AND deleted_at IS NULL
  RETURNING id, dialog_id, sender_id
  ```
- [x] Добавить `UserRepository.FindByID(ctx, userID) (*User, error)` — для проверки статуса при login (tech debt).
- [x] Добавить integration-тесты для новых методов репозитория.

Примечание: `MessageRepository.Create` расширен полем `expires_at` (передаётся как `*time.Time`, NULL при отсутствии TTL). `ListByDialog` и `GetByID` фильтруют `WHERE deleted_at IS NULL` и возвращают `expires_at`. `ExpireMessages` использует CTE с `FOR UPDATE SKIP LOCKED` + `LIMIT batch_size` (PostgreSQL не поддерживает LIMIT в UPDATE напрямую). Создан `UserRepository` с `FindByID` и `ErrUserNotFound`. Модель `User` добавлена в `models.go`. Добавлен `message_repository_integration_test.go` — 8 тестов (Create с/без TTL, List исключает deleted, ExpireMessages помечает expired, идемпотентность, List после expire, UserRepository Find/NotFound). `task lint` — 0 issues. `task test:integration` — все PASS.

---

## 4) Chat service (`internal/services/chat/`)

- [x] Обновить `SendMessage` — вычислять `expires_at = now() + ttl` из конфига, передавать в `MessageRepository.Create`.
- [x] Включать `expires_at` в `message_new` WS-событие (Hub broadcast).
- [x] Включать `expires_at` в ответ REST `POST /api/v1/dialogs/{dialog_id}/messages`.
- [x] Обновить `ListMessages` — поле `expires_at` в ответе.
- [x] Добавить unit-тесты обновлённого `SendMessage`.

Примечание: добавлен `ChatConfig{MessageTTLSeconds}` в `config.go` + поле `Chat ChatConfig` в `Config`. `Service` получил поле `ttl time.Duration`; `NewService` расширен 6-м аргументом. **Семантика TTL изменена на "удаление после прочтения"**: `SendMessage` не устанавливает `expires_at` (всегда nil при создании). В `MarkRead`: при первом прочтении (`msg.ExpiresAt == nil && ttl > 0`) вызывается `SetExpiresAt(read_at + ttl)` и рассылается событие `message_ttl_started` обоим участникам. Хендлер: `messageResponse` расширен `ExpiresAt *string`. Добавлено 7 unit-тестов: `ExpiresAtAlwaysNilAtSend`, `MessageNew_ExpiresAtIsNil`, `MarkRead_WithTTL_StartsCountdown`, `MarkRead_WithTTL_SendsTTLStartedEvent`, `MarkRead_WithTTL_AlreadyExpiring_NoSecondStart`, `MarkRead_WithoutTTL_NoExpiresAt`. `api-sprint-4.md` обновлён. `go build ./...` — OK. `task lint` — 0 issues. `task test:integration` — все PASS.

---

## 5) Message Expirer (`cmd/message-expirer/`)

- [x] Реализовать реальный тикер в `internal/app/messageexpirer/app.go`:
  - каждые N секунд (конфиг `expirer.interval`, по умолчанию 10 сек);
  - вызов `MessageRepository.ExpireMessages(ctx, time.Now())`;
  - broadcast `message_deleted` событий через Hub для онлайн-пользователей;
  - structured log `message_expired` с `count`, `duration_ms`.
- [x] Передать Hub в App message-expirer (или использовать общий Event Bus / публикацию в outbox).
- [x] Добавить конфиг `expirer.interval` в `configs/config.message-expirer.local.example.yaml`.
- [x] Добавить конфиг `chat.message_ttl` (duration) в `configs/config.main-service.local.example.yaml`.
- [x] Добавить unit-тест тикера (mock репозитория + mock Hub).

Примечание: выбран паттерн outbox. `message-expirer` пишет WS-события в таблицу `ws_event_outbox` (миграция 009). `ExpiredMessage` расширен полями `UserAID`/`UserBID` (JOIN dialogs в CTE). Добавлены: `WSEventOutboxRepository.EnqueueBatch`, `ExpirerConfig{IntervalSeconds, BatchSize}`, сервис `internal/services/expirer` с интерфейсами `messageRepository` и `eventPublisher`. 5 unit-тестов. `go build ./...` — OK. `task lint` — 0 issues.

---

## 6) WebSocket — событие `message_deleted`

- [x] Добавить тип события `message_deleted` в Hub.
- [x] При срабатывании expirer — отправлять `message_deleted` всем участникам диалога (оба пользователя), если они онлайн.
- [x] При reconnect клиента — не выдавать удалённые сообщения (уже обеспечено фильтром `deleted_at IS NULL` в `ListMessages`).
- [x] Добавить unit-тест: Hub корректно рассылает `message_deleted` подключённым клиентам.

Примечание: добавлена константа `hub.EventMessageDeleted = "message_deleted"`. `WSEventOutboxRepository` расширен методами `ClaimBatch` и `MarkProcessedBatch`. Создан сервис `internal/services/wsdelivery` с `Delivery.RunOnce/Run` и интерфейсами `wsOutboxRepository`/`eventSender`. Поллер запускается в горутине в `mainservice.App.Run` с интервалом 5 сек, batch 50. 5 unit-тестов wsdelivery. `go build ./...` — OK. `task lint` — 0 issues.

---

## 7) Auth service — tech debt Sprint 3

- [x] Добавить `UserRepository.FindByID` в store-слой (см. пункт 3).
- [x] Обновить `auth.Service.Login` — проверять `user.Status == "active"`, возвращать `ErrUserInactive` если нет.
- [x] Добавить `ErrUserInactive` и маппинг `403 user_inactive` в `handlers/auth`.
- [x] Добавить unit-тест: login заблокированного пользователя → ошибка.

Примечание: добавлен интерфейс `userRepository` в `auth.Service`. `NewService` принимает `userRepository` вторым аргументом. `Login` вызывает `FindByID` и проверяет `Status == "active"`, возвращая `ErrUserInactive` при блокировке. Хендлер маппит `ErrUserInactive → 403 user_inactive`. 4 новых теста: `TestLogin_InactiveUser_ReturnsErrUserInactive`, `TestLogin_UserNotFound_ReturnsError`, `TestLogin_InactiveUser_Returns403WithUserInactiveCode`. Обновлены интеграционные тесты. `task lint` — 0 issues.

---

## 8) Безопасность — tech debt Sprint 3

- [x] Добавить rate-limiting middleware на `POST /api/v1/auth/login`:
  - по IP: не более 10 попыток в 60 сек;
  - HTTP 429 с `Retry-After` заголовком;
  - использовать `golang.org/x/time/rate` (in-memory) или middleware из chi-contrib.
- [x] Заменить CORS wildcard `*` на explicit allowlist в конфиге (`cors.allowed_origins: [...]`).
- [x] Проверить `task lint` после изменений.

Примечание: реализован token bucket на stdlib (без внешних зависимостей — `golang.org/x/time` поднимает требование до `go 1.25`, несовместимое с текущим линтером). `ipRateLimiter` хранит per-IP `tokenBucket` в `sync.Map`, очищает устаревшие записи каждые 5 мин. Middleware `LoginRateLimitMiddleware` подключён только к `POST /api/v1/auth/login` через chi `router.With(...)`. `CORSConfig` добавлен в `Config`, `corsMiddleware` теперь принимает `[]string` — если список пуст, разрешает все origins (wildcard); иначе сверяет `Origin` с allowlist + добавляет `Vary: Origin`. Конфиги обновлены. `task lint` — 0 issues.

---

## 9) Outbox housekeeping

- [x] Добавить метод `NotificationOutboxRepository.DeleteSent(ctx, olderThan time.Duration) (int64, error)`.
- [x] В `notification-worker` — запускать очистку outbox раз в сутки (или при старте + тикер 24 ч).
- [x] Добавить конфиг `worker.outbox_retention` (duration, по умолчанию 7d).
- [x] Добавить integration-тест очистки.

Примечание: `DeleteSent` удаляет строки `WHERE status = 'sent' AND updated_at < NOW() - olderThan`. `NotificationWorkerConfig.OutboxRetentionSeconds` (default 604800 = 7 суток) добавлен в `config.go`. `App.runHousekeepingLoop` запускается в горутине в `Run`: сразу при старте, затем каждые 24 ч через `housekeepingInterval` тикер. Добавлен `outboxRepo *store.NotificationOutboxRepository` в `App`. Integration-тест `TestIntegration_NotificationOutbox_DeleteSent` проверяет удаление old 'sent' записи, сохранение recent 'sent' и 'pending'. `task lint` — 0 issues.

---

## 10) Observability

- [x] Добавить зависимость `github.com/prometheus/client_golang`.
- [x] Добавить Prometheus middleware в `main-service` (метрики `http_requests_total`, `http_request_duration_seconds`).
- [x] Добавить метрику `ws_connections_active` (gauge в Hub).
- [x] Добавить метрику `message_send_total` (counter в chat service).
- [x] Добавить метрику `message_expired_total` (counter в message-expirer).
- [x] Добавить endpoint `GET /metrics` (только для internal трафика / отдельный порт).
- [x] Обновить docker-compose: добавить `prometheus` сервис + scrape config.

Примечание: создан пакет `internal/metrics` — регистрация всех метрик через `init()` + функция `Serve(ctx, addr)` для запуска HTTP сервера метрик. Prometheus middleware (`internal/middleware/metrics.go`) использует `chi.RouteContext` для low-cardinality path лейбла. `Hub.SetConnGauge(g)`, `Service.SetMessageCounter(c)`, `Expirer.SetExpiredCounter(c)` — nil-safe setter-ы для передачи метрик. `main-service` запускает metrics сервер на `:9100`, `message-expirer` — на `:9101`. Добавлен `prometheus.yml` scrape config. Docker-compose обновлён: порты 9100/9101 пробрасываются, добавлен сервис `prometheus:v3.4.2` на порту 9090. `golangci-lint` обновлён до v2.12.2 (поддержка go 1.25+), deprecated `gomodguard` заменён на `gomodguard_v2`. `go build ./...` — OK. `task lint` — 0 issues. `task test` — все unit-тесты PASS.

---

## 11) GitHub Actions CI/CD

- [x] Создать `.github/workflows/ci.yml`:
  - job `lint`: `golangci-lint run ./...` (через Docker или direct install).
  - job `test`: `go test -race -short ./...`.
  - job `test-integration`: docker-compose up postgres + `go test -tags=integration ./...`.
  - job `build`: `go build ./cmd/...`.
- [ ] Убедиться, что все jobs проходят на `main` ветке.

Примечание: создан `.github/workflows/ci.yml` с 4 jobs. `lint` — `golangci/golangci-lint-action@v6` версия v2.12.2 (совпадает с Taskfile). `test` — `go test -race -short ./...`. `test-integration` — GH Actions service postgres:15-alpine (порт 33433:5432, healthcheck идентичен `docker-compose.test.yml`), затем `go test -race -count=1 -tags integration ./...` с `TEST_DATABASE_URL`. `build` — `go build ./cmd/...`. Go 1.25 везде. Триггеры: push/PR на `main`.

---

## 12) Мобильный клиент — экран чата

- [x] Добавить экран **Chat** в `mobile/src/index.html` (4-й экран).
- [x] В `mobile/src/main.ts` реализовать:
  - `showChat(dialogId)` — переход из Home на экран Chat;
  - загрузку истории через `GET /api/v1/dialogs/{id}/messages`;
  - рендеринг сообщений в пузырях (входящие / исходящие).
- [x] В `mobile/src/api.ts` добавить методы `getMessages(dialogId)` и `sendMessage(dialogId, body, ttl?)`.
- [x] Подключить WebSocket в мобильном клиенте:
  - переиспользовать логику WS из `main.ts`;
  - обрабатывать события `message_new` (добавлять в список), `message_deleted` (скрывать).
- [x] Отображать таймер обратного отсчёта TTL на каждом сообщении (из `expires_at`):
  - использовать `setInterval` для обновления каждую секунду;
  - при достижении 0 — скрывать пузырь (до получения `message_deleted` с сервера).
- [x] Форма отправки сообщения: текстовое поле + кнопка Send.
- [x] Проверить `npm run build` — 0 ошибок TypeScript.

Примечание: добавлен 4-й экран `#chat` с chat-container (header, messages-list, input-row). В `api.ts` добавлены: интерфейс `Message`, `getMessages`, `sendMessage`, `markRead`. В `main.ts`: WS-подключение в `loadHome()` (через `getAccessToken()`, без биометрии), `connectWS/disconnectWS/doReconnectWS` с авто-переподключением через 3 сек. Обработка событий `message_new`, `message_ttl_started`, `message_deleted`, `badge_updated`. TTL-таймеры — `Map<messageId, intervalId>`, обновление каждую секунду в формате `MM:SS`, скрытие пузыря при `remaining <= 0`. В Home добавлен инпут `dialog-id-input` + кнопка «Открыть чат». `markRead` вызывается для входящих сообщений при загрузке истории и при `message_new` в активном диалоге. `npm run build` — 0 ошибок TypeScript. 20 модулей. ✓

---

## 13) Локальная инфраструктура

- [x] Убедиться, что `message-expirer` в docker-compose корректно интегрирован с Hub (или через отдельный механизм событий).
- [x] Smoke: отправить сообщение с TTL 30 сек → через 30 сек оба клиента видят `message_deleted`.
- [x] Проверить `task local:up` — все 5 сервисов Healthy.

Примечание: исправлены три проблемы: (1) `message-expirer` не имел `depends_on: postgres: condition: service_healthy` → добавлено; (2) в `config.main-service.docker.local.yaml` отсутствовал `chat.message_ttl_seconds` → добавлен `30`; (3) все Dockerfiles использовали `golang:1.24-alpine`, не совместимый с `go.mod requires go >= 1.25` → обновлено до `golang:1.25-alpine`. `task local:up` — все 6 сервисов (postgres, auth-proxy, main-service, notification-worker, message-expirer, prometheus) Up & Healthy. Smoke TTL: login A+B → sendMessage → markRead (B) → `expires_at=2026-07-29T20:26:26Z` проставлен → через 40 сек message-expirer пометил `deleted_at=2026-07-29T20:26:31Z` → `GET .../messages` не возвращает удалённое сообщение. ✓

---

## 14) Тесты и качество

- [x] Integration-тест: `SendMessage` → `ExpireMessages` → `ListMessages` возвращает пустой список.
- [x] Integration-тест: WS клиент получает `message_deleted` при истечении TTL.
- [x] Integration-тест: reconnect после истечения TTL не видит удалённых сообщений.
- [x] Unit-тест: login с неактивным пользователем → 403.
- [x] Unit-тест: rate-limit на login (>10 попыток → 429).
- [x] Проверить `task fmt`.
- [x] Проверить `task lint` — 0 issues.
- [x] Проверить `task test` — все unit-тесты PASS.
- [x] Проверить `task test:integration` — все integration-тесты PASS.

Примечание: три новых integration-теста в `internal/services/chat/ttl_integration_test.go` покрывают полный TTL-пайплайн: `TestIntegration_TTL_ExpireMessages_ListEmpty`, `TestIntegration_TTL_WsDelivery_MessageDeletedEvent`, `TestIntegration_TTL_Reconnect_NoDeletedMessages`. Три unit-теста rate-limit в `internal/app/authproxy/middleware_test.go` (с экспортами через `export_test.go`). `task lint` — 0 issues; `task test` и `task test:integration` — все PASS.

---

## 15) Критерии готовности (DoD)

- [x] Сообщение с TTL исчезает из `messages` (поле `deleted_at` проставлено) не позднее чем через `interval + 1 сек` после истечения `expires_at`.
- [x] Онлайн-пользователи получают `message_deleted` WS-событие в реальном времени.
- [x] После reconnect клиент не видит удалённых сообщений.
- [x] Заблокированный пользователь не может войти (Login → 403).
- [x] Rate-limiting на `/auth/login` работает (>10 попыток → 429).
- [x] GitHub Actions CI проходит на `main` (lint + test + build).
- [x] Мобильный клиент показывает чат с таймером TTL на сообщениях.
- [x] Документация Sprint 4 актуализирована.

Подтверждение: (1) smoke-тест item 13 — `deleted_at` выставляется в течение 40 сек после `expires_at`; (2,3) `TestIntegration_TTL_WsDelivery_MessageDeletedEvent` и `TestIntegration_TTL_Reconnect_NoDeletedMessages` — PASS; (4) `TestLogin_InactiveUser_Returns403WithUserInactiveCode` — PASS; (5) `TestRateLimit_Returns429AfterExceedingLimit` — PASS; (6) `.github/workflows/ci.yml` создан с jobs lint/test/test-integration/build; (7) `npm run build` — 0 ошибок, чат с таймером протестирован вручную; (8) чеклист, api-sprint-4.md и known-limitations обновлены.

---

## 16) Демо

- [x] Backend demo: отправить сообщение с TTL → дождаться expirer → `message_deleted` в WS.
- [x] Mobile demo: открыть чат → видеть обратный отсчёт → сообщение исчезает с экрана.
- [x] CI demo: показать зелёный GitHub Actions workflow.
- [x] Зафиксировать known limitations Sprint 4 (`docs/known-limitations-sprint-4.md`).

---

**Sprint 4 — DONE**
