# Sprint 7 — детальный план (Список чатов / username)

Источник: итоги Sprint 6 (PWA + auth username/password), feedback UX: ручной ввод `dialog_id` UUID.

## 1) Цель спринта

Убрать demo-идентификацию чата через UUID диалога. Пользователь работает с **именами** (username):

- после логина видит список своих диалогов с username собеседника;
- может начать новый чат, указав username существующего пользователя;
- UUID `dialog_id` остаётся внутренним идентификатором API/WS/push, но **не вводится руками** в UI.

К концу Sprint 7 PWA на `https://beepru.ru` выглядит как обычный мессенджер: список чатов → переписка.

## 2) Входные условия (после Sprint 6)

- Login/register по username/password; device binding; Web Push; PWA на Home Screen.
- Диалоги: таблица `dialogs (id, user_a_id, user_b_id)`, `GetOrCreate` по паре пользователей.
- Клиент: экран Home с полем «Dialog ID» + кнопка «Открыть чат».
- Нет публичного `GET /dialogs` и `POST /dialogs` для клиента.
- Seed/prod: тестовые `demoa`/`demob` или `alice`/`bob` создаются вручную/скриптом.

## 3) Ключевые задачи

### A. API: список диалогов

**Проблема:** клиент не может узнать свои чаты без заранее известного UUID.

**Решение:**

1. `GET /api/v1/dialogs` (auth required):
   - возвращает диалоги, где текущий user — `user_a_id` или `user_b_id`;
   - для каждого: `dialog_id`, `peer` (`user_id`, `username`), опционально `last_message` (preview, created_at), `unread_count` (для peer→me), `updated_at` (по last message или `dialogs.created_at`);
   - сортировка: по активности (последнее сообщение) desc.
2. Репозиторий: `ListByUserID` + join `users` на peer + subquery/join messages/receipts.
3. Пагинация: out of scope (достаточно полного списка для 1:1 MVP); при росте — cursor later.

### B. API: создать / открыть чат по username

**Проблема:** нет способа «написать пользователю X» без SQL.

**Решение:**

1. `POST /api/v1/dialogs` body `{ "username": "bob" }`:
   - найти user по username (case-insensitive, как login);
   - нельзя создать чат с самим собой → `400`;
   - user не найден → `404 user_not_found`;
   - `GetOrCreate(dialogID=newUUID, me, peer)` — существующая пара возвращает тот же dialog;
   - ответ: тот же shape, что элемент списка (минимум `dialog_id` + `peer`).
2. Опционально (Should): `GET /api/v1/users/search?q=` — prefix search usernames (exclude self), limit 20; для автокомплита «Новый чат».

### C. Клиент PWA

1. Home → **список диалогов** (username, preview, unread badge на строке).
2. Кнопка «Новый чат» → ввод username (или search) → `POST /dialogs` → `showChat(dialog_id)`.
3. Убрать обязательный ручной ввод UUID (поле можно оставить скрытым в debug/dev только если нужно).
4. Заголовок чата: `peer.username`, не обрезанный dialog id.
5. После login / возврата на Home — `GET /dialogs` + обновление unread (существующий `/me/unread-count` или сумма по списку).
6. Push `notificationclick` / `open_dialog` — без изменений контракта; UI подтягивает peer при открытии.

### D. Тесты и качество

- Unit: service list/create (self, not found, get-or-create idempotent).
- Handler tests: 200/400/404/401.
- Integration: register A+B → create dialog by username → list contains peer → send/list messages.
- `task lint`, `task test`, `task test:integration`.
- Smoke prod: два пользователя, новый чат по username, список виден с обоих сторон.

## 4) Что не входит (out of scope)

- Групповые чаты, названия чатов, аватарки.
- PWA PIN unlock (→ Sprint 8); WebAuthn later.
- At-rest / E2EE шифрование сообщений (→ Sprint 9; E2EE вне scope).
- Typing indicators, presence, поиск по тексту сообщений.
- Изменение формата WS-событий сообщений.
- Per-message TTL override (known limitation Sprint 4) — только если останется явный запас времени.
- Нативный Capacitor App Store.

## 5) Риски

| Риск | Митигация |
|------|-----------|
| N+1 запросов в List | Один SQL с join/aggregates |
| Утечка username enumeration через search | Auth required; rate-limit optional; только prefix ≥2 символов |
| Старые клиенты с UUID-полем | Breaking UX на Home — ок для внутреннего продукта; deep link `?dialog=` сохранить |
| Пустой список у новых юзеров | CTA «Начните чат» + форма username |

## 6) Критерии готовности (DoD)

- [x] UUID диалога не нужно вводить для обычного сценария.
- [x] Список чатов показывает username собеседника.
- [x] Новый чат создаётся по username через API + UI.
- [x] Push / deep link по `dialog_id` по-прежнему открывает чат.
- [x] Lint/tests green; smoke на prod с двумя аккаунтами.

Примечание: закрыто 2026-08-07 — см. `docs/sprint-7-checklist.md`, `docs/known-limitations-sprint-7.md`.

## 7) Артефакты

- `docs/sprint-7-plan.md` (этот файл)
- `docs/sprint-7-checklist.md`
- `docs/api-sprint-7.md`
- По завершении: `docs/known-limitations-sprint-7.md`
