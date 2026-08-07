# Sprint 7 Checklist

Источник: `docs/sprint-7-plan.md`.

**Цель спринта:** список диалогов и старт чата по username — без ручного ввода UUID.

**Предусловия:**
- Sprint 6 закрыт или функционально готов (PWA, login username/password, chat API).
- На prod/local есть минимум два тестовых пользователя.

**Статус:** PLANNED

---

## 1) Подготовка и контракты

- [x] Утвердить response shape элемента диалога (`dialog_id`, `peer`, `last_message?`, `unread_count`, `updated_at`).

Примечание: канон — `DialogListItem` в `docs/api-sprint-7.md` §1; `last_message` всегда object|null (не omit); `body_preview` ≤120 рун; тот же shape для POST.

- [x] Утвердить `GET /api/v1/dialogs`, `POST /api/v1/dialogs`, опционально `GET /api/v1/users/search`.

Примечание: канон — `docs/api-sprint-7.md` §0; Must: `GET/POST /api/v1/dialogs` (auth); Should: `GET /api/v1/users/search` (auth); не ломать `…/dialogs/{id}/messages`.

- [x] Зафиксировать коды ошибок: `user_not_found`, `cannot_dialog_with_self`, `invalid_argument`.

Примечание: канон — `docs/api-sprint-7.md` §0.1; self → `cannot_dialog_with_self` (400); missing/inactive peer → `user_not_found` (404); валидация → `invalid_argument` (400); auth → `unauthenticated` (401).

- [x] Подготовить / актуализировать `docs/api-sprint-7.md`.

Примечание: контракты зафиксированы в `docs/api-sprint-7.md` (list/create dialogs, optional users/search, error codes).

---

## 2) Store / репозиторий диалогов

- [x] `DialogRepository.ListByUserID(ctx, userID)` — диалоги пользователя + peer ids.
- [x] Join/загрузка `username` peer из `users`.
- [x] Last message preview + timestamp (последнее не soft-deleted сообщение).
- [x] Unread count per dialog для текущего user (через receipts / messages).
- [x] При необходимости индексы (messages by dialog_id + created_at) — миграция только если explain показывает проблему.
- [x] Unit/integration-тесты репозитория.

Примечание: один SQL в `ListByUserID` (peer JOIN + LATERAL last message + per-dialog unread, `deleted_at IS NULL`); модель `DialogListItem`; индекс `messages_dialog_created_at_idx` уже есть (003) — миграция не нужна. Тесты: `dialog_repository_integration_test.go`; `task lint` / `task test` / `task test:integration` green.

---

## 3) Service + HTTP: список и создание

- [ ] `chat.Service.ListDialogs(ctx, userID)`.
- [ ] `chat.Service.CreateDialogByUsername(ctx, userID, username)` → FindByUsername + GetOrCreate.
- [ ] Handler `GET /api/v1/dialogs`.
- [ ] Handler `POST /api/v1/dialogs` `{ "username" }`.
- [ ] Зарегистрировать роуты в `mainservice` (auth middleware).
- [ ] Handler tests: 200, 401, 400 self, 404 missing user, idempotent create.

---

## 4) (Should) Поиск пользователей

- [ ] `GET /api/v1/users/search?q=&limit=` — prefix по username, exclude self, только `active`.
- [ ] Валидация: `q` минимум 2 символа.
- [ ] Тесты + подключение в UI автокомплита (если успеваем).

---

## 5) Клиент PWA — список чатов

- [ ] `api.ts`: `listDialogs()`, `createDialog(username)`, опционально `searchUsers(q)`.
- [ ] Home: заменить обязательный UUID-input на список диалогов.
- [ ] Строка списка: username, preview, unread (если >0).
- [ ] Тап → `showChat(dialog_id)`; заголовок чата = peer username.
- [ ] Пустой список: текст + кнопка «Новый чат».
- [ ] После `loadHome` — refresh списка (и sync app badge как в Sprint 6).

---

## 6) Клиент PWA — новый чат

- [ ] UI «Новый чат»: поле username (+ опционально результаты search).
- [ ] `POST /dialogs` → открыть чат; обработка 404/400 с понятными сообщениями.
- [ ] Сохранить deep link `/?dialog=` и SW `open_dialog` без ломки.

---

## 7) Тесты и качество

- [ ] Unit service/handler для list + create.
- [ ] Integration: register/login A+B → create by username → list → send message.
- [ ] `task fmt`, `task lint`, `task test`, `task test:integration`.
- [ ] Smoke prod: два аккаунта, чат по username, список с обеих сторон.

---

## 8) Документация и закрытие

- [ ] Обновить `docs/chat-architecture-plan.md` (статус Sprint 7).
- [ ] `docs/known-limitations-sprint-7.md`.
- [ ] Чеклист footer → **DONE**.

---

## 9) Критерии готовности (DoD)

- [ ] Обычный пользователь не вводит UUID диалога.
- [ ] Список чатов с username собеседника.
- [ ] Новый чат по username работает end-to-end.
- [ ] Push/deep-link по `dialog_id` открывает нужный чат.
- [ ] Lint/tests green.

---

**Sprint 7 — PLANNED**
