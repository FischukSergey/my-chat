# Sprint 8 Checklist

Источник: `docs/sprint-8-plan.md`.

**Цель спринта:** биометрический unlock PWA через WebAuthn / platform passkey (Face ID на iOS Home Screen).

**Предусловия:**
- PWA на prod с PIN unlock (`startUnlockNoBiometric`).
- Выбран Вариант A из plan §3.C (passkey = gate unlock, не полный passwordless).

**Статус:** PLANNED

---

## 1) Подготовка и решения

- [ ] Зафиксировать RP ID / origins для local и prod.
- [ ] Выбрать библиотеку (`go-webauthn/webauthn` или актуальный аналог).
- [ ] Утвердить: WebAuthn verify → local unlock (PIN bypass), JWT refresh остаётся как есть.
- [x] Подготовить `docs/api-sprint-8.md`.

Примечание: контракты зафиксированы в `docs/api-sprint-8.md` (register/login begin+finish, credentials list/delete).
- [ ] Проверить на целевом iPhone: `PublicKeyCredential` в установленном PWA.

---

## 2) Миграция и store

- [ ] Миграция `webauthn_credentials` (+ индексы по `user_id`).
- [ ] Repository: Create / ListByUser / GetByCredentialID / UpdateSignCount / Delete.
- [ ] Challenge store (Redis предпочтительно; TTL 2–5 мин).
- [ ] Тесты store.

---

## 3) WebAuthn service + HTTP

- [ ] Config: `webauthn.rp_id`, `rp_origins`, `rp_display_name` в yaml + env.
- [ ] `POST /webauthn/register/begin` + `finish`.
- [ ] `POST /webauthn/login/begin` + `finish` (assertion для unlock).
- [ ] `GET /webauthn/credentials`, `DELETE /webauthn/credentials/{id}`.
- [ ] Rate-limit begin/finish.
- [ ] Handler + service tests (mock authenticator data где возможно).

---

## 4) Клиент PWA — регистрация

- [ ] Feature detect platform authenticator.
- [ ] После PIN setup / из Settings: «Включить Face ID».
- [ ] `navigator.credentials.create` по options с сервера.
- [ ] Сохранить локальный флаг `webauthn_enabled` (Preferences).
- [ ] Обработка Cancel / NotAllowedError.

---

## 5) Клиент PWA — unlock

- [ ] `startUnlock`: если webauthn_enabled → begin/get/finish → при успехе unlock vault без PIN prompt.
- [ ] Fallback: PIN screen при ошибке/отмене.
- [ ] Не ломать Capacitor `startUnlock` + LocalAuthentication.
- [ ] UI Settings: список/удаление passkey.

---

## 6) Конфиг и prod

- [ ] Prod config RP ID = `beepru.ru`, origin = `https://beepru.ru`.
- [ ] Local: `localhost` origins для `task local:up`.
- [ ] Секреты не требуются сверх TLS; не коммитить challenge keys если появятся.
- [ ] Deploy + smoke iPhone PWA Face ID sheet.

---

## 7) Тесты и качество

- [ ] Unit/integration backend ceremonies.
- [ ] `task fmt`, `task lint`, `task test`, `task test:integration`.
- [ ] Manual: register passkey → kill PWA → Face ID unlock → Home.
- [ ] Manual: revoke credential → снова PIN.

---

## 8) Документация и закрытие

- [ ] Обновить `docs/chat-architecture-plan.md` (Sprint 8).
- [ ] `docs/known-limitations-sprint-8.md` (passwordless out of scope, Safari-tab limits, etc.).
- [ ] Чеклист → **DONE**.

---

## 9) DoD

- [ ] Passkey register + biometric unlock в PWA на iPhone.
- [ ] PIN fallback работает.
- [ ] Revoke credential работает.
- [ ] Native path не регрессировал.
- [ ] Lint/tests green.

---

**Sprint 8 — PLANNED**
