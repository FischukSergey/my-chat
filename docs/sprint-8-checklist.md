# Sprint 8 Checklist

Источник: `docs/sprint-8-plan.md`.

**Цель спринта:** локальный PIN-unlock в PWA; push title = username отправителя; фон чата (бирюза + watermark). WebAuthn — out of scope.

**Предусловия:**
- PWA на prod; cold start сегодня = `startUnlockNoBiometric` (auto refresh без PIN).
- Sprint 7 Home стабилен.
- Push сейчас: title = preview текста, body = «Новое сообщение».

**Статус:** DONE

---

## 1) Подготовка и решения

- [x] Утвердить длину PIN: **4 цифры**.
- [x] Утвердить grace period resume: **60s** (константа в клиенте).
- [x] Утвердить lockout: **5 неверных попыток → clear tokens → Login**.
- [x] Утвердить logout: **сбрасывать PIN verifier** вместе с токенами.
- [x] Утвердить уровень защиты: Must = UI gate; Should = encrypt refresh (KDF+AES-GCM) в том же спринте если успеваем.
- [x] Подготовить `docs/api-sprint-8.md` под PIN + push title contract.

Примечание: решения §1 зафиксированы 2026-08-08: PIN length=4; grace=60s; lockout=5→clear tokens+PIN→Login; logout wipe PIN; Must=UI gate, Should=encrypt refresh. `docs/api-sprint-8.md` обновлён (PIN + push title).
- [x] Утвердить визуал чата: бирюзовый tint + watermark (opacity/паттерн на глаз при реализации).

---

## 2) Клиент — PIN storage / crypto

- [x] Модуль PIN: setup / verify / change / clear (Preferences keys из api doc).
- [x] Хранить только salt + hash (не plaintext PIN).
- [x] (Should) Encrypt refresh token ключом из PIN; без PIN ciphertext нечитаем.
- [x] Unit-тесты hash/verify (и encrypt round-trip если Should взят).

Примечание: `mobile/src/pin.ts` — SHA-256(salt||PIN) verifier; PBKDF2+AES-GCM refresh (`enc:v1:`); константы PIN_LENGTH=4, grace/attempts. Тесты: `cd mobile && npm test` (vitest, 14 passed). Wire в UI/auth — §3+.

---

## 3) Клиент — Setup PIN

- [x] Экран setup после успешного register/login, если PIN не задан.
- [x] Двойной ввод + валидация длины/цифр.
- [x] Не пускать на Home до успешного setup.

Примечание: экран `#setup-pin`; после login и register→auto-login → `enterAppAfterAuth`; `loadHome` gated через `isPinSet`; при setup encrypt refresh (`enc:v1:`); logout wipe PIN.

---

## 4) Клиент — Unlock PIN

- [x] Заменить auto-`startUnlockNoBiometric` на экран ввода PIN при `hasRefresh && pin_set`.
- [x] Успех → silent refresh → Home (как сейчас после gate).
- [x] Нет PIN при наличии refresh → Setup PIN (миграция существующих сессий).
- [x] «Выйти из аккаунта» с unlock-экрана работает.
- [x] Lockout по §1.

Примечание: `startUnlockWithPin` вместо silent refresh; decrypt `enc:v1:` → `apiRefresh` → re-encrypt; 5 fails → clear tokens+PIN → Login; native Face ID сохранён (`startUnlock`), при ciphertext после биометрии — PIN.
Post-sprint UX (до Sprint 9): кнопка «Разблокировать» убрана; unlock при вводе 4-й цифры (`bindPinInput` + `handleUnlockPin`).

---

## 5) Клиент — Resume / background lock

- [x] На `visibilitychange` / hide: учитывать grace period.
- [x] После истечения grace при возврате → Unlock PIN (скрыть chat/home content).
- [x] Пока locked: не flush mark_read / не светить переписку.

Примечание: `PIN_LOCK_GRACE_MS=60s`; hide → `hiddenAt`; resume > grace → `appLocked` + unlock PIN (chat/home через `display:none`); успех resume = verify PIN → restore screen + `loadChatHistory` / `refreshHomeData`; `tryMarkRead`/`flushPendingMarkRead` no-op при `appLocked`.
Post-sprint: истёкшие пузыри скрываются локально по `expires_at`; reload ленты на resume и WS reconnect (пропущенный `message_deleted` в фоне PWA).

---

## 6) Клиент — Settings и native

- [x] Смена PIN (старый → новый ×2).
- [x] Не ломать Capacitor `startUnlock` + LocalAuthentication.
- [x] (Optional) Native тоже может использовать PIN как fallback — не Must.

Примечание: Home → «Сменить PIN» (`#change-pin`); `changePin` + re-encrypt refresh. Native: `startUnlock` (Face ID) без изменений Must; optional fallback PIN если refresh `enc:v1:` (из §4).

---

## 7) Push — title = sender username (backend)

- [x] `enqueueOutbox`: добавить `sender_username` в payload (lookup по `sender_id`).
- [x] Worker / `push.Message`: прокинуть username.
- [x] `webpush.buildPayload`: `title` = username; `body` = «Новое сообщение»; preview **не** в title.
- [x] Unit-тесты webpush + outbox payload.
- [x] Manual: offline push показывает username отправителя.

Примечание: `sender_username` в outbox (lookup `FindByID`, fallback `"user"`); `push.Message.SenderUsername` → `buildPayload` title; preview остаётся в payload, не в title. Unit + manual smoke (подтверждено пользователем 2026-08-08).

---

## 8) Клиент — фон чата

- [x] CSS: чуть более тёмный фон области сообщений с бирюзовым оттенком (CSS variables).
- [x] Водяной знак / паттерн (низкая opacity), не мешает читать пузыри.
- [x] Smoke: свой/чужой bubble читаемы на новом фоне.

Примечание: `#messages-list` — `--chat-bg` / `--chat-bg-deep` + SVG watermark; smoke подтверждён пользователем 2026-08-08.

---

## 9) Тесты и качество

- [x] `task fmt`, `task lint`, `task test` (+ `task test:integration` если enqueue/outbox затронут).
- [x] Manual PWA: register → setup PIN → Home.
- [x] Manual: kill PWA → PIN → Home.
- [x] Manual: background > grace → PIN.
- [x] Manual: wrong PIN ×5 → Login; logout → login → setup PIN снова.
- [x] Manual: push title = username; chat background ok.

Примечание: авто — `task fmt` / `lint` / `test` / `test:integration` green; `cd mobile && npm test` — 14 passed. Manual PWA/smoke на устройстве — подтверждено пользователем 2026-08-08.

---

## 10) Документация и закрытие

- [x] Обновить `docs/chat-architecture-plan.md` (Sprint 8 DONE + known limits).
- [x] `docs/known-limitations-sprint-8.md` (UI-gate vs encrypt; WebAuthn deferred; no message text in push).
- [x] Чеклист → **DONE**.

---

## 11) DoD

- [x] PIN обязателен после login/register в PWA.
- [x] Cold start и resume (после grace) требуют PIN.
- [x] Смена PIN / logout поведение по §1.
- [x] Push title = username отправителя.
- [x] Фон чата: бирюза + watermark.
- [x] Native Face ID path не регрессировал.
- [x] Lint/tests green; smoke на `beepru.ru` PWA.

---

**Sprint 8 — DONE**
