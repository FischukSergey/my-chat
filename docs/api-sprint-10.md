# API Sprint 10 — удаление своего сообщения

Новый клиентский endpoint. Хранение и WS-событие — как в Sprint 4 (`deleted_at`, `message_deleted`).

Связанные: `docs/sprint-10-plan.md`, `docs/api-sprint-4.md` §события, `docs/api-sprint-7.md` (preview).

---

## 1) Решения

| Тема | Значение |
|------|----------|
| Метод | `DELETE /api/v1/messages/{id}` |
| Auth | `Authorization: Bearer <access>` |
| Кто | только `sender_id` текущего пользователя |
| Эффект | soft-delete у **обоих** участников |
| Успех | `204 No Content` |
| Уже удалено / нет id | `404` `not_found` |
| Чужое сообщение | `403` `forbidden` |
| Невалидный UUID | `400` `invalid_argument` |
| Без токена | `401` `unauthenticated` |

Тело запроса пустое. Новых REST-полей у сообщения нет.

---

## 2) HTTP

```
DELETE /api/v1/messages/{message_id}
Authorization: Bearer <access_token>
```

**204** — строка помечена `deleted_at`. Дальше `GET …/messages` её не вернёт.

**Ошибки** (формат как в Sprint 4):

| HTTP | code | Когда |
|------|------|--------|
| 400 | `invalid_argument` | `{id}` не UUID |
| 401 | `unauthenticated` | нет / битый access |
| 403 | `forbidden` | не автор |
| 404 | `not_found` | нет сообщения или уже `deleted_at` |

---

## 3) Побочные эффекты (не отдельный API)

1. Hub: `message_deleted` обоим участникам диалога (тот же payload, что TTL):

```json
{
  "event": "message_deleted",
  "data": {
    "message_id": "…",
    "dialog_id": "…"
  }
}
```

2. Получателю: `badge_updated` + silent `badge_sync` (unread мог уменьшиться).
3. Preview в `GET /dialogs` сам берёт последнее `deleted_at IS NULL` (Sprint 7).

Офлайн-клиент подтягивает состояние через `ListMessages` / resume reload (Sprint 8).

---

## 4) Клиент

- Вызывать только для исходящих пузырей.
- После 204 или входящего `message_deleted` — скрыть пузырь (как TTL).
- Confirm перед DELETE.

---

## 5) Вне scope

HARD DELETE, delete-for-me, edit, удаление чужих, Sprint 9 ciphertext.
