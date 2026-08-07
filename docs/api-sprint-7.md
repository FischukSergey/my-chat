# API Sprint 7 — Список диалогов / username

Базовый path: `/api/v1`. Auth: Bearer access JWT (как Sprint 6). Username — case-insensitive (нормализация `lower`, как login/register).

Связанные: `docs/api-sprint-6.md` (auth, messages, unread), `docs/sprint-7-plan.md`.

---

## 0) Утверждённые endpoints

Сервис: `main-service`. Все ниже — за `Authenticate` (кроме уже существующих публичных routes Sprint 6).

| Priority | Method | Path | Назначение |
|----------|--------|------|------------|
| Must | `GET` | `/api/v1/dialogs` | Список диалогов caller (`DialogListItem[]`) |
| Must | `POST` | `/api/v1/dialogs` | Get-or-create 1:1 по `{ "username" }` → один `DialogListItem` |
| Should | `GET` | `/api/v1/users/search` | Prefix-поиск username для UI «Новый чат» |

Правила регистрации роутов:

- `GET/POST /api/v1/dialogs` — **новые** handlers; не ломать существующие `…/dialogs/{id}/messages`.
- `GET /api/v1/users/search` — рядом с `POST /api/v1/users/register`, но **только** под auth middleware (register остаётся публичным).
- Пагинация list — out of scope; query params у `GET /dialogs` нет.
- Deep link / push по `dialog_id` — без изменений контракта (Sprint 4–6).

---

## 0.1) Коды ошибок

Формат тела без изменений (как Sprint 1–6):

```json
{
  "error": {
    "code": "user_not_found",
    "message": "user not found",
    "details": {}
  }
}
```

### Новые / уточнённые коды Sprint 7

| Код | HTTP | Endpoint | Когда |
|-----|------|----------|--------|
| `invalid_argument` | 400 | `POST /dialogs` | пустой/`trim`-пустой username; невалидный JSON body |
| `invalid_argument` | 400 | `GET /users/search` | `q` пустой или `<2` символов после trim/lower; невалидный `limit` |
| `cannot_dialog_with_self` | 400 | `POST /dialogs` | нормализованный username = текущий user |
| `user_not_found` | 404 | `POST /dialogs` | пользователь не найден **или** `status != active` (не различать — без enumeration статуса) |
| `unauthenticated` | 401 | все auth-required | нет/битый access JWT |

Правила:

- Для self-chat — **отдельный** код `cannot_dialog_with_self`, не общий `invalid_argument` (клиент показывает понятное сообщение).
- Для отсутствующего peer — **`user_not_found`**, не общий `not_found` (как `username_taken` vs `conflict` в Sprint 6).
- Inactive peer при create трактуется как `user_not_found` (404), не `user_inactive` (тот код остаётся для login своего аккаунта).
- `GET /dialogs` при успешной auth ошибок бизнес-логики не возвращает (пустой список → `200` + `"dialogs": []`).

Пример `cannot_dialog_with_self`:

```json
{
  "error": {
    "code": "cannot_dialog_with_self",
    "message": "cannot create dialog with yourself",
    "details": {}
  }
}
```

---

## 1) GET /dialogs

Список диалогов текущего пользователя.

**Auth:** required.

**Request:** без body. Query params: нет (пагинация — out of scope).

**Response 200:**

```json
{
  "dialogs": [
    {
      "dialog_id": "9484571a-5410-41d2-a56b-1f4e834ddd10",
      "peer": {
        "user_id": "…",
        "username": "bob"
      },
      "last_message": {
        "message_id": "…",
        "sender_id": "…",
        "body_preview": "привет",
        "created_at": "2026-08-06T12:00:00Z"
      },
      "unread_count": 2,
      "updated_at": "2026-08-06T12:00:00Z"
    }
  ]
}
```

**Утверждённый shape элемента** (`DialogListItem`) — единый для `GET /dialogs[]` и `POST /dialogs`:

| Поле | Тип | Обязательность |
|------|-----|----------------|
| `dialog_id` | UUID string | всегда |
| `peer` | `{ user_id, username }` | всегда |
| `last_message` | object \| `null` | всегда (ключ присутствует; `null`, если сообщений нет) |
| `last_message.message_id` | UUID string | если `last_message` не `null` |
| `last_message.sender_id` | UUID string | если `last_message` не `null` |
| `last_message.body_preview` | string | если `last_message` не `null` |
| `last_message.created_at` | RFC3339 UTC | если `last_message` не `null` |
| `unread_count` | int ≥ 0 | всегда |
| `updated_at` | RFC3339 UTC | всегда |

Правила:

- Только диалоги, где caller ∈ `{user_a_id, user_b_id}`.
- `peer` — второй участник (не caller).
- `last_message` — всегда в JSON как object или `null` (не omit ключа).
- `body_preview` — усечённый plaintext (≤120 рун, как `chat.BuildPreview`); без ciphertext E2EE.
- Soft-deleted сообщения не участвуют в preview.
- `unread_count` — число непрочитанных **входящих** в этом диалоге для caller.
- `updated_at` — `max(last_message.created_at, dialog.created_at)`.
- Сортировка списка: `updated_at` DESC.

**Errors:** `401 unauthenticated` (см. §0.1).

---

## 2) POST /dialogs

Создать или получить существующий 1:1 диалог по username собеседника.

**Auth:** required.

**Request:**

```json
{
  "username": "bob"
}
```

**Response 200** (создан или уже существовал — idempotent):

```json
{
  "dialog_id": "9484571a-5410-41d2-a56b-1f4e834ddd10",
  "peer": {
    "user_id": "…",
    "username": "bob"
  },
  "last_message": null,
  "unread_count": 0,
  "updated_at": "2026-08-06T12:05:00Z"
}
```

Правила:

- Username нормализуется (`trim` + lower), как в auth.
- `GetOrCreate` по канонической паре `(min(user_a), max(user_b))` — существующая реализация.
- Если диалог уже есть — вернуть его (не 409).

**Errors:** см. §0.1 (`invalid_argument`, `cannot_dialog_with_self`, `user_not_found`, `unauthenticated`).

---

## 3) GET /users/search (Should)

Поиск пользователей для UI «Новый чат».

**Auth:** required. Priority: Should (DoD спринта не блокирует; UI может работать с точным username без search).

**Query:**

- `q` — prefix username, min 2 символа после trim/lower.
- `limit` — optional, default 20, max 50.

**Response 200:**

```json
{
  "users": [
    { "user_id": "…", "username": "bob" },
    { "user_id": "…", "username": "bobby" }
  ]
}
```

Правила:

- Только `status = active`.
- Исключить текущего пользователя.
- Без email/phone/других PII.
- Пустой `q` или `<2` символов → `400 invalid_argument`.

**Errors:** см. §0.1 (`invalid_argument`, `unauthenticated`).

---

## 4) Без изменений (напоминание)

- `GET/POST /dialogs/{id}/messages`, WS, push payload с `dialog_id` — как Sprint 4–6.
- `GET /me/unread-count` — остаётся глобальным badge; список даёт per-dialog unread.
- Register/login — без breaking changes.

---

## 5) Клиентский flow

1. Login → `GET /dialogs` → render list.
2. Tap row → chat screen (`dialog_id` internal).
3. New chat → (optional search) → `POST /dialogs` → chat screen.
4. Push click → `dialog_id` from payload → open chat (peer username подтянуть из list cache или отдельного GET, если понадобится later).
