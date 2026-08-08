# Sprint 8 Checklist

Источник: `docs/sprint-8-plan.md`.

**Цель спринта:** локальный PIN-unlock в PWA; push title = username отправителя; фон чата (бирюза + watermark). WebAuthn — out of scope.

**Предусловия:**
- PWA на prod; cold start сегодня = `startUnlockNoBiometric` (auto refresh без PIN).
- Sprint 7 Home стабилен.
- Push сейчас: title = preview текста, body = «Новое сообщение».

**Статус:** PLANNED

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

---

## 5) Клиент — Resume / background lock

- [ ] На `visibilitychange` / hide: учитывать grace period.
- [ ] После истечения grace при возврате → Unlock PIN (скрыть chat/home content).
- [ ] Пока locked: не flush mark_read / не светить переписку.

---

## 6) Клиент — Settings и native

- [ ] Смена PIN (старый → новый ×2).
- [ ] Не ломать Capacitor `startUnlock` + LocalAuthentication.
- [ ] (Optional) Native тоже может использовать PIN как fallback — не Must.

---

## 7) Push — title = sender username (backend)

- [ ] `enqueueOutbox`: добавить `sender_username` в payload (lookup по `sender_id`).
- [ ] Worker / `push.Message`: прокинуть username.
- [ ] `webpush.buildPayload`: `title` = username; `body` = «Новое сообщение»; preview **не** в title.
- [ ] Unit-тесты webpush + outbox payload.
- [ ] Manual: offline push показывает username отправителя.

---

## 8) Клиент — фон чата

- [ ] CSS: чуть более тёмный фон области сообщений с бирюзовым оттенком (CSS variables).
- [ ] Водяной знак / паттерн (низкая opacity), не мешает читать пузыри.
- [ ] Smoke: свой/чужой bubble читаемы на новом фоне.

---

## 9) Тесты и качество

- [ ] `task fmt`, `task lint`, `task test` (+ `task test:integration` если enqueue/outbox затронут).
- [ ] Manual PWA: register → setup PIN → Home.
- [ ] Manual: kill PWA → PIN → Home.
- [ ] Manual: background > grace → PIN.
- [ ] Manual: wrong PIN ×5 → Login; logout → login → setup PIN снова.
- [ ] Manual: push title = username; chat background ok.

---

## 10) Документация и закрытие

- [ ] Обновить `docs/chat-architecture-plan.md` (Sprint 8 DONE + known limits).
- [ ] `docs/known-limitations-sprint-8.md` (UI-gate vs encrypt; WebAuthn deferred; no message text in push).
- [ ] Чеклист → **DONE**.

---

## 11) DoD

- [ ] PIN обязателен после login/register в PWA.
- [ ] Cold start и resume (после grace) требуют PIN.
- [ ] Смена PIN / logout поведение по §1.
- [ ] Push title = username отправителя.
- [ ] Фон чата: бирюза + watermark.
- [ ] Native Face ID path не регрессировал.
- [ ] Lint/tests green; smoke на `beepru.ru` PWA.

---

**Sprint 8 — PLANNED**
