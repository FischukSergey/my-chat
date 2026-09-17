# Sprint 10 — детальный план (удаление своего сообщения)

Источник: Sprint 9 (at-rest encryption) отложен. Нужно дать автору возможность убрать уже отправленное сообщение у обоих участников.

Существующее удаление — только TTL (`message-expirer` → `deleted_at` → WS `message_deleted`). Ручного unsend нет.

## 1) Цель спринта

Автор может удалить **своё** сообщение. Удаление **для всех** (оба участника диалога), через уже принятый soft-delete.

К концу спринта: в PWA у исходящего пузыря есть действие «Удалить»; после успеха пузырь исчезает у отправителя и у получателя (WS или reload истории).

## 2) Входные условия

- Sprint 4: `deleted_at`, `ListMessages` без soft-deleted, WS `message_deleted`.
- Sprint 7: preview списка не берёт `deleted_at IS NOT NULL`.
- Sprint 8: PWA чат + resume reload истории.
- Sprint 9 (шифрование) — **не начинать**, не смешивать DoD.

## 3) Ключевые задачи

### A. Решения (зафиксировать в чеклисте §1)

| Тема | Решение (Must) |
|------|----------------|
| Кому удаляется | **Обоим** (как TTL), не «только у меня» |
| Кто может | Только `sender_id == current user` |
| Окно времени | **Нет** (пока сообщение ещё не soft-deleted) |
| Хранение | Soft-delete `deleted_at = now()`, не HARD DELETE |
| WS | Тот же `message_deleted` (`message_id`, `dialog_id`) |
| Повторный DELETE | 404 `not_found` (строка уже не видна) |
| Чужое / не участник | 403 `forbidden` |
| Unread / preview | Пересчёт badge получателя; preview сам отфильтрует deleted |

### B. API

`DELETE /api/v1/messages/{id}` (auth). Ответ **204**. Контракт — `docs/api-sprint-10.md`.

### C. Сервер

1. Store: `SoftDelete(ctx, messageID) (bool, error)` — `UPDATE … SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`.
2. Service: `GetByID` → проверка автора → `SoftDelete` → `message_deleted` обоим через Hub → `badge_updated` / badge_sync получателю.
3. Route в `main-service` рядом с `POST …/messages/{id}/read`.

Миграция не нужна.

### D. Клиент PWA

1. `deleteMessage(id)` в `api.ts`.
2. На исходящем пузыре действие «Удалить» + confirm.
3. Успех / входящий `message_deleted` — скрыть пузырь (как TTL).
4. Список чатов: уже есть refresh при `message_deleted` вне открытого диалога; при удалении last message — `GET /dialogs` после события.

## 4) Что не входит

- Удаление чужого сообщения / модерация.
- «Удалить только у себя».
- HARD DELETE / `pg_cron` purge (backlog; не путать с этим спринтом).
- Редактирование (`edit`) сообщения.
- Sprint 9 encryption.
- Media/вложения.

## 5) Риски

| Риск | Митигация |
|------|-----------|
| Получатель офлайн в момент DELETE | `ListMessages` не вернёт строку; resume reload |
| Preview на Home устарел | `message_deleted` → `scheduleHomeRefresh` (уже есть) |
| Unread не упал | `badge_updated` получателю после delete |

## 6) DoD

- [x] Автор удаляет своё сообщение; получатель не может чужое.
- [x] Оба видят исчезновение (WS или повторный GET).
- [x] TTL/expirer не сломан.
- [x] Lint/tests green; smoke в PWA.

## 7) Артефакты

- `docs/sprint-10-plan.md` (этот файл)
- `docs/sprint-10-checklist.md`
- `docs/api-sprint-10.md`
- По завершении: `docs/known-limitations-sprint-10.md`
