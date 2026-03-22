# Детальный план реализации `my-chat` (архитектура в стиле OtusMS)

## 1) Цели и ограничения

Проект: чат для двоих пользователей с мобильным клиентом, где обязательны:
- push-уведомления о новых сообщениях;
- бейдж непрочитанных сообщений на иконке приложения;
- вход с биометрией Face ID / Touch ID;
- автоудаление сообщений по таймеру, включая исчезновение на экране.

Ограничения и вводные:
- backend пишется на Go;
- мобильный клиент не полностью нативный (iOS/Android), предпочтительно гибридный;
- архитектура должна быть похожа на OtusMS: сервисный подход, слои `handlers -> services -> store`, конфиги, middleware, observability.

---

## 2) Целевая архитектура

### 2.1 Сервисы (по аналогии с OtusMS `cmd/*`)

1. `cmd/main-service/`
   - основной API для клиента (HTTP + WebSocket);
   - чтение/отправка сообщений;
   - учет статусов (`sent`, `delivered`, `read`);
   - расчет и выдача `unread_count`;
   - публикация событий в брокер (опционально) для надежной доставки уведомлений.

2. `cmd/auth-proxy/`
   - вход/обновление токена/выход;
   - интеграция с внешним провайдером auth (на MVP можно локальный JWT + refresh);
   - endpoint для биометрического re-auth (клиент подтверждает локальной Face ID, сервис валидирует refresh flow).

3. `cmd/notification-worker/`
   - обработка событий новых сообщений;
   - отправка push (APNs для iOS, FCM для Android);
   - установка badge count в payload;
   - ретраи и DLQ (dead-letter queue) при ошибках доставки.

4. `cmd/message-expirer/`
   - периодическая задача удаления просроченных сообщений (`expires_at <= now`);
   - генерация событий `message_deleted` для синхронного удаления с экранов клиентов.

### 2.2 Логическая схема взаимодействия

1. Пользователь A отправляет сообщение в `main-service`.
2. `main-service` сохраняет сообщение в PostgreSQL, выставляет `expires_at`, status=`sent`.
3. Если пользователь B онлайн, сообщение уходит по WebSocket сразу.
4. Если B офлайн/в фоне, `notification-worker` отправляет push с badge.
5. При открытии чата клиент читает backlog и подтверждает `read`.
6. `message-expirer` удаляет просроченные сообщения и инициирует удаление на экранах обоих клиентов.

---

## 3) Структура репозитория (рекомендуемая)

```text
my-chat/
  cmd/
    main-service/
    auth-proxy/
    notification-worker/
    message-expirer/
  internal/
    handlers/
      auth/
      chat/
      ws/
      health/
    services/
      auth/
      chat/
      notifications/
      expiry/
    store/
      user/
      chat/
      device/
      migrations/
    middleware/
      jwt.go
      rbac.go
      request_id.go
      logging.go
      ratelimit.go
    config/
      config.go
      parse.go
    clients/
      push/
      mainservice/
    metrics/
    logger/
    models/
  proto/
    chat/v1/
  deploy/
    local/
      docker-compose.local.yml
    prod/
      docker-compose.prod.yml
  configs/
    config.local.example.yaml
  docs/
    chat-architecture-plan.md
```

---

## 4) Технологические решения

### 4.1 Backend
- Go 1.23+;
- HTTP: `chi`;
- Realtime: WebSocket (`gorilla/websocket` или `nhooyr.io/websocket`);
- DB: PostgreSQL + `pgx/v5`;
- миграции: `goose` или свой runner;
- кэш/сессии (опционально): Redis;
- брокер событий (опционально, но желательно): Kafka/NATS/RabbitMQ.

### 4.2 Мобильный клиент
- гибридный клиент: Ionic + Capacitor (или React + Capacitor);
- biometry plugin (Face ID / Touch ID / Android biometrics);
- push plugin для APNs/FCM;
- хранение токенов в secure storage (Keychain/Keystore).

---

## 5) Модель данных (PostgreSQL)

Минимальные таблицы:

1. `users`
   - `id uuid pk`
   - `created_at timestamptz`
   - `status text` (active/blocked)

2. `dialogs`
   - `id uuid pk`
   - `user_a_id uuid`
   - `user_b_id uuid`
   - `created_at timestamptz`
   - уникальность пары пользователей.

3. `messages`
   - `id uuid pk`
   - `dialog_id uuid fk`
   - `sender_id uuid fk`
   - `body text`
   - `created_at timestamptz`
   - `expires_at timestamptz`
   - `deleted_at timestamptz null`
   - `status text` (sent/delivered/read/deleted)

4. `message_receipts`
   - `message_id uuid fk`
   - `user_id uuid fk`
   - `delivered_at timestamptz null`
   - `read_at timestamptz null`

5. `devices`
   - `id uuid pk`
   - `user_id uuid fk`
   - `platform text` (ios/android)
   - `push_token text`
   - `badge_count int`
   - `last_seen_at timestamptz`

---

## 6) API-контракты (MVP)

### 6.1 HTTP
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `POST /api/v1/devices/register`
- `GET /api/v1/dialogs/{id}/messages?limit=&before=`
- `POST /api/v1/dialogs/{id}/messages`
- `POST /api/v1/messages/{id}/read`
- `GET /api/v1/me/unread-count`
- `GET /health`

### 6.2 WebSocket
- `GET /ws/connect` (JWT в header/cookie)
- события:
  - `message_new`
  - `message_delivered`
  - `message_read`
  - `message_deleted`
  - `badge_updated`

---

## 7) Авторизация и Face ID

1. Первый вход: логин/код/пароль -> выдача `access` + `refresh`.
2. `refresh` хранится только в secure storage клиента.
3. Перед использованием refresh клиент запрашивает локальную биометрию.
4. При успешной биометрии выполняется refresh flow, сервер выдает новый access.

Важно:
- Face ID не заменяет серверную аутентификацию, а защищает доступ к локальному refresh-токену;
- при смене биометрии на устройстве делать revoke текущей сессии и повторный full login.

---

## 8) Уведомления и бейдж

1. Сервер ведет источник истины по unread count.
2. При новом сообщении офлайн-пользователю:
   - увеличить unread count;
   - отправить push с `badge=<актуальное значение>`.
3. При открытии чата и `read`:
   - пересчитать unread;
   - отправить клиенту `badge_updated` и обнулить локальный badge.

Рекомендуется:
- идемпотентные push-задачи (dedup key);
- retry с backoff;
- аудит-лог `push_attempts`.

---

## 9) Автоудаление сообщений (TTL)

1. В момент создания сообщения назначать `expires_at` (например, `now + 24h`, задается политикой диалога).
2. Клиент отображает таймер до удаления.
3. `message-expirer` раз в N секунд:
   - помечает истекшие сообщения `deleted_at=now, status=deleted`;
   - публикует событие `message_deleted`.
4. Клиенты получают событие и удаляют сообщение с экрана без ручного refresh.
5. При reconnect клиент не получает удаленные сообщения в истории.

---

## 10) Безопасность

- JWT с коротким TTL (`5-15` минут), refresh с ротацией;
- все endpoint'ы под TLS;
- RBAC: `user`, `admin`, `service-account`;
- rate limit на auth и send-message;
- серверная валидация всех входных данных;
- журналирование критичных действий (auth, token refresh, delete event).

---

## 11) Наблюдаемость и эксплуатация

Как в OtusMS:
- structured logging (`slog`) + `request_id`;
- Prometheus метрики:
  - `http_requests_total`
  - `ws_connections_active`
  - `message_send_total`
  - `message_delivery_latency_seconds`
  - `push_send_total`
  - `message_expired_total`
- `/health` + readiness/liveness;
- pprof/debug endpoint отдельно от клиентского API.

---

## 12) Docker и CI/CD

1. `deploy/local/docker-compose.local.yml`:
   - postgres
   - redis (опционально)
   - main-service
   - auth-proxy
   - notification-worker
   - message-expirer

2. GitHub Actions:
   - lint (`golangci-lint`);
   - unit tests (`go test -race -short ./...`);
   - integration tests (postgres + api/ws сценарии);
   - сборка docker-образов;
   - деплой на staging/prod.

---

## 13) План работ (по спринтам)

### Sprint 1 (MVP backend foundation)
- каркас репозитория и конфиги;
- `main-service` + `/health`;
- миграции `users/dialogs/messages`;
- базовый auth (login/refresh/logout);
- отправка/чтение сообщений через HTTP;
- WebSocket подключение и `message_new`.

Критерий: два пользователя могут обменяться сообщениями онлайн.

### Sprint 2 (уведомления + badge)
- регистрация device token;
- notification-worker + интеграция APNs/FCM;
- серверный unread count;
- синхронизация badge на клиенте.

Критерий: офлайн-пользователь получает push, бейдж корректен.

### Sprint 3 (Face ID + session hardening)
- secure storage токенов;
- биометрический unlock на клиенте;
- ротация refresh токена;
- revoke сессий.

Критерий: вход в приложение защищен биометрией, refresh flow безопасен.

### Sprint 4 (TTL и удаление с экрана)
- `expires_at` + политика TTL;
- message-expirer;
- событие `message_deleted` в реальном времени;
- тесты на рассинхронизацию при reconnect.

Критерий: сообщения исчезают по таймеру у обоих пользователей и в БД.

---

## 14) Риски и решения

1. Push на iOS нестабилен в dev-окружении
   - Решение: ранний staging с реальными APNs credentials.

2. Рассинхрон бейджа между клиентом и сервером
   - Решение: сервер — единственный источник истины, периодическая re-sync.

3. Потеря websocket-событий
   - Решение: sequence id + догрузка пропущенных событий через REST при reconnect.

4. Удаление по TTL расходится между устройствами
   - Решение: authoritative delete на сервере + обязательное `message_deleted` событие.

---

## 15) Что делать прямо сейчас

1. Утвердить стек клиента (рекомендация: Capacitor + React/Ionic).
2. Зафиксировать auth-стратегию для MVP (локальный JWT или внешний IdP).
3. Создать skeleton `cmd/main-service` и `internal/{handlers,services,store,...}`.
4. Реализовать вертикальный срез:
   - login -> send message -> receive message -> mark read -> unread count.
5. После этого подключать push и TTL.
