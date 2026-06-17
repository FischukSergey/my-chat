# API контракты Sprint 2 (push + badge)

Источник: `docs/sprint-2-plan.md`, `docs/sprint-2-checklist.md`, `docs/api-sprint-1.md`.

## 1) Зафиксированные решения Sprint 2

- Добавляются device endpoints в `main-service`:
  - `POST /api/v1/devices/register`;
  - `POST /api/v1/devices/unregister`.
- Добавляется серверный outbox для push-уведомлений: `notification_outbox`.
- Добавляется WebSocket событие `badge_updated`.
- Правило доставки push:
  - получатель online по WS -> outbox-задача не создается;
  - получатель offline по WS -> создается outbox-задача.
- Единый формат ошибок API не меняется относительно Sprint 1.

## 2) Scope Sprint 2 по контрактам

В Sprint 2 входят:
- контракты Device API;
- формат outbox payload;
- контракт `badge_updated`;
- правила online/offline ветвления для публикации в outbox.

В Sprint 2 не входят:
- production-специфика APNs/FCM (сертификаты, ключи, квоты);
- клиентская логика мобильного приложения;
- изменение контрактов Sprint 1 auth/chat ручек.

## 3) Общие соглашения

- Base path API: `/api/v1`.
- Формат данных: `application/json`.
- Идентификаторы: UUID в строковом виде.
- Время: RFC3339 (`2026-04-04T12:34:56Z`).
- Аутентификация защищенных endpoint:
  - `Authorization: Bearer <access_token>`.
- Идемпотентность:
  - повторный `register` для того же `(user_id, platform, push_token)` не создает дубликаты;
  - повторный `unregister` для уже отключенного токена возвращает успех.

## 4) Формат ошибок

Все неуспешные ответы возвращаются в формате:

```json
{
  "error": {
    "code": "invalid_argument",
    "message": "platform is invalid",
    "details": {
      "field": "platform"
    }
  }
}
```

Коды ошибок Sprint 2:
- `invalid_argument` (400)
- `unauthenticated` (401)
- `forbidden` (403)
- `not_found` (404)
- `conflict` (409)
- `internal` (500)

## 5) Device API (`main-service`)

### `POST /api/v1/devices/register`

Назначение: зарегистрировать push-устройство текущего пользователя или обновить существующую запись.

Request:

```json
{
  "platform": "ios",
  "push_token": "apns_device_token",
  "device_id": "f2f6cf73-1f6f-4428-b0fc-8f9f0ee9a145"
}
```

Правила валидации:
- `platform` обязателен, допустимые значения: `ios`, `android`, `web`.
- `push_token` обязателен, после `trim` не пустой, длина `1..1024`.
- `device_id` optional, UUID; если не передан, сервер может хранить `NULL`.

Поведение:
- upsert по уникальному ключу `(user_id, platform, push_token)`;
- при upsert обновляются:
  - `enabled=true`;
  - `last_seen_at=now()`;
  - `device_id` (если передан).

Response `200`:

```json
{
  "device": {
    "id": "2bdbf257-2b33-48ec-a6f8-e8a6ccd09444",
    "user_id": "22222222-2222-2222-2222-222222222222",
    "platform": "ios",
    "push_token": "apns_device_token",
    "enabled": true,
    "last_seen_at": "2026-04-04T12:34:56Z"
  }
}
```

### `POST /api/v1/devices/unregister`

Назначение: отключить push-токен текущего пользователя.

Request:

```json
{
  "platform": "ios",
  "push_token": "apns_device_token"
}
```

Правила валидации:
- `platform` обязателен, допустимые значения: `ios`, `android`, `web`.
- `push_token` обязателен, после `trim` не пустой, длина `1..1024`.

Поведение:
- soft-disable: `enabled=false`, `last_seen_at=now()`;
- физическое удаление строки не выполняется;
- операция идемпотентна (повторный вызов возвращает успех).

Response `204`: пустое тело.

## 6) `notification_outbox` payload

Назначение: внутренний контракт задачи для `notification-worker`.

Обязательные поля payload:
- `event_type` (`message_new`);
- `user_id` (получатель push);
- `message_id`;
- `dialog_id`;
- `sender_id`;
- `preview` (текст превью для уведомления);
- `unread_count` (актуальное серверное значение);
- `created_at` (время формирования payload);
- `dedup_key` (идемпотентный ключ задачи).

Пример payload:

```json
{
  "event_type": "message_new",
  "user_id": "22222222-2222-2222-2222-222222222222",
  "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
  "sender_id": "11111111-1111-1111-1111-111111111111",
  "preview": "hello, this is a new message",
  "unread_count": 3,
  "created_at": "2026-04-04T12:34:56Z",
  "dedup_key": "message_new:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa:22222222-2222-2222-2222-222222222222"
}
```

Ограничения:
- `preview` хранится в усеченном виде, максимум `120` символов.
- `preview` не должен содержать служебные переносы строк (`\n`, `\r`), они заменяются пробелом.
- `dedup_key` обязан быть детерминированным и уникальным для комбинации `event_type + message_id + receiver_user_id`.

## 7) WebSocket событие `badge_updated`

Событие отправляется пользователю при изменении его серверного `unread_count` (например, после `read`).

Формат события:

```json
{
  "event": "badge_updated",
  "data": {
    "user_id": "22222222-2222-2222-2222-222222222222",
    "unread_count": 2,
    "badge": 2,
    "reason": "message_read"
  },
  "ts": "2026-04-04T12:35:15Z"
}
```

Правила:
- `badge` и `unread_count` в Sprint 2 имеют одинаковое значение.
- `reason` допустимые значения:
  - `message_read` (после `POST /api/v1/messages/{id}/read`);
  - `sync` (фоновая серверная пересинхронизация состояния).

## 8) Правила online/offline и публикации в outbox

### Ветка online

Если получатель online по WS в момент `SendMessage`:
- отправляется `message_new` по WS;
- outbox-задача push **не** создается;
- `message_delivered` работает по текущему best-effort правилу Sprint 1.

### Ветка offline

Если получатель offline по WS в момент `SendMessage`:
- `message_new` в WS не отправляется;
- создается запись в `notification_outbox` со статусом `pending`;
- задача создается ровно один раз на комбинацию `event_type + message_id + receiver_user_id` (через `dedup_key`).

### Неопределенное состояние соединения

Если состояние WS нельзя надежно определить:
- применяется fallback в сторону outbox (создаем задачу);
- дубли отсекаются через `dedup_key` и идемпотентную обработку в worker.

## 9) Критерий совместимости

Любая реализация Sprint 2 должна соблюдать этот документ:
- handlers/services не меняют описанные здесь поля и значения без обновления файла;
- debug-сценарии используют эти же endpoint/events;
- тесты Sprint 2 проверяют именно эти контракты.
