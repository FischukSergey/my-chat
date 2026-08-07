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

- [x] `chat.Service.ListDialogs(ctx, userID)`.
- [x] `chat.Service.CreateDialogByUsername(ctx, userID, username)` → FindByUsername + GetOrCreate.
- [x] Handler `GET /api/v1/dialogs`.
- [x] Handler `POST /api/v1/dialogs` `{ "username" }`.
- [x] Зарегистрировать роуты в `mainservice` (auth middleware).
- [x] Handler tests: 200, 401, 400 self, 404 missing user, idempotent create.

Примечание: `ListDialogs` / `CreateDialogByUsername` в `internal/services/chat/dialogs.go`; handlers в `handlers/chat/dialogs.go`; роуты auth-group в `mainservice`. Коды: `invalid_argument` / `cannot_dialog_with_self` / `user_not_found` / `unauthenticated`. Тесты service+handler; `task lint` / `task test` / `task test:integration` green.

---

## 4) (Should) Поиск пользователей

- [x] `GET /api/v1/users/search?q=&limit=` — prefix по username, exclude self, только `active`.
- [x] Валидация: `q` минимум 2 символа.
- [x] Тесты + подключение в UI автокомплита (если успеваем).

Примечание: store `SearchByUsernamePrefix` (`starts_with`); service `Search` (q≥2 рун, limit default 20/max 50); handler auth-route. Клиент: `searchUsers()` в `mobile/src/api.ts` (UI автокомплит — в §5–6). Тесты unit+integration; lint/tests green.

---

## 5) Клиент PWA — список чатов

- [x] `api.ts`: `listDialogs()`, `createDialog(username)`, опционально `searchUsers(q)`.
- [x] Home: заменить обязательный UUID-input на список диалогов.
- [x] Строка списка: username, preview, unread (если >0).
- [x] Тап → `showChat(dialog_id)`; заголовок чата = peer username.
- [x] Пустой список: текст + кнопка «Новый чат».
- [x] После `loadHome` — refresh списка (и sync app badge как в Sprint 6).

Примечание: Home = list + unread badge sync; UUID-input убран; заголовок чата = peer.username.

---

## 6) Клиент PWA — новый чат

- [x] UI «Новый чат»: поле username (+ опционально результаты search).
- [x] `POST /dialogs` → открыть чат; обработка 404/400 с понятными сообщениями.
- [x] Сохранить deep link `/?dialog=` и SW `open_dialog` без ломки.

Примечание: экран `new-chat` + debounce `searchUsers`; `ApiError` для user_not_found / cannot_dialog_with_self / invalid_argument; `/?dialog=` и SW `open_dialog` → pending → `showChat` после `loadHome`.

---

## 7) Тесты и качество

- [x] Unit service/handler для list + create.
- [x] Integration: register/login A+B → create by username → list → send message.
- [x] `task fmt`, `task lint`, `task test`, `task test:integration`.
- [x] Smoke prod: два аккаунта, чат по username, список с обеих сторон.

Примечание: unit — `services/chat/service_test.go` + `handlers/chat/handler_test.go`; E2E integration — `TestIntegration_RegisterCreateListSend`. `task fmt` / `lint` / `test` / `test:integration` green. Smoke **local** (`task local:up` + seed alice/bob): login → POST /dialogs → list с обеих сторон → send → bob видит preview+unread. Повтор на beepru.ru — после deploy.

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
