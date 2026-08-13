# Детальный план реализации `my-chat` (архитектура в стиле OtusMS)

Документ — living architecture: цели, принятые решения и фактическое устройство системы.  
Детальные задачи и DoD — в `docs/sprint-N-plan.md` / `docs/sprint-N-checklist.md`. Контракты API — в `docs/api-sprint-N.md`.

**Статус (2026-08):** спринты **1–8 DONE**. Далее: **9** (at-rest encryption сообщений). Prod: **https://beepru.ru**.

---

## 1) Цели и ограничения

Проект: чат для двоих пользователей с мобильным/PWA-клиентом, где обязательны:
- push-уведомления о новых сообщениях;
- бейдж непрочитанных сообщений на иконке приложения;
- вход с биометрией (Face ID / Touch ID) как защита локального refresh;
- автоудаление сообщений по таймеру, включая исчезновение на экране.

Ограничения и вводные:
- backend на Go;
- клиент гибридный / PWA (не полностью нативный App Store);
- архитектура в стиле OtusMS: сервисы, слои `handlers → services → store`, конфиги, middleware, observability;
- **без** Kubernetes, групповых чатов, E2EE, media/attachments (вне текущего scope).

---

## 2) Целевая архитектура

### 2.1 Сервисы (`cmd/*`)

1. `cmd/main-service/`
   - HTTP API + WebSocket;
   - регистрация пользователей, чат, devices, unread;
   - `/health`, `/debug` (dev);
   - владелец миграций БД.

2. `cmd/auth-proxy/`
   - `login` / `refresh` / `logout`;
   - JWT access + server-side refresh sessions (`auth_sessions`);
   - rate limit на login;
   - биометрия — только на клиенте (gate к refresh), не отдельный серверный endpoint.

3. `cmd/notification-worker/`
   - poll `notification_outbox`;
   - провайдеры push: `noop` | `dev-log` | **`webpush`** (Sprint 6);
   - badge в payload; retry/backoff; деактивация мёртвых подписок (410/404).

4. `cmd/message-expirer/`
   - soft-delete по `expires_at`;
   - события `message_deleted` через `ws_event_outbox` → доставка онлайн-клиентам.

### 2.2 Логическая схема взаимодействия

1. Пользователь A регистрируется (`POST /api/v1/users/register`) и логинится (`username`/`password` → JWT + refresh).
2. A отправляет сообщение в `main-service`; статус `sent`, receipts создаются.
3. Если B онлайн — событие по WebSocket (`message_new` и др.).
4. Если B офлайн — задача в `notification_outbox` → worker шлёт push (целевой путь: Web Push / VAPID).
5. При `mark_read` — пересчёт unread, `badge_updated` по WS; TTL стартует от первого чтения (`expires_at = read_at + ttl`).
6. `message-expirer` удаляет просроченные сообщения и шлёт `message_deleted`.

### 2.3 Принятые инфраструктурные решения

| Тема | Решение |
|------|---------|
| Брокер событий | **Нет.** PostgreSQL outbox: `notification_outbox`, `ws_event_outbox` |
| Redis | **Нет** |
| Push | Целевой: **Web Push (VAPID)** для PWA (iOS 16.4+ / браузеры). APNs/FCM и App Store — out of scope |
| Prod edge | Nginx :80/:443, path-based routing на одном домене |
| Домен | `beepru.ru` |
| Образы | Сборка на VPS (`docker compose build`), без внешнего registry |
| CD | GitHub Actions → SSH → `git pull` + compose up + `nginx -s reload` |
| Секреты | `/opt/my-chat/.env` на VPS; GitHub Secrets для SSH; в git только `.env.example` |

---

## 3) Структура репозитория (фактическая)

```text
my-chat/
  cmd/
    main-service/
    auth-proxy/
    notification-worker/
    message-expirer/
  internal/
    app/                 # wiring: mainservice, authproxy, notificationworker, messageexpirer
    handlers/            # auth, chat, device, user, ws, health, debug
    services/            # auth, chat, device, user, notification, expirer, wsdelivery
    store/               # репозитории + migrations/ (001–011) + Migrate()
    clients/push/        # Provider: noop, dev-log [, webpush]
    hub/                 # WebSocket hub
    jwt/
    middleware/
    config/
    metrics/
    logger/
  mobile/                # Vite + Capacitor 8 (+ целевой PWA в Sprint 6)
  deploy/
    local/               # compose, prometheus, seed-users.sql
    prod/                # compose, nginx, init-ssl.sh
    test/                # postgres для integration tests (:33433)
  configs/               # per-service: *.local.example, *.docker.local, *.prod
  docs/
    chat-architecture-plan.md
    sprint-N-plan.md / sprint-N-checklist.md / api-sprint-N.md
    known-limitations-sprint-N.md
  Taskfile.yml
  .github/workflows/     # ci.yml, cd.yml
```

Слои: `handlers → services → store`. Пустые заготовки пакетов и `proto/` не используются.

---

## 4) Технологические решения

### 4.1 Backend
- Go **1.25**;
- HTTP: `chi/v5`;
- WebSocket: `coder/websocket`;
- DB: PostgreSQL + `pgx/v5`;
- миграции: свой runner (`internal/store/migrate.go`, `//go:embed`, advisory lock) — **не goose**;
- пароли: `golang.org/x/crypto/bcrypt` (cost 12);
- Web Push (Sprint 6): `SherClockHolmes/webpush-go`;
- без Redis / Kafka / NATS / gRPC.

### 4.2 Клиент
- **Сейчас:** Vanilla TS + Vite + Capacitor 8; `@aparajita/capacitor-biometric-auth`; secure storage для refresh.
- **Цель Sprint 6:** PWA (`manifest.json`, Service Worker, Web Push, `navigator.setAppBadge`); установка с Safari «На экран Домой».
- Нативный App Store / Android store — не входят в текущий scope.

### 4.3 Локальная разработка
- Управление только через Taskfile: `task local:*`, `task lint`, `task test`, `task test:integration`.
- Compose: `deploy/local/docker-compose.local.yml`.
- Seed: `deploy/local/seed-users.sql` (`alice`/`bob`, пароль `password123`) — вручную после `local:up`.

---

## 5) Модель данных (PostgreSQL)

Ключевые таблицы (миграции `001`–`011`):

1. `users` — `id`, `status`, `username` UNIQUE, `password_hash`, `created_at`
2. `dialogs` — пара `user_a_id` / `user_b_id`
3. `messages` — `body`, `status`, `expires_at`, `deleted_at`, …
4. `message_receipts` — `delivered_at`, `read_at`
5. `devices` — `platform` (`ios`|`android`|`web`), nullable `push_token`, `push_subscription` JSONB
6. `notification_outbox` — push-задачи (dedup, pending/sent/failed, retry)
7. `auth_sessions` — refresh family, `token_hash`, `device_id`, revoke/reuse detection
8. `ws_event_outbox` — offline/async доставка WS-событий (в т.ч. `message_deleted`)

Unread badge считается запросами по receipts/messages — отдельного `badge_count` в `devices` нет.

---

## 6) API-контракты

Актуальные детали Sprint 6: `docs/api-sprint-6.md`. Prod base: `https://beepru.ru`.

### 6.1 HTTP (сводка)

**auth-proxy** (prod: `/auth/...` → strip prefix):
- `POST /api/v1/auth/login` — `{username, password}`; опционально `X-Device-ID` (Sprint 6)
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`

**main-service**:
- `POST /api/v1/users/register`
- `POST /api/v1/devices/register` | `unregister` — для `web`: `push_subscription` (Sprint 6 §8)
- `GET /api/v1/push/vapid-public-key` — публичный (Sprint 6 §8)
- `GET|POST` dialogs/messages, mark read, `GET /api/v1/me/unread-count`
- `GET /health`, `GET /debug` (local/dev)

### 6.2 WebSocket
- `GET /ws/connect?token=...`
- события: `message_new`, `message_delivered`, `message_read`, `message_ttl_started`, `message_deleted`, `badge_updated`
- heartbeat ping/pong — Sprint 6 §11 (ещё не в коде)

### 6.3 Debug web client
- `GET /debug` в `main-service` — встроенная HTML-страница для ручной отладки.
- Только local/dev; в prod отключать флагом.
- **Важно:** после перехода login на username/password debug UI / mobile могут отставать — для smoke auth использовать curl, пока клиенты не обновлены (Sprint 6 §13–14).

### 6.4 Prod routing (nginx)

| Path | Upstream |
|------|----------|
| `/api/` | main-service:8080 |
| `/ws/` | main-service:8080 (upgrade) |
| `/auth/` | auth-proxy:33081 |
| `/health` | main-service:8080 |
| `/` | пока 404 (раздача PWA — часть Sprint 6) |

---

## 7) Авторизация и биометрия

1. Регистрация: `username` (3–50, `[a-zA-Z0-9_]`) + `password` (≥8) → bcrypt.
2. Login: проверка credentials → access JWT (короткий TTL) + refresh; сессия в `auth_sessions`.
3. Refresh с ротацией; reuse detection → revoke family.
4. Клиент хранит refresh в Preferences / secure storage; native — локальная биометрия (Capacitor) перед чтением refresh; PWA — локальный PIN (Sprint 8), без серверной проверки PIN.
5. Device binding (`X-Device-ID` на login/refresh → `403 device_mismatch`) — Sprint 6 §12.
6. Rate limit login: **10 req / 60s / IP** → `429`.

Внешний IdP не используется. Face ID / PIN не заменяют серверную аутентификацию (password + JWT).

---

## 8) Уведомления и бейдж

1. Сервер — источник истины по unread count.
2. Офлайн-получатель: запись в `notification_outbox` → worker → push с актуальным badge (пересчёт перед send — Sprint 6 §9).
3. `mark_read` → `badge_updated` по WS; silent `badge_sync` на другие устройства — Sprint 6 §10.
4. Провайдеры: local `dev-log`, prod до Sprint 6 — `noop`; цель — `webpush` + VAPID в `.env`.
5. Идемпотентность через dedup key в outbox; retry с backoff.

---

## 9) Автоудаление сообщений (TTL)

1. TTL задаётся глобально: `chat.message_ttl_seconds` (**0 = выкл**). Prod: **60s**; local example: **300s**.
2. Семантика: таймер стартует **после первого `mark_read`**, не при создании (`expires_at = read_at + ttl`); событие `message_ttl_started`.
3. `message-expirer` (тикер ~10s, batch 100): soft-delete + `message_deleted` через `ws_event_outbox`.
4. Клиенты удаляют сообщение с экрана; reconnect не возвращает удалённые из истории.

---

## 10) Безопасность

Реализовано / принято:
- JWT + refresh rotation + session revoke / reuse detection;
- TLS на prod (Let's Encrypt);
- rate limit на login;
- валидация входа; bcrypt для паролей;
- секреты вне git; CORS allowlist (prod: `https://beepru.ru`).

Отложено / не в scope сейчас:
- полноценный RBAC (`user`/`admin`/`service-account`);
- `request_id` middleware;
- rate limit на send-message;
- E2EE.

---

## 11) Наблюдаемость и эксплуатация

- structured logging (`slog`);
- Prometheus (сервисы отдают метрики; scrape в **local** compose):
  - `http_requests_total`, `http_request_duration_seconds`
  - `ws_connections_active`
  - `message_send_total`
  - `message_expired_total`
- main-service metrics `:9100`, message-expirer `:9101`;
- `/health`; в prod Prometheus в compose не обязателен;
- `/debug` — только для разработки.

Планировались, но не заведены: `message_delivery_latency_seconds`, `push_send_total`.

---

## 12) Docker и CI/CD

### Local
`deploy/local/docker-compose.local.yml`: postgres (`33432`), auth-proxy (`33081`), main-service (`8080`), notification-worker, message-expirer, prometheus (`9090`).  
Управление: `task local:up|down|ps|logs|…`.

### Prod
`deploy/prod/docker-compose.prod.yml`: postgres, сервисы приложения, nginx, certbot, nginx-reloader.  
Публичны только 80/443. Первичный TLS: `deploy/prod/init-ssl.sh`.

### CI/CD
- CI: lint + tests + build (`.github/workflows/ci.yml`);
- CD: push в `main` → SSH на VPS → pull/build/up → nginx reload (`.github/workflows/cd.yml`).

Известные нюансы: `docs/known-limitations-sprint-5.md` (порядок `set`/`rewrite` в nginx, reload после compose, нет `www`, seed в prod вручную).

---

## 13) План работ (по спринтам)

### Sprint 1 — DONE (MVP backend)
Каркас, main-service + `/debug`, миграции, JWT login/refresh/logout (тогда ещё `user_id`), HTTP chat, WS `message_new`, local Docker.

### Sprint 2 — DONE (уведомления + badge)
Devices API, `notification_outbox`, worker (`dev-log`/`noop`), серверный unread, `badge_updated`. Реальный APNs/FCM не внедрялся — позже заменён стратегией Web Push.

### Sprint 3 — DONE (Face ID + session hardening)
`auth_sessions`, ротация refresh, revoke/reuse, Capacitor-клиент, secure storage + биометрия.

### Sprint 4 — DONE (TTL + чат UI)
TTL после read, `message-expirer`, `message_deleted`, rate-limit login, CORS, Prometheus, экран Chat, CI.

### Sprint 5 — DONE (prod deploy)
Домен `beepru.ru`, nginx+TLS, `deploy/prod/`, CD на VPS, prod-конфиги, Page Visibility для mark-read.

### Sprint 6 — DONE / closing (production polish / PWA)
PWA на Home Screen, Web Push (VAPID), badge + `badge_sync`, device binding, login username/password, раздача клиента с `/`. Опциональные хвосты (heartbeat demo, known-limitations) — см. чеклист.

Детали: `docs/sprint-6-plan.md`, `docs/sprint-6-checklist.md`.

### Sprint 7 — DONE (список чатов / username)
`GET/POST /api/v1/dialogs`, `GET /users/search` (Should); PWA Home = список + «Новый чат» по username; без ручного UUID. WS `message_new` вне открытого чата → refresh списка. Known limitations: `docs/known-limitations-sprint-7.md`.

Детали: `docs/sprint-7-plan.md`, `docs/sprint-7-checklist.md`, `docs/api-sprint-7.md`.

### Sprint 8 — DONE (PWA unlock / PIN + UX)
Локальный PIN (4 цифры) после register/login; cold start + resume (grace 60s); unlock в PWA по вводу 4-й цифры (без кнопки «Разблокировать»); refresh encrypted (`enc:v1:` PBKDF2+AES-GCM); push title = username отправителя; фон чата бирюза + watermark. Без Redis/PIN API. WebAuthn — later (8.1+). Capacitor Face ID сохранён. Known limitations: `docs/known-limitations-sprint-8.md`.

Детали: `docs/sprint-8-plan.md`, `docs/sprint-8-checklist.md`, `docs/api-sprint-8.md`.

### Sprint 9 — PLANNED (message encryption at-rest)
AES-256-GCM envelope (Вариант A): ciphertext в БД, plaintext только в памяти сервиса и у клиента по TLS. Не E2EE. Hard-delete/purge — вне scope.

Детали: `docs/sprint-9-plan.md`, `docs/sprint-9-checklist.md`, `docs/api-sprint-9.md`.

---

## 14) Риски и решения

1. **Web Push только после «На экран Домой» (Safari)**  
   → баннер установки PWA; проверка `'PushManager' in window`.

2. **Рассинхрон бейджа**  
   → сервер — источник истины; пересчёт перед send; silent `badge_sync` на другие устройства; клиентский `setAppBadge` на WS.

3. **Потеря WS-событий / мёртвые соединения**  
   → heartbeat (Sprint 6); backlog через REST; полноценный event-cursor/replay — known limitation (отложено).

4. **TTL расходится между устройствами**  
   → authoritative delete на сервере + `message_deleted`.

5. **Ручной dialog UUID в UI**  
   → закрыто в Sprint 7 (список + create by username).

6. **Face ID недоступен в Safari/PWA через Capacitor**  
   → закрыто в Sprint 8 (локальный PIN + encrypt refresh); WebAuthn platform passkey — follow-up 8.1+.

7. **Устаревший push endpoint**  
   → HTTP 404/410 → деактивировать подписку в БД.

8. **Plaintext body в дампах БД**  
   → Sprint 9: at-rest AES-GCM; ключ отдельно от БД; не путать с E2EE.

---

## 15) Что делать сейчас (фокус)

1. **Sprint 9** — at-rest encryption сообщений (после 7/8; не смешивать DoD).
2. Хвосты Sprint 6/7/8 known-limitations — по приоритету (unregister devices, CountUnread vs deleted_at, WebAuthn 8.1).
3. Не переоткрывать Sprint 1–5; infra — поверх `deploy/prod/`.

Операционные скиллы: `.cursor/skills/local-dev`, `prod-deploy`, `sprint-work`.
