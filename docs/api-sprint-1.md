# API контракты Sprint 1 (MVP)

Источник: `docs/sprint-1-plan.md`, `docs/sprint-1-checklist.md`, `docs/chat-architecture-plan.md`.

## 1) Зафиксированные решения Sprint 1

- Auth-стратегия: отдельный `auth-proxy` с JWT (`access` + `refresh`).
- `main-service` доверяет JWT и извлекает `user_id` через middleware.
- Realtime события Sprint 1: `message_new`, `message_delivered`, `message_read`.
- Единый формат ошибок API: `code`, `message`, `details`.

## 2) Границы MVP

В Sprint 1 входят:
- auth flow (`login`, `refresh`, `logout`);
- чат по HTTP (`send`, `list`, `read`, `unread_count`);
- WebSocket подключение и события доставки/прочтения;
- отладка через `/debug`.

В Sprint 1 не входят:
- push/APNs/FCM;
- `message_deleted`, TTL-очистка;
- devices API;
- production hardening (RBAC, rate-limit, advanced security policies).

## 3) Общие соглашения

- Base path API: `/api/v1`.
- Формат данных: `application/json`.
- Идентификаторы: UUID в строковом виде.
- Время: RFC3339 (`2026-03-23T12:34:56Z`).
- Аутентификация защищенных ручек:
  - `Authorization: Bearer <access_token>`.

## 4) Формат ошибок

Все неуспешные ответы возвращаются в формате:

```json
{
  "error": {
    "code": "invalid_argument",
    "message": "dialog_id is invalid",
    "details": {
      "field": "dialog_id"
    }
  }
}
```

Коды ошибок Sprint 1:
- `invalid_argument` (400)
- `unauthenticated` (401)
- `forbidden` (403)
- `not_found` (404)
- `conflict` (409)
- `internal` (500)

## 5) Auth API (`auth-proxy`)

### `POST /api/v1/auth/login`

Назначение: вход пользователя и выдача токенов.

Request:

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111"
}
```

Response `200`:

```json
{
  "access_token": "jwt-access",
  "refresh_token": "jwt-refresh",
  "token_type": "Bearer",
  "expires_in": 900
}
```

### `POST /api/v1/auth/refresh`

Назначение: обновление access-токена.

Request:

```json
{
  "refresh_token": "jwt-refresh"
}
```

Response `200`:

```json
{
  "access_token": "jwt-access-new",
  "refresh_token": "jwt-refresh-new",
  "token_type": "Bearer",
  "expires_in": 900
}
```

### `POST /api/v1/auth/logout`

Назначение: завершение сессии (MVP: best-effort invalidation refresh).

Request:

```json
{
  "refresh_token": "jwt-refresh"
}
```

Response `204`: пустое тело.

## 6) Chat HTTP API (`main-service`)

### `GET /api/v1/dialogs/{id}/messages?limit=&before=`

Назначение: получить историю сообщений диалога (по убыванию времени, пагинация).

Параметры:
- `limit` (optional, default 50, max 100)
- `before` (optional, RFC3339)

Response `200`:

```json
{
  "items": [
    {
      "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
      "dialog_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
      "sender_id": "11111111-1111-1111-1111-111111111111",
      "body": "hello",
      "created_at": "2026-03-23T12:00:00Z"
    }
  ],
  "next_before": "2026-03-23T11:59:59Z"
}
```

### `POST /api/v1/dialogs/{id}/messages`

Назначение: отправить сообщение в диалог.

Request:

```json
{
  "body": "hello"
}
```

Response `201`:

```json
{
  "message": {
    "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "hello",
    "created_at": "2026-03-23T12:00:00Z"
  }
}
```

### `POST /api/v1/messages/{id}/read`

Назначение: отметить сообщение прочитанным текущим пользователем.

Response `204`: пустое тело.

### `GET /api/v1/me/unread-count`

Назначение: получить число непрочитанных сообщений для текущего пользователя.

Response `200`:

```json
{
  "unread_count": 3
}
```

## 7) WebSocket API (`main-service`)

### `GET /ws/connect`

Подключение пользователя к realtime-каналу.

Требования:
- валидный `Authorization: Bearer <access_token>`;
- при невалидном токене сервер закрывает handshake с `401`.

Формат событий:

```json
{
  "event": "message_new",
  "data": {},
  "ts": "2026-03-23T12:00:00Z"
}
```

События Sprint 1:

1) `message_new` (получателю)

```json
{
  "event": "message_new",
  "data": {
    "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "hello",
    "created_at": "2026-03-23T12:00:00Z"
  },
  "ts": "2026-03-23T12:00:00Z"
}
```

2) `message_delivered` (отправителю, MVP best-effort)

```json
{
  "event": "message_delivered",
  "data": {
    "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
    "user_id": "22222222-2222-2222-2222-222222222222",
    "delivered_at": "2026-03-23T12:00:01Z"
  },
  "ts": "2026-03-23T12:00:01Z"
}
```

3) `message_read` (отправителю)

```json
{
  "event": "message_read",
  "data": {
    "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
    "user_id": "22222222-2222-2222-2222-222222222222",
    "read_at": "2026-03-23T12:00:05Z"
  },
  "ts": "2026-03-23T12:00:05Z"
}
```

## 8) Критерий совместимости для следующих шагов

Любой следующий этап реализации Sprint 1 должен соблюдать этот документ:
- новые handlers/services не меняют указанные контракты без явного обновления файла;
- `/debug` использует эти же маршруты и форматы;
- integration-тест `login -> send -> list -> read -> unread` проверяет именно эти поля и коды.
