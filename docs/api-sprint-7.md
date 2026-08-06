# API Sprint 7 — Список диалогов / username

Базовый path: `/api/v1`. Auth: Bearer access JWT (как Sprint 6). Username — case-insensitive (нормализация `lower`, как login/register).

Связанные: `docs/api-sprint-6.md` (auth, messages, unread), `docs/sprint-7-plan.md`.

---

## 1) GET /dialogs

Список диалогов текущего пользователя.

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

Правила:

- Только диалоги, где caller ∈ `{user_a_id, user_b_id}`.
- `peer` — второй участник.
- `last_message` — `null` / omit, если сообщений нет.
- `body_preview` — усечённый текст (например ≤120 символов); без ciphertext E2EE (plaintext MVP).
- Soft-deleted сообщения не участвуют в preview.
- `unread_count` — число непрочитанных входящих в этом диалоге для caller.
- `updated_at` — `max(last_message.created_at, dialog.created_at)`.
- Сортировка: `updated_at` DESC.

**Errors:** `401` unauthorized.

---

## 2) POST /dialogs

Создать или получить существующий 1:1 диалог по username собеседника.

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

**Errors:**

| HTTP | code | когда |
|------|------|--------|
| 400 | `invalid_argument` | пустой username |
| 400 | `cannot_dialog_with_self` | username = текущий user |
| 404 | `user_not_found` | пользователь не найден / не active |
| 401 | — | нет/битый access |

---

## 3) GET /users/search (Should)

Поиск пользователей для UI «Новый чат».

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

**Errors:** `400`, `401`.

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
