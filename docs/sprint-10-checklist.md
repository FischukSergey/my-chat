# Sprint 10 Checklist

Источник: `docs/sprint-10-plan.md`.

**Цель спринта:** автор удаляет своё сообщение у обоих участников (soft-delete + WS `message_deleted`).

**Предусловия:**
- Sprint 4 TTL/soft-delete и Sprint 7 список чатов работают.
- Sprint 9 (шифрование) отложен, не смешивать DoD.

**Статус:** DONE  
**Порядок:** после Sprint 8; Sprint 9 не открывать.

---

## 1) Подготовка и контракты

- [x] Утвердить: delete-for-everyone, только автор, без окна времени, soft-delete.
- [x] Утвердить `DELETE /api/v1/messages/{id}` → 204; ошибки `not_found` / `forbidden`.
- [x] Подготовить `docs/api-sprint-10.md`.

Примечание: решения и контракт — `docs/sprint-10-plan.md` §3.A, `docs/api-sprint-10.md`.

---

## 2) Store

- [x] `MessageRepository.SoftDelete` (`deleted_at`, идемпотентный WHERE).
- [x] `GetByID` → `ErrMessageNotFound` при отсутствии / уже deleted.
- [x] Integration: delete скрывает строку из `ListByDialog`.

Примечание: `SoftDelete` + `ErrMessageNotFound`; тест `TestMessageRepository_SoftDelete_HidesFromListAndGetByID`. `task test:integration` green.

---

## 3) Service

- [x] `DeleteOwnMessage(ctx, messageID, userID)`.
- [x] Не автор → `ErrForbiddenMessageDelete`.
- [x] Нет сообщения → `ErrMessageNotFound`.
- [x] WS `message_deleted` обоим; badge получателю.
- [x] Unit-тесты: author / foreign / missing.

Примечание: Hub `EventMessageDeleted` обоим + `badge_updated` / `badge_sync` peer. Тесты в `service_test.go`.

---

## 4) HTTP

- [x] `DELETE /api/v1/messages/{id}` в `main-service`.
- [x] 204 / 404 / 403 / 401.
- [x] Handler-тест на 204 и 403.

Примечание: также 404-тест. Роут рядом с `POST …/messages/{id}/read`.

---

## 5) Клиент PWA

- [x] `deleteMessage` в `api.ts`.
- [x] «Удалить» на исходящем пузыре + confirm.
- [x] Скрыть пузырь после успеха и по `message_deleted`.
- [x] Не показывать действие на входящих.

Примечание: кнопка только у `.bubble.outgoing`; confirm «Удалить это сообщение у всех?»; `message_deleted` всегда делает `scheduleHomeRefresh`.

---

## 6) Тесты и качество

- [x] `task lint`, `task test` (integration — если store/service затронуты).
- [x] TTL expire по-прежнему шлёт `message_deleted`.
- [x] Manual: Alice удаляет → у Bob пузырь пропадает; Bob не удаляет чужое.

Примечание: `task lint` / `task test` / `task test:integration` green; expirer-тесты без регрессии. Manual smoke подтверждён пользователем 2026-09-17.

---

## 7) Документация и закрытие

- [x] `docs/chat-architecture-plan.md` — Sprint 10.
- [x] `docs/known-limitations-sprint-10.md` при закрытии.
- [x] Чеклист → **DONE**.

---

## 8) DoD

- [x] Только автор, удаление у обоих.
- [x] WS + история согласованы.
- [x] Lint/tests green.

Примечание: авто green; smoke подтверждён 2026-09-17.

---

**Sprint 10 — DONE**
