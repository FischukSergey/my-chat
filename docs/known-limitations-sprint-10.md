# Known Limitations — Sprint 10

Дата: 2026-09-17  
Статус: зафиксировано по итогам спринта (manual smoke подтверждён)

---

## 1. Удаление только у обоих, только автор

**Что сделано:** `DELETE /api/v1/messages/{id}` — soft-delete `deleted_at`, WS `message_deleted` обоим, кнопка «Удалить» только на исходящих.

**Остаётся:** нет «удалить только у себя», нет модерации чужих, нет окна «unsend 2 минуты». Повторный DELETE → `404 not_found`.

---

## 2. Нет HARD DELETE / purge

Строка остаётся в БД с `deleted_at`. `message-expirer` и unsend делят одну семантику. Физическая очистка / `pg_cron` — backlog (не путать со Sprint 9).

---

## 3. Офлайн в момент DELETE

Hub шлёт `message_deleted` только онлайн-клиентам (в отличие от TTL-expirer, который пишет `ws_event_outbox`). Офлайн подтягивает состояние через `GET` истории / resume reload. Preview на Home обновляется по событию или при возврате на список.

---

## 4. Нет edit и вложений

Редактирование текста и media/фото — вне scope. Sprint 9 (at-rest encryption) по-прежнему отложен.

---

## 5. Вне scope (напоминание)

- Delete-for-me.
- HARD DELETE / purge soft-deleted.
- Edit сообщения.
- Media/attachments.
- Sprint 9 AES-GCM.
