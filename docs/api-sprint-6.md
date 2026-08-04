# API контракты Sprint 6 (Production Polish)

Источник: `docs/sprint-6-plan.md`, `docs/sprint-6-checklist.md`.

## 1) Зафиксированные решения Sprint 6

- **Auth**: переход с demo-логина по `user_id` на `username` + `password` (bcrypt, cost=12).
- **Регистрация**: новый публичный endpoint `POST /api/v1/users/register` на `main-service`.
- **Login**: `POST /api/v1/auth/login` принимает `{username, password}`; путь `user_id` удаляется.
- **Device binding**: опциональный заголовок `X-Device-ID` на login/refresh; mismatch → `403 device_mismatch`.
- **Web Push (VAPID)**: вместо APNs/FCM токена клиент передаёт объект `PushSubscription`; `push_token` остаётся nullable для будущего использования.
- **VAPID public key**: публичный `GET /api/v1/push/vapid-public-key`.
- **Platform**: `web` — основное значение для PWA; `ios`/`android` остаются допустимыми.
- **Rate-limit** на login (10 req / 60 sec / IP) сохраняется без изменений.
- Chat API, TTL, WS-события сообщений — без breaking changes относительно Sprint 4–5.

## 2) Scope Sprint 6 по контрактам

В Sprint 6 входят:
- схема `users`: поля `username`, `password_hash`;
- схема `devices`: поле `push_subscription` (JSONB), `push_token` становится nullable;
- `POST /api/v1/users/register`;
- обновлённый `POST /api/v1/auth/login` (username/password + опциональный `X-Device-ID`);
- обновлённый `POST /api/v1/auth/refresh` (проверка `X-Device-ID` при привязке);
- обновлённый `POST /api/v1/devices/register` (`push_subscription` для `platform=web`);
- `GET /api/v1/push/vapid-public-key`;
- новые коды ошибок: `invalid_credentials`, `username_taken`, `device_mismatch`.

В Sprint 6 не входят (контракты не меняются):
- OAuth / WebAuthn server-side;
- APNs/FCM payload-контракты;
- групповые чаты;
- изменение формата chat/WS message events (кроме silent push payload внутри worker).

## 3) Общие соглашения

Без изменений относительно Sprint 5:

- Prod base: `https://beepru.ru`.
- Path-based routing: `/api/v1/...` → `main-service`, `/auth/...` → `auth-proxy`, `/ws/connect` → `main-service`.
- Формат: `application/json`.
- Идентификаторы: UUID в строковом виде.
- Время: RFC3339.
- Защищённые endpoints: `Authorization: Bearer <access_token>`.

Prod URL для auth:

| Endpoint | Prod path |
|----------|-----------|
| login | `POST /auth/api/v1/auth/login` |
| refresh | `POST /auth/api/v1/auth/refresh` |
| logout | `POST /auth/api/v1/auth/logout` |
| register | `POST /api/v1/users/register` |
| vapid key | `GET /api/v1/push/vapid-public-key` |
| devices | `POST /api/v1/devices/register` |

## 4) Формат ошибок

Формат тела без изменений:

```json
{
  "error": {
    "code": "invalid_credentials",
    "message": "invalid username or password",
    "details": {}
  }
}
```

### Новые коды ошибок Sprint 6

| Код | HTTP | Когда используется |
|-----|------|--------------------|
| `invalid_credentials` | 401 | Неверный username/password при login |
| `username_taken` | 409 | Username уже занят при register |
| `device_mismatch` | 403 | Refresh с другим `X-Device-ID`, чем привязан к сессии |

### Коды из Sprint 1–5 (остаются)

| Код | HTTP |
|-----|------|
| `invalid_argument` | 400 |
| `unauthenticated` | 401 |
| `session_expired` | 401 |
| `session_revoked` | 401 |
| `session_compromised` | 401 |
| `forbidden` | 403 |
| `user_inactive` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `rate_limit_exceeded` | 429 |
| `internal` | 500 |

Примечание: для занятого username используется **`username_taken`**, а не общий `conflict`.

---

## 5) Модель данных — изменения

### Таблица `users` (миграция `010_user_credentials.sql`)

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | uuid PK | Без изменений |
| `status` | text | Без изменений (`active` / `blocked`) |
| `created_at` | timestamptz | Без изменений |
| `username` | text UNIQUE NOT NULL | **Новое.** Логин пользователя |
| `password_hash` | text NOT NULL | **Новое.** bcrypt hash (cost=12) |

Индекс: `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username)`.

Правила для `username`:
- длина 3–50;
- допустимые символы: латиница `a-zA-Z`, цифры `0-9`, `_`;
- регистр сохраняется как введён; уникальность — case-sensitive на уровне БД (клиент нормализует ввод trim).

Правила для `password` (только на API, в БД хранится hash):
- минимум 8 символов;
- максимум не фиксируется жёстко на схеме; handler отклоняет пустой/короткий пароль.

Seed (local): `deploy/local/seed-users.sql` — тестовые пользователи с username + bcrypt-hash.

### Таблица `devices` (миграция `011_device_push_subscription.sql`)

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | uuid PK | Без изменений |
| `user_id` | uuid FK | Без изменений |
| `platform` | text | `ios` \| `android` \| `web` |
| `push_token` | text **NULL** | Legacy / future APNs-FCM; для `web` может быть NULL |
| `push_subscription` | jsonb NULL | **Новое.** Web Push subscription object |
| `enabled` | boolean | Без изменений |
| `last_seen_at` | timestamptz | Без изменений |
| `created_at` / `updated_at` | timestamptz | Без изменений |

Формат `push_subscription` в БД (JSONB):

```json
{
  "endpoint": "https://web.push.apple.com/...",
  "keys": {
    "p256dh": "base64url...",
    "auth": "base64url..."
  }
}
```

Upsert-ключ для `platform=web`:
- уникальность по `(user_id, platform, endpoint)`, где `endpoint` берётся из `push_subscription.endpoint`;
- при повторной регистрации того же endpoint: `enabled=true`, обновляются `push_subscription` и `last_seen_at`.

Для `ios`/`android` (если используются): допускается передача `push_token` как раньше; `push_subscription` не обязателен.

---

## 6) Users API (`main-service`)

### `POST /api/v1/users/register`

Назначение: создать нового пользователя с credentials. Публичный роут (без auth middleware).

Request:

```json
{
  "username": "alice",
  "password": "secret123"
}
```

Правила валидации:
- `username` обязателен, после `trim` длина 3–50, regex `^[a-zA-Z0-9_]+$`;
- `password` обязателен, длина ≥ 8.

Поведение:
1. `bcrypt.GenerateFromPassword(password, 12)`.
2. `INSERT INTO users (id, username, password_hash, status)` — `id` генерируется UUID, `status='active'`.
3. При нарушении UNIQUE по `username` → `409 username_taken`.

Response `201 Created`:

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111"
}
```

Ошибки:
- `400 invalid_argument` — невалидный username/password.
- `409 username_taken` — username уже существует.
- `500 internal` — ошибка БД / bcrypt.

Примечание: регистрация **не** выдаёт токены. Клиент после `201` переходит на login.

---

## 7) Auth API (`auth-proxy`) — изменения

### `POST /api/v1/auth/login` (prod: `POST /auth/api/v1/auth/login`)

**Breaking change:** тело `{ "user_id": "..." }` удаляется.

Request:

```json
{
  "username": "alice",
  "password": "secret123"
}
```

Опциональный заголовок:

```
X-Device-ID: <uuid>
```

Правила валидации:
- `username` обязателен, непустой после trim;
- `password` обязателен, непустой;
- `X-Device-ID` optional; если передан — валидный UUID.

Поведение:
1. `UserRepository.FindByUsername(username)`.
2. Если пользователь не найден → `401 invalid_credentials` (без раскрытия, существует ли username).
3. Если `status != active` → `403 user_inactive`.
4. `bcrypt.CompareHashAndPassword` → при несовпадении `401 invalid_credentials`.
5. Создать `auth_sessions` (как в Sprint 3); если передан `X-Device-ID` — сохранить в `auth_sessions.device_id`.
6. Выдать access + refresh (+ `session_id` в ответе, как в Sprint 3).

Response `200`:

```json
{
  "access_token": "jwt-access",
  "refresh_token": "jwt-refresh",
  "token_type": "Bearer",
  "expires_in": 900,
  "session_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
}
```

Ошибки:
- `400 invalid_argument` — отсутствующие/невалидные поля.
- `401 invalid_credentials` — неверный логин или пароль.
- `403 user_inactive` — пользователь заблокирован.
- `429 rate_limit_exceeded` — >10 попыток с IP за 60 сек (`Retry-After`).
- `500 internal`.

### `POST /api/v1/auth/refresh` (prod: `POST /auth/api/v1/auth/refresh`)

Request (тело без изменений):

```json
{
  "refresh_token": "jwt-refresh-old"
}
```

Опциональный заголовок:

```
X-Device-ID: <uuid>
```

Дополнительное поведение Sprint 6:
1. После успешной валидации сессии: если у сессии `device_id IS NOT NULL`, сравнить с заголовком `X-Device-ID`.
2. Если заголовок отсутствует или не совпадает → `403 device_mismatch` (сессию не ротировать).
3. Если у сессии `device_id IS NULL` — проверка не выполняется (обратная совместимость старых сессий).

Остальные ошибки refresh без изменений (`session_revoked`, `session_compromised`, `session_expired`, …).

### `POST /api/v1/auth/logout`

Без изменений относительно Sprint 3.

---

## 8) Push / Devices API (`main-service`)

### `GET /api/v1/push/vapid-public-key`

Назначение: отдать публичный VAPID-ключ для `pushManager.subscribe()`. Публичный роут (без auth).

Response `200`:

```json
{
  "public_key": "<base64url-encoded-uncompressed-point>"
}
```

Ошибки:
- `500 internal` — ключ не сконфигурирован на сервере.

### `POST /api/v1/devices/register`

Назначение: зарегистрировать устройство для push. Требует auth.

Request для PWA (`platform=web`):

```json
{
  "platform": "web",
  "push_subscription": {
    "endpoint": "https://web.push.apple.com/...",
    "keys": {
      "p256dh": "base64url...",
      "auth": "base64url..."
    }
  },
  "device_id": "f2f6cf73-1f6f-4428-b0fc-8f9f0ee9a145"
}
```

Request для legacy token-платформ (опционально, не основной путь Sprint 6):

```json
{
  "platform": "ios",
  "push_token": "apns_device_token",
  "device_id": "f2f6cf73-1f6f-4428-b0fc-8f9f0ee9a145"
}
```

Правила валидации:
- `platform` обязателен: `ios` | `android` | `web`.
- Для `platform=web`:
  - `push_subscription` обязателен;
  - `push_subscription.endpoint` — непустой URL;
  - `push_subscription.keys.p256dh` и `keys.auth` — непустые строки;
  - `push_token` не требуется.
- Для `platform=ios|android`:
  - `push_token` обязателен (как в Sprint 2: trim, длина 1..1024);
  - `push_subscription` не требуется.
- `device_id` optional, UUID.

Поведение:
- upsert + `enabled=true`, `last_seen_at=now()`;
- для `web` сохраняется JSON `push_subscription`.

Response `200`:

```json
{
  "device": {
    "id": "2bdbf257-2b33-48ec-a6f8-e8a6ccd09444",
    "user_id": "22222222-2222-2222-2222-222222222222",
    "platform": "web",
    "push_subscription": {
      "endpoint": "https://web.push.apple.com/...",
      "keys": {
        "p256dh": "base64url...",
        "auth": "base64url..."
      }
    },
    "enabled": true,
    "last_seen_at": "2026-08-02T12:34:56Z"
  }
}
```

Ошибки:
- `400 invalid_argument` — невалидные поля.
- `401 unauthenticated`.
- `500 internal`.

### `POST /api/v1/devices/unregister`

Для `web` допускается отключение по `push_subscription.endpoint` (или по сохранённому `device` id — уточняется при реализации §8 чеклиста; минимальный контракт Sprint 6: unregister по тем же полям, что и register — `platform` + идентификатор подписки).

Минимальный request для web:

```json
{
  "platform": "web",
  "push_subscription": {
    "endpoint": "https://web.push.apple.com/..."
  }
}
```

Поведение без изменений: soft-disable (`enabled=false`), идемпотентно, `204`.

---

## 9) Внутренний контракт Web Push payload (notification-worker)

Не HTTP API для клиента; фиксируется для согласования SW и worker.

### Alert (обычное уведомление)

```json
{
  "title": "<preview>",
  "body": "Новое сообщение",
  "badge": 3,
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
  "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
}
```

### Silent badge sync (`event_type = badge_sync`)

```json
{
  "type": "badge_sync",
  "badge": 0
}
```

- без `title`/`body`;
- TTL доставки короткий (≈60 сек);
- Service Worker вызывает `setAppBadge(badge)` без `showNotification`.

HTTP 404/410 от push-endpoint → деактивировать подписку устройства в БД (`enabled=false`).

---

## 10) Миграция клиента (breaking)

| Было (Sprint 3–5) | Стало (Sprint 6) |
|-------------------|------------------|
| Login body `{user_id}` | `{username, password}` |
| Нет register | `POST /api/v1/users/register` |
| `devices/register` с `push_token` | для web — `push_subscription` |
| Нет VAPID endpoint | `GET /api/v1/push/vapid-public-key` |
| Refresh без device binding | опциональный `X-Device-ID` → `403 device_mismatch` |

Клиент PWA обязан:
1. Зарегистрировать пользователя (или использовать seed).
2. Login с username/password + `X-Device-ID`.
3. Получить VAPID public key → `pushManager.subscribe` → `devices/register` с `platform=web`.

---

## 11) Пример happy-path

```
POST /api/v1/users/register
  {username, password} → 201 {user_id}

POST /auth/api/v1/auth/login
  Headers: X-Device-ID: <uuid>
  {username, password} → 200 {access_token, refresh_token, session_id}

GET /api/v1/push/vapid-public-key
  → 200 {public_key}

POST /api/v1/devices/register
  Authorization: Bearer <access>
  {platform: "web", push_subscription: {...}, device_id: <uuid>}
  → 200 {device}

# далее chat API без изменений
```

---

## 12) Сводка изменений контрактов

| Endpoint | Изменение |
|----------|-----------|
| `POST /api/v1/users/register` | **Новый** |
| `POST /api/v1/auth/login` | body: username/password; header `X-Device-ID`; ошибка `invalid_credentials` |
| `POST /api/v1/auth/refresh` | header `X-Device-ID`; ошибка `device_mismatch` |
| `GET /api/v1/push/vapid-public-key` | **Новый** |
| `POST /api/v1/devices/register` | `push_subscription` для `web`; `push_token` не обязателен для web |
| Chat / WS message events | без breaking changes |
