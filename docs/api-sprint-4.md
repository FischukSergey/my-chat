# API контракты Sprint 4 (TTL и удаление сообщений)

Источник: `docs/sprint-4-checklist.md`, `docs/chat-architecture-plan.md` §13.

## 1) Зафиксированные решения Sprint 4

- TTL задаётся **глобально** через конфиг сервиса (`chat.message_ttl_seconds`); per-dialog политики — вне scope Sprint 4.
- Таблица `messages` расширяется двумя полями: `expires_at TIMESTAMPTZ NULL` и `deleted_at TIMESTAMPTZ NULL`.
- **Семантика TTL: "удаление после прочтения"** — `expires_at` устанавливается не при создании, а при **первом прочтении** получателем (`expires_at = read_at + ttl`). До прочтения `expires_at = NULL`.
- Если TTL = 0 — сообщение не истекает никогда (`expires_at` остаётся `NULL`).
- **Soft delete**: `message-expirer` проставляет `deleted_at = now()`, физически строку не удаляет.
- Все запросы `ListMessages` фильтруют `WHERE deleted_at IS NULL`.
- При прочтении: сервер отправляет WS-событие `message_ttl_started` обоим участникам — таймер запускается синхронно у обоих.
- При истечении TTL сервер публикует WS-событие `message_deleted` обоим участникам диалога (если онлайн).
- Если получатель никогда не прочитал сообщение — оно остаётся в БД бессрочно (принудительного hard deadline нет, вне scope Sprint 4).
- Auth API (`auth-proxy`) и Device API **не меняются** в Sprint 4.
- Добавляется новый код ошибки `user_inactive` (403) при попытке войти заблокированным пользователем (tech debt из Sprint 3).
- Добавляется rate-limiting на `POST /api/v1/auth/login` (429 с заголовком `Retry-After`).

## 2) Scope Sprint 4 по контрактам

В Sprint 4 входят:
- обновлённый контракт `POST /api/v1/dialogs/{dialog_id}/messages` (поле `expires_at` в ответе, всегда `null` при создании);
- обновлённый контракт `GET /api/v1/dialogs/{dialog_id}/messages` (поле `expires_at` в ответе, `deleted_at IS NULL` фильтрация);
- новое WS-событие `message_ttl_started` (сигнализирует о начале отсчёта после прочтения);
- новое WS-событие `message_deleted`;
- поле `expires_at` в WS-событии `message_new` всегда `null` (таймер не запущен);
- новый код ошибки `user_inactive` (403) для `POST /api/v1/auth/login`;
- rate-limiting на `POST /api/v1/auth/login`.

В Sprint 4 не входят:
- per-dialog настройки TTL;
- изменение Device API;
- production APNs/FCM;
- физическое удаление сообщений (`HARD DELETE`);
- OAuth / парольная аутентификация.

## 3) Общие соглашения

Без изменений относительно Sprint 3:

- Base path API: `/api/v1`.
- Формат данных: `application/json`.
- Идентификаторы: UUID в строковом виде.
- Время: RFC3339 (`2026-07-15T12:34:56Z`).
- Аутентификация защищённых endpoints: `Authorization: Bearer <access_token>`.

## 4) Формат ошибок

Формат неизменен. Новые коды ошибок Sprint 4:

| Код | HTTP | Когда используется |
|-----|------|--------------------|
| `user_inactive` | 403 | Login пользователя со статусом не `active` |
| `rate_limit_exceeded` | 429 | Превышен лимит попыток входа с данного IP |

### Коды ошибок из Sprint 1–3 (остаются)

| Код | HTTP |
|-----|------|
| `invalid_argument` | 400 |
| `unauthenticated` | 401 |
| `session_expired` | 401 |
| `session_revoked` | 401 |
| `session_compromised` | 401 |
| `forbidden` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `internal` | 500 |

## 5) Модель данных — изменения

### Таблица `messages` (расширение через миграцию 008)

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | uuid | Без изменений |
| `dialog_id` | uuid | Без изменений |
| `sender_id` | uuid | Без изменений |
| `body` | text | Без изменений |
| `created_at` | timestamptz | Без изменений |
| `expires_at` | timestamptz, nullable | **Новое.** Время истечения; NULL = сообщение не истекает |
| `deleted_at` | timestamptz, nullable | **Новое.** Время soft-delete; NULL = сообщение активно |

### Структура `Message` в API-ответах (обновлённая)

```json
{
  "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
  "sender_id": "11111111-1111-1111-1111-111111111111",
  "body": "Привет!",
  "created_at": "2026-07-23T14:00:00Z",
  "expires_at": "2026-07-23T14:05:00Z"
}
```

Поле `expires_at`:
- присутствует всегда в ответах API Sprint 4;
- `null` — если TTL не настроен (`chat.message_ttl_seconds = 0`);
- RFC3339 строка — если TTL задан.

Поле `deleted_at` **не включается** в API-ответы — оно служебное (soft delete). Клиенты узнают об удалении через WS-событие `message_deleted`.

## 6) Chat API (`main-service`) — изменения

### `POST /api/v1/dialogs/{dialog_id}/messages`

**Изменение:** добавлено поле `expires_at` в ответ.

Request (без изменений):

```json
{
  "body": "Текст сообщения"
}
```

Правила валидации (без изменений):
- `body` обязателен, не пустая строка.

Поведение (изменение Sprint 4):
- при сохранении сообщения вычисляется `expires_at = now() + chat.message_ttl_seconds`;
- если `chat.message_ttl_seconds = 0` → `expires_at = NULL`.

Response `201`:

```json
{
  "message": {
    "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "Текст сообщения",
    "created_at": "2026-07-23T14:00:00Z",
    "expires_at": "2026-07-23T14:05:00Z"
  }
}
```

Ошибки (без изменений):
- `400 invalid_argument` — пустое тело.
- `401 unauthenticated` — невалидный access token.
- `403 forbidden` — текущий пользователь не участник диалога.
- `404 not_found` — диалог не существует.
- `500 internal`.

---

### `GET /api/v1/dialogs/{dialog_id}/messages`

**Изменение:** добавлено поле `expires_at` в каждом сообщении; удалённые сообщения (`deleted_at IS NOT NULL`) не возвращаются.

Query params (без изменений):
- `limit` — int, по умолчанию 50.
- `before` — UUID, курсор пагинации.

Response `200`:

```json
{
  "messages": [
    {
      "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
      "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
      "sender_id": "11111111-1111-1111-1111-111111111111",
      "body": "Привет!",
      "created_at": "2026-07-23T14:00:00Z",
      "expires_at": "2026-07-23T14:05:00Z"
    }
  ],
  "next_cursor": null
}
```

Поведение при reconnect:
- клиент запрашивает `GET .../messages` после восстановления WS → получает только активные сообщения (soft-deleted исключены).

---

## 7) WebSocket события — изменения

### Существующее событие `message_new` (изменение)

**Добавлено поле `expires_at`.**

```json
{
  "type": "message_new",
  "message": {
    "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
    "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "Привет!",
    "created_at": "2026-07-23T14:00:00Z",
    "expires_at": "2026-07-23T14:05:00Z"
  }
}
```

---

### Новое событие `message_deleted`

Публикуется `message-expirer`-ом через Hub обоим участникам диалога при истечении TTL.

```json
{
  "type": "message_deleted",
  "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd"
}
```

Поля:

| Поле | Тип | Описание |
|------|-----|----------|
| `type` | string | Всегда `"message_deleted"` |
| `message_id` | string (UUID) | ID удалённого сообщения |
| `dialog_id` | string (UUID) | ID диалога |

Поведение на клиенте:
- получив `message_deleted` → немедленно скрыть пузырь с `message_id` из UI;
- отменить активный таймер обратного отсчёта для этого сообщения;
- если пользователь был офлайн — при reconnect `ListMessages` не вернёт удалённое сообщение.

---

### Новое событие `message_ttl_started`

Публикуется сервером при **первом прочтении** сообщения получателем (вызов `MarkRead`).
Получают **оба** участника диалога — и отправитель, и получатель.

```json
{
  "type": "message_ttl_started",
  "message_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "dialog_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
  "expires_at": "2026-07-23T14:10:00Z"
}
```

Поля:

| Поле | Тип | Описание |
|------|-----|----------|
| `type` | string | Всегда `"message_ttl_started"` |
| `message_id` | string (UUID) | ID сообщения |
| `dialog_id` | string (UUID) | ID диалога |
| `expires_at` | string (RFC3339) | Время истечения (`read_at + ttl`) |

Поведение на клиенте:
- получив `message_ttl_started` → запустить таймер обратного отсчёта для `message_id`;
- таймер показывать обоим участникам (и отправителю, и получателю);
- при `expires_at - now() <= 0` → скрыть пузырь (до `message_deleted` от сервера).

---

### Существующие события (без изменений)

| Тип | Описание |
|-----|----------|
| `message_delivered` | Сообщение доставлено онлайн-получателю |
| `message_read` | Получатель прочитал сообщение |
| `badge_updated` | Обновление счётчика непрочитанных |

---

## 8) Auth API — изменения Sprint 4

### `POST /api/v1/auth/login` — новое поведение

**Изменение:** добавлена проверка статуса пользователя.

Поведение (добавление к Sprint 3):
1. Найти пользователя по `user_id` в таблице `users`.
2. Если `users.status != 'active'` → вернуть `403 user_inactive`.
3. Остальная логика без изменений (создание сессии, выдача token pair).

Новая ошибка:
- `403 user_inactive` — пользователь существует, но его статус не `active`.

**Rate limiting:**
- Лимит: не более **10 запросов** с одного IP за **60 секунд**.
- При превышении: `429 Too Many Requests` с заголовком `Retry-After: <seconds>`.

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "too many login attempts, try again later",
    "details": {}
  }
}
```

---

## 9) Конфигурация — новые поля

### `config.main-service.local.example.yaml` (добавление)

```yaml
chat:
  message_ttl_seconds: 300  # 5 минут; 0 = TTL не используется
```

### `config.message-expirer.local.example.yaml` (добавление)

```yaml
expirer:
  interval_seconds: 10   # интервал тикера в секундах
  batch_size: 100        # максимальное число сообщений за одну итерацию

database:
  dsn: postgres://chat_service:chat_service@localhost:33432/chat_service?sslmode=disable
```

---

## 10) Поведение `message-expirer`

Алгоритм одной итерации тикера (каждые `expirer.interval_seconds`):

```
1. expired_messages = UPDATE messages
     SET deleted_at = now()
     WHERE expires_at <= now() AND deleted_at IS NULL
     RETURNING id, dialog_id, sender_id
     LIMIT batch_size

   Примечание: expires_at устанавливается только при прочтении (MarkRead),
   поэтому непрочитанные сообщения с expires_at = NULL в выборку не попадают.

2. Для каждого dialog_id в expired_messages:
   - получить список user_id участников диалога (из dialogs)
   - для каждого онлайн-участника в Hub:
     - отправить WS событие message_deleted {type, message_id, dialog_id}

3. Логировать: message_expired count=N duration_ms=M
```

Гарантии:
- атомарность: `SET deleted_at` и `RETURNING` выполняются в одном запросе;
- idempotency: `WHERE deleted_at IS NULL` исключает повторную обработку;
- batch limit: не более `batch_size` сообщений за итерацию (защита от long-running query).

---

## 11) Поведение клиента — TTL таймер (мобильный)

### Жизненный цикл сообщения с TTL

```
Отправитель пишет → message_new (expires_at: null)  → без таймера
Получатель читает → message_read + message_ttl_started (expires_at: T)
                  → оба участника запускают обратный отсчёт
T истекает        → message_deleted → оба участника скрывают пузырь
```

### Алгоритм на клиенте

При получении **`message_new`** (WS) или загрузке **`ListMessages`**:
1. Если `expires_at != null` → сообщение уже прочитано ранее; запустить таймер от текущего момента.
2. Если `expires_at == null` → ждать события `message_ttl_started`.

При получении **`message_ttl_started`**:
1. Найти пузырь с `message_id` в UI.
2. Запустить таймер (`setInterval`): `remaining = expires_at - now()`, обновление каждую секунду.
3. Отображать оставшееся время в формате `MM:SS`.
4. При `remaining <= 0` → скрыть пузырь (упреждающее удаление до прихода `message_deleted`).

При получении **`message_deleted`**:
1. Отменить активный таймер.
2. Скрыть пузырь (если ещё не скрыт упреждающим удалением).

Примечание: сообщения без прочтения (`expires_at == null`) остаются в UI бессрочно — таймер не показывается.

---

## 12) E2E сценарии (тестовые кейсы)

### Сценарий 1: Полный цикл — отправка → прочтение → удаление

```
POST /dialogs/{id}/messages {body: "Привет"}
  → 201 {message: {id: MSG_ID, expires_at: null}}  ← таймер не запущен

WS user_a: message_new {id: MSG_ID, expires_at: null}
WS user_b: message_new {id: MSG_ID, expires_at: null}

user_b вызывает POST /messages/MSG_ID/read

WS user_a: message_read {message_id: MSG_ID, ...}
WS user_a: message_ttl_started {message_id: MSG_ID, expires_at: T+5min}
WS user_b: message_ttl_started {message_id: MSG_ID, expires_at: T+5min}
  ← оба участника запускают обратный отсчёт

(через 5 минут + до 10 сек expirer interval)

WS user_a: message_deleted {message_id: MSG_ID, dialog_id: DLG_ID}
WS user_b: message_deleted {message_id: MSG_ID, dialog_id: DLG_ID}

GET /dialogs/{id}/messages
  → 200 {messages: []}  ← MSG_ID отсутствует
```

### Сценарий 2: Получатель никогда не прочитал

```
POST /dialogs/{id}/messages {body: "Привет"}
  → 201 {message: {id: MSG_ID, expires_at: null}}

(через часы/дни — user_b так и не открыл диалог)

GET /dialogs/{id}/messages (от user_b)
  → 200 {messages: [{id: MSG_ID, expires_at: null}]}  ← сообщение живёт
```

### Сценарий 3: Reconnect после истечения TTL

```
user_b прочитал и ушёл офлайн во время expires_at

user_b reconnects WS

user_b calls GET /dialogs/{id}/messages
  → 200 {messages: []}  ← удалённые сообщения не возвращаются
```

### Сценарий 3: Login заблокированного пользователя

```
POST /auth/login {user_id: BLOCKED_USER_ID}
  → 403 user_inactive
```

### Сценарий 4: Rate limiting

```
POST /auth/login × 11 (с одного IP за 60 сек)
  → первые 10: 200 OK
  → 11-й: 429 rate_limit_exceeded (Retry-After: 42)
```

---

## 13) Обратная совместимость

| Компонент | Sprint 1–3 контракт | Sprint 4 изменение |
|-----------|---------------------|--------------------|
| `POST .../messages` response | без `expires_at` | `expires_at` добавлен (backward compatible) |
| `GET .../messages` response | без `expires_at` | `expires_at` добавлен (backward compatible) |
| WS `message_new` | без `expires_at` | `expires_at` добавлен, всегда `null` при создании (backward compatible) |
| WS `message_ttl_started` | не существовал | **Новое событие** (клиент без обработки просто игнорирует) |
| WS `message_deleted` | не существовал | **Новое событие** |
| `POST /auth/login` | без проверки статуса | добавлена проверка `status = active` |
| `POST /auth/login` | без rate-limiting | добавлен rate-limiting (429) |
| Auth / Device API | без изменений | без изменений |

**Важно:** клиент Sprint 1–3, не знающий о `expires_at` и `message_deleted`, продолжит работать — новые поля аддитивны, новое событие он просто проигнорирует.

---

## 14) Критерий совместимости

Любая реализация Sprint 4 должна соблюдать этот документ:
- handlers/services не меняют описанные здесь поля и значения без обновления файла;
- integration-тесты Sprint 4 проверяют именно эти контракты и поведение;
- мобильный клиент ориентируется на структуры из §5 и события из §7.
