# Sprint 4 Checklist

Источник: `docs/chat-architecture-plan.md` §13 (Sprint 4) + tech debt из `docs/known-limitations-sprint-3.md`.

**Цель спринта:** сообщения исчезают по таймеру у обоих пользователей и в БД; мобильный клиент показывает полноценный чат с обратным отсчётом TTL.

---

## 1) Подготовка и контракты

- [ ] Утвердить модель TTL: глобальный конфиг vs политика диалога (на Sprint 4 — глобальный конфиг).
- [ ] Утвердить поля `expires_at TIMESTAMPTZ NULL` и `deleted_at TIMESTAMPTZ NULL` в `messages`.
- [ ] Утвердить формат события `message_deleted` (payload: `{type, message_id, dialog_id}`).
- [ ] Утвердить формат TTL в `SendMessage` API: включать `expires_at` в ответ и в `message_new` WS-событие.
- [ ] Утвердить интервал тикера `message-expirer` (по умолчанию: 10 сек).
- [ ] Подготовить `docs/api-sprint-4.md` с обновлёнными контрактами.

---

## 2) База данных и миграции

- [ ] Создать миграцию `internal/store/migrations/008_message_ttl.sql`:
  - добавить `expires_at TIMESTAMPTZ NULL` в `messages`;
  - добавить `deleted_at TIMESTAMPTZ NULL` в `messages`;
  - добавить индекс `idx_messages_expires_at` (`expires_at`) WHERE `deleted_at IS NULL`.
- [ ] Обновить модель `Message` в `internal/store/models.go` (поля `ExpiresAt`, `DeletedAt`).
- [ ] Проверить миграции на чистой БД и при повторном запуске (идемпотентность).

---

## 3) Store-слой (`internal/store/`)

- [ ] Обновить `MessageRepository.Create` — принимать и сохранять `expires_at`.
- [ ] Обновить `MessageRepository.List` — фильтровать `WHERE deleted_at IS NULL`.
- [ ] Добавить `MessageRepository.ExpireMessages(ctx, now time.Time) ([]ExpiredMessage, error)` — batch UPDATE:
  ```sql
  UPDATE messages
  SET deleted_at = $1
  WHERE expires_at <= $1 AND deleted_at IS NULL
  RETURNING id, dialog_id, sender_id
  ```
- [ ] Добавить `UserRepository.FindByID(ctx, userID) (*User, error)` — для проверки статуса при login (tech debt).
- [ ] Добавить integration-тесты для новых методов репозитория.

---

## 4) Chat service (`internal/services/chat/`)

- [ ] Обновить `SendMessage` — вычислять `expires_at = now() + ttl` из конфига, передавать в `MessageRepository.Create`.
- [ ] Включать `expires_at` в `message_new` WS-событие (Hub broadcast).
- [ ] Включать `expires_at` в ответ REST `POST /api/v1/dialogs/{dialog_id}/messages`.
- [ ] Обновить `ListMessages` — поле `expires_at` в ответе.
- [ ] Добавить unit-тесты обновлённого `SendMessage`.

---

## 5) Message Expirer (`cmd/message-expirer/`)

- [ ] Реализовать реальный тикер в `internal/app/messageexpirer/app.go`:
  - каждые N секунд (конфиг `expirer.interval`, по умолчанию 10 сек);
  - вызов `MessageRepository.ExpireMessages(ctx, time.Now())`;
  - broadcast `message_deleted` событий через Hub для онлайн-пользователей;
  - structured log `message_expired` с `count`, `duration_ms`.
- [ ] Передать Hub в App message-expirer (или использовать общий Event Bus / публикацию в outbox).
- [ ] Добавить конфиг `expirer.interval` в `configs/config.message-expirer.local.example.yaml`.
- [ ] Добавить конфиг `chat.message_ttl` (duration) в `configs/config.main-service.local.example.yaml`.
- [ ] Добавить unit-тест тикера (mock репозитория + mock Hub).

---

## 6) WebSocket — событие `message_deleted`

- [ ] Добавить тип события `message_deleted` в Hub.
- [ ] При срабатывании expirer — отправлять `message_deleted` всем участникам диалога (оба пользователя), если они онлайн.
- [ ] При reconnect клиента — не выдавать удалённые сообщения (уже обеспечено фильтром `deleted_at IS NULL` в `ListMessages`).
- [ ] Добавить unit-тест: Hub корректно рассылает `message_deleted` подключённым клиентам.

---

## 7) Auth service — tech debt Sprint 3

- [ ] Добавить `UserRepository.FindByID` в store-слой (см. пункт 3).
- [ ] Обновить `auth.Service.Login` — проверять `user.Status == "active"`, возвращать `ErrUserInactive` если нет.
- [ ] Добавить `ErrUserInactive` и маппинг `403 user_inactive` в `handlers/auth`.
- [ ] Добавить unit-тест: login заблокированного пользователя → ошибка.

---

## 8) Безопасность — tech debt Sprint 3

- [ ] Добавить rate-limiting middleware на `POST /api/v1/auth/login`:
  - по IP: не более 10 попыток в 60 сек;
  - HTTP 429 с `Retry-After` заголовком;
  - использовать `golang.org/x/time/rate` (in-memory) или middleware из chi-contrib.
- [ ] Заменить CORS wildcard `*` на explicit allowlist в конфиге (`cors.allowed_origins: [...]`).
- [ ] Проверить `task lint` после изменений.

---

## 9) Outbox housekeeping

- [ ] Добавить метод `NotificationOutboxRepository.DeleteSent(ctx, olderThan time.Duration) (int64, error)`.
- [ ] В `notification-worker` — запускать очистку outbox раз в сутки (или при старте + тикер 24 ч).
- [ ] Добавить конфиг `worker.outbox_retention` (duration, по умолчанию 7d).
- [ ] Добавить integration-тест очистки.

---

## 10) Observability

- [ ] Добавить зависимость `github.com/prometheus/client_golang`.
- [ ] Добавить Prometheus middleware в `main-service` (метрики `http_requests_total`, `http_request_duration_seconds`).
- [ ] Добавить метрику `ws_connections_active` (gauge в Hub).
- [ ] Добавить метрику `message_send_total` (counter в chat service).
- [ ] Добавить метрику `message_expired_total` (counter в message-expirer).
- [ ] Добавить endpoint `GET /metrics` (только для internal трафика / отдельный порт).
- [ ] Обновить docker-compose: добавить `prometheus` сервис + scrape config.

---

## 11) GitHub Actions CI/CD

- [ ] Создать `.github/workflows/ci.yml`:
  - job `lint`: `golangci-lint run ./...` (через Docker или direct install).
  - job `test`: `go test -race -short ./...`.
  - job `test-integration`: docker-compose up postgres + `go test -tags=integration ./...`.
  - job `build`: `go build ./cmd/...`.
- [ ] Убедиться, что все jobs проходят на `main` ветке.

---

## 12) Мобильный клиент — экран чата

- [ ] Добавить экран **Chat** в `mobile/src/index.html` (4-й экран).
- [ ] В `mobile/src/main.ts` реализовать:
  - `showChat(dialogId)` — переход из Home на экран Chat;
  - загрузку истории через `GET /api/v1/dialogs/{id}/messages`;
  - рендеринг сообщений в пузырях (входящие / исходящие).
- [ ] В `mobile/src/api.ts` добавить методы `getMessages(dialogId)` и `sendMessage(dialogId, body, ttl?)`.
- [ ] Подключить WebSocket в мобильном клиенте:
  - переиспользовать логику WS из `main.ts`;
  - обрабатывать события `message_new` (добавлять в список), `message_deleted` (скрывать).
- [ ] Отображать таймер обратного отсчёта TTL на каждом сообщении (из `expires_at`):
  - использовать `setInterval` для обновления каждую секунду;
  - при достижении 0 — скрывать пузырь (до получения `message_deleted` с сервера).
- [ ] Форма отправки сообщения: текстовое поле + кнопка Send.
- [ ] Проверить `npm run build` — 0 ошибок TypeScript.

---

## 13) Локальная инфраструктура

- [ ] Убедиться, что `message-expirer` в docker-compose корректно интегрирован с Hub (или через отдельный механизм событий).
- [ ] Smoke: отправить сообщение с TTL 30 сек → через 30 сек оба клиента видят `message_deleted`.
- [ ] Проверить `task local:up` — все 5 сервисов Healthy.

---

## 14) Тесты и качество

- [ ] Integration-тест: `SendMessage` → `ExpireMessages` → `ListMessages` возвращает пустой список.
- [ ] Integration-тест: WS клиент получает `message_deleted` при истечении TTL.
- [ ] Integration-тест: reconnect после истечения TTL не видит удалённых сообщений.
- [ ] Unit-тест: login с неактивным пользователем → 403.
- [ ] Unit-тест: rate-limit на login (>10 попыток → 429).
- [ ] Проверить `task fmt`.
- [ ] Проверить `task lint` — 0 issues.
- [ ] Проверить `task test` — все unit-тесты PASS.
- [ ] Проверить `task test:integration` — все integration-тесты PASS.

---

## 15) Критерии готовности (DoD)

- [ ] Сообщение с TTL исчезает из `messages` (поле `deleted_at` проставлено) не позднее чем через `interval + 1 сек` после истечения `expires_at`.
- [ ] Онлайн-пользователи получают `message_deleted` WS-событие в реальном времени.
- [ ] После reconnect клиент не видит удалённых сообщений.
- [ ] Заблокированный пользователь не может войти (Login → 403).
- [ ] Rate-limiting на `/auth/login` работает (>10 попыток → 429).
- [ ] GitHub Actions CI проходит на `main` (lint + test + build).
- [ ] Мобильный клиент показывает чат с таймером TTL на сообщениях.
- [ ] Документация Sprint 4 актуализирована.

---

## 16) Демо

- [ ] Backend demo: отправить сообщение с TTL → дождаться expirer → `message_deleted` в WS.
- [ ] Mobile demo: открыть чат → видеть обратный отсчёт → сообщение исчезает с экрана.
- [ ] CI demo: показать зелёный GitHub Actions workflow.
- [ ] Зафиксировать known limitations Sprint 4 (`docs/known-limitations-sprint-4.md`).

---

**Sprint 4 — IN PROGRESS**
