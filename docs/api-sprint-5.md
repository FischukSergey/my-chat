# API контракты Sprint 5 (Production URL)

Источник: `docs/sprint-5-plan.md`, `docs/sprint-5-checklist.md`.

## 1) Зафиксированные решения Sprint 5

- **Prod URL**: `https://beepru.ru` — placeholder; заменить на реальный домен после его регистрации.
- **Маршрутизация**: path-based routing на одном домене (без поддоменов) через Nginx reverse proxy.
- **Протоколы**: HTTPS (TLS 1.2/1.3) для REST, WSS для WebSocket.
- **SSL**: Let's Encrypt + Certbot в Docker, автопродление.
- **API контракты**: без изменений относительно Sprint 4; Sprint 5 — только инфраструктурный деплой.

## 2) Base URL для мобильного клиента

```
https://beepru.ru
```

> **Важно:** `beepru.ru` — placeholder. После регистрации домена и добавления A-записи замените его на реальный домен во всех конфигах (`configs/`, `deploy/prod/`, мобильный клиент).

## 3) Таблица маршрутизации (Nginx → сервисы)

| Путь запроса | Протокол | Upstream | Описание |
|-------------|----------|----------|----------|
| `/api/v1/...` | HTTPS | `main-service:8080` | Chat API |
| `/ws/connect` | WSS | `main-service:8080` | WebSocket (upgrade) |
| `/auth/...` | HTTPS | `auth-proxy:33081` | Auth API |
| `/health` | HTTPS | `main-service:8080` | Health check |

## 4) Общие соглашения

Без изменений относительно Sprint 1–4:

- Base path REST API: `/api/v1`.
- Формат данных: `application/json`.
- Идентификаторы: UUID в строковом виде.
- Время: RFC3339 (`2026-07-30T12:34:56Z`).
- Аутентификация защищённых endpoints: `Authorization: Bearer <access_token>`.

## 5) Формат ошибок

Без изменений. Актуальный список кодов (Sprint 1–4):

| Код | HTTP | Когда используется |
|-----|------|--------------------|
| `invalid_argument` | 400 | Невалидный запрос |
| `unauthenticated` | 401 | Невалидный или отсутствующий access token |
| `session_expired` | 401 | Access token истёк |
| `session_revoked` | 401 | Сессия отозвана (logout) |
| `session_compromised` | 401 | Обнаружена смена user-agent при refresh |
| `forbidden` | 403 | Нет прав доступа к ресурсу |
| `user_inactive` | 403 | Пользователь заблокирован (Sprint 4) |
| `not_found` | 404 | Ресурс не найден |
| `conflict` | 409 | Конфликт данных |
| `rate_limit_exceeded` | 429 | Превышен лимит запросов (Sprint 4) |
| `internal` | 500 | Внутренняя ошибка сервера |

---

## 6) Auth API (`auth-proxy`) — prod endpoints

Base: `https://beepru.ru/auth`

### `POST /auth/api/v1/auth/login`

Вход пользователя и выдача JWT токенов.

Request:

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111"
}
```

Response `200`:

```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "token_type": "Bearer",
  "expires_in": 900
}
```

Ошибки:
- `400 invalid_argument` — невалидный `user_id`.
- `403 user_inactive` — пользователь заблокирован.
- `429 rate_limit_exceeded` — более 10 запросов с одного IP за 60 сек (`Retry-After` заголовок).

---

### `POST /auth/api/v1/auth/refresh`

Обновление access token по refresh token.

Request:

```json
{
  "refresh_token": "eyJ..."
}
```

Response `200`:

```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "token_type": "Bearer",
  "expires_in": 900
}
```

Ошибки:
- `401 session_expired` — refresh token истёк.
- `401 session_revoked` — сессия отозвана.
- `401 session_compromised` — обнаружена подозрительная активность.

---

### `POST /auth/api/v1/auth/logout`

Завершение сессии (отзыв refresh token).

Request:

```json
{
  "refresh_token": "eyJ..."
}
```

Response `204 No Content`.

---

## 7) Chat API (`main-service`) — prod endpoints

Base: `https://beepru.ru/api/v1`

Все endpoints требуют `Authorization: Bearer <access_token>`.

### `GET /api/v1/me/unread-count`

Получить общее количество непрочитанных сообщений.

Response `200`:

```json
{
  "unread_count": 5
}
```

---

### `POST /api/v1/dialogs/{dialog_id}/messages`

Отправить сообщение в диалог.

Path params:
- `dialog_id` — UUID диалога.

Request:

```json
{
  "body": "Текст сообщения"
}
```

Response `201`:

```json
{
  "message": {
    "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "Текст сообщения",
    "created_at": "2026-07-30T12:34:56Z",
    "expires_at": null
  }
}
```

Ошибки:
- `400 invalid_argument` — пустое тело.
- `401 unauthenticated` — невалидный access token.
- `403 forbidden` — текущий пользователь не участник диалога.
- `404 not_found` — диалог не существует.

---

### `GET /api/v1/dialogs/{dialog_id}/messages`

Получить историю сообщений диалога (с пагинацией).

Path params:
- `dialog_id` — UUID диалога.

Query params:
- `limit` — int, по умолчанию 50.
- `before` — UUID, курсор для пагинации (ID последнего полученного сообщения).

Response `200`:

```json
{
  "messages": [
    {
      "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
      "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
      "sender_id": "11111111-1111-1111-1111-111111111111",
      "body": "Привет!",
      "created_at": "2026-07-30T12:34:56Z",
      "expires_at": "2026-07-30T12:39:56Z"
    }
  ],
  "next_cursor": null
}
```

Примечание: удалённые сообщения (`deleted_at IS NOT NULL`) не возвращаются.

---

### `POST /api/v1/messages/{message_id}/read`

Отметить сообщение как прочитанное. Запускает TTL таймер (если `message_ttl_seconds > 0`).

Path params:
- `message_id` — UUID сообщения.

Response `204 No Content`.

Ошибки:
- `401 unauthenticated` — невалидный access token.
- `403 forbidden` — текущий пользователь не получатель сообщения.
- `404 not_found` — сообщение не существует.

---

### `POST /api/v1/devices/register`

Зарегистрировать push-токен устройства.

Request:

```json
{
  "token": "push-device-token",
  "platform": "ios"
}
```

Response `204 No Content`.

---

### `POST /api/v1/devices/unregister`

Удалить push-токен устройства.

Request:

```json
{
  "token": "push-device-token"
}
```

Response `204 No Content`.

---

## 8) Health check

### `GET /health`

Проверка доступности сервиса. Не требует аутентификации.

Response `200`:

```json
{
  "status": "ok"
}
```

---

## 9) WebSocket — prod endpoint

```
wss://beepru.ru/ws/connect?token=<access_token>
```

После успешного Upgrade (HTTP 101) клиент получает и отправляет JSON-сообщения.

### Параметры подключения

| Параметр | Тип | Описание |
|----------|-----|----------|
| `token` | string (query) | Валидный access JWT |

### Исходящие события (сервер → клиент)

| Тип | Описание |
|-----|----------|
| `message_new` | Новое сообщение в одном из диалогов пользователя |
| `message_delivered` | Сообщение доставлено онлайн-получателю |
| `message_read` | Получатель прочитал сообщение |
| `message_ttl_started` | TTL таймер запущен (после первого прочтения) |
| `message_deleted` | Сообщение удалено по истечении TTL |
| `badge_updated` | Обновление счётчика непрочитанных |

Пример события `message_new`:

```json
{
  "type": "message_new",
  "message": {
    "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "Привет!",
    "created_at": "2026-07-30T12:34:56Z",
    "expires_at": null
  }
}
```

Пример события `message_ttl_started`:

```json
{
  "type": "message_ttl_started",
  "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
  "expires_at": "2026-07-30T12:39:56Z"
}
```

Пример события `message_deleted`:

```json
{
  "type": "message_deleted",
  "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd"
}
```

---

## 10) Smoke-тесты для мобильного клиента

После смены placeholder на реальный домен проверить:

```bash
# Health check
curl https://beepru.ru/health

# Login
curl -X POST https://beepru.ru/auth/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id": "11111111-1111-1111-1111-111111111111"}'

# Unread count (с Bearer token)
curl https://beepru.ru/api/v1/me/unread-count \
  -H "Authorization: Bearer <access_token>"

# WebSocket (wscat или мобильный клиент)
wss://beepru.ru/ws/connect?token=<access_token>
```

---

## 11) Scope Sprint 5 по контрактам

В Sprint 5 входят:
- фиксация prod URL и маршрутизации (этот файл);
- инфраструктурные файлы: Dockerfiles, docker-compose.prod.yml, nginx конфиг, `.env.example`.

В Sprint 5 не входят:
- изменения в API контрактах;
- новые endpoints или WS события;
- Auth / Device API — без изменений.
