# Sprint 7 Checklist

Источник: `docs/sprint-7-plan.md`.

**Цель спринта:** список диалогов и старт чата по username — без ручного ввода UUID.

**Предусловия:**
- Sprint 6 закрыт или функционально готов (PWA, login username/password, chat API).
- На prod/local есть минимум два тестовых пользователя.

**Статус:** PLANNED

---

## 1) Подготовка и контракты

- [ ] Утвердить response shape элемента диалога (`dialog_id`, `peer`, `last_message?`, `unread_count`, `updated_at`).
- [ ] Утвердить `GET /api/v1/dialogs`, `POST /api/v1/dialogs`, опционально `GET /api/v1/users/search`.
- [ ] Зафиксировать коды ошибок: `user_not_found`, `cannot_dialog_with_self`, `invalid_argument`.
- [x] Подготовить / актуализировать `docs/api-sprint-7.md`.

Примечание: контракты зафиксированы в `docs/api-sprint-7.md` (list/create dialogs, optional users/search).

---

## 2) Store / репозиторий диалогов

- [ ] `DialogRepository.ListByUserID(ctx, userID)` — диалоги пользователя + peer ids.
- [ ] Join/загрузка `username` peer из `users`.
- [ ] Last message preview + timestamp (последнее не soft-deleted сообщение).
- [ ] Unread count per dialog для текущего user (через receipts / messages).
- [ ] При необходимости индексы (messages by dialog_id + created_at) — миграция только если explain показывает проблему.
- [ ] Unit/integration-тесты репозитория.

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
