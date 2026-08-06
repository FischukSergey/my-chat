# Sprint 8 — детальный план (PWA unlock / WebAuthn)

Источник: ограничение Sprint 3/6 — Face ID в Capacitor native работает; в Safari / PWA на Home Screen `LocalAuthentication` недоступен, используется только PIN unlock.

## 1) Цель спринта

Дать пользователю **биометрический / системный unlock** в установленном PWA (iOS Safari Home Screen, Android Chrome), без App Store и без Capacitor Face ID:

- регистрация **WebAuthn / passkey** (platform authenticator: Face ID, Touch ID, Android biometrics);
- после успешного login + PIN setup — опциональная привязка passkey;
- cold start / resume: unlock через WebAuthn вместо (или в дополнение к) PIN;
- сервер хранит credentials и проверяет assertion.

К концу Sprint 8: на iPhone PWA можно разблокировать Face ID через системный WebAuthn dialog.

## 2) Входные условия

- Sprint 6: PWA, secure storage PIN, `startUnlockNoBiometric` для non-native.
- Sprint 7 (желательно): список чатов — UX-база; Sprint 8 можно параллелить по backend, но UI unlock — после стабильного Home.
- Auth: access/refresh JWT, device_id, sessions.
- Нет таблицы WebAuthn credentials; нет `/webauthn/*` endpoints.

## 3) Ключевые задачи

### A. Модель данных

1. Таблица `webauthn_credentials` (миграция):
   - `id` (credential id, bytea/base64url PK or unique);
   - `user_id` UUID FK;
   - `device_id` TEXT nullable (привязка к устройству, если есть);
   - `public_key` BYTEA;
   - `attestation_type` / `transport` / `aaguid` — по необходимости библиотеки;
   - `sign_count` BIGINT;
   - `name` TEXT (user label, optional);
   - `created_at`, `last_used_at`;
   - `backup_eligible` / `backup_state` — если библиотека отдаёт.
2. Challenge store: Redis или in-memory с TTL (для registration/login ceremony) — ключ `webauthn:challenge:{userID|session}`.
3. Политика: один или несколько credentials на user; revoke по id.

### B. Backend WebAuthn

Библиотека (рекомендация): `github.com/go-webauthn/webauthn` (актуальная на момент спринта).

1. Config: `RP_ID` (`beepru.ru`), `RP_ORIGINS` (`https://beepru.ru`), `RP_DISPLAY_NAME`.
2. Endpoints (auth Bearer required, кроме recovery — см. ниже):
   - `POST /api/v1/webauthn/register/begin` → options + challenge;
   - `POST /api/v1/webauthn/register/finish` → verify attestation, persist credential;
   - `POST /api/v1/webauthn/login/begin` → options (discoverable или allowCredentials по user);
   - `POST /api/v1/webauthn/login/finish` → verify assertion → выдать **короткий unlock token** или подтвердить local session (см. D).
3. `DELETE /api/v1/webauthn/credentials/{id}` — снять passkey.
4. `GET /api/v1/webauthn/credentials` — список (id, name, created_at) для UI настроек.

### C. Семантика unlock vs login

Разделить понятия:

| Сценарий | Механизм |
|----------|----------|
| Первый вход / смена аккаунта | username + password → JWT (как сейчас) |
| Разблокировка локального vault (PIN) | локально, без сети (как Sprint 3) |
| Разблокировка PWA с Face ID | WebAuthn assertion → сервер подтверждает → клиент открывает vault / пропускает PIN |

Варианты интеграции (выбрать в §1 чеклиста):

**Вариант A (рекомендуемый для MVP):**  
После password login пользователь регистрирует passkey. При cold start:  
1) если есть local session hint + passkey → `login/begin`+`finish` с userHandle;  
2) успех → refresh/access как «доказанный владелец» **или** только local unlock flag без нового JWT (если refresh ещё валиден в secure storage).  
Проще для MVP: WebAuthn = **gate перед чтением PIN/secure storage**, сервер verify, клиент затем `unlockWithPin` автоматически или bypass PIN prompt.

**Вариант B:**  
WebAuthn passwordless login (usernameless discoverable credentials) — шире scope, выше риск UX/iOS quirks → **Should/later**, не Must.

Must Sprint 8: **Вариант A** — passkey как biometric unlock поверх существующей session/PIN модели.

### D. Клиент PWA

1. Feature detect: `window.PublicKeyCredential`, `isUserVerifyingPlatformAuthenticatorAvailable()`.
2. После первого успешного unlock/PIN setup — баннер «Включить Face ID / биометрию» → register ceremony.
3. На `startUnlock`: если credential зарегистрирован локально (флаг в Preferences) → `navigator.credentials.get` flow вместо PIN (fallback на PIN при Cancel/error).
4. Capacitor native: оставить существующий Face ID path; не ломать.
5. Settings: «Удалить passkey» / «Добавить passkey».
6. iOS: только в установленном PWA / подходящем контексте; документировать ограничение Safari-вкладки.

### E. Безопасность

- RP ID строго `beepru.ru` (не localhost в prod config).
- Local: отдельные origins для dev (`localhost`).
- Challenges одноразовые, TTL ≤ 2–5 мин.
- Не логировать public keys / assertions целиком.
- CSRF: same-site + Bearer; WebAuthn origin check в библиотеке.
- Rate-limit begin/finish.

### F. Тесты

- Unit: finish registration/login happy path с mock.
- Integration: register user → webauthn register → assert → unlock path.
- Ручной smoke iPhone PWA: Face ID dialog → Home.

## 4) Что не входит (out of scope)

- Полный passwordless (без username/password вообще) как единственный способ входа.
- Sync passkeys через iCloud как продуктовая фича (система делает сама — мы только platform authenticator).
- Android-specific Credential Manager UI beyond WebAuthn.
- Замена Capacitor LocalAuthentication в native build.
- Sprint 7 UI (список чатов) — чужой scope; только точка входа unlock.

## 5) Риски

| Риск | Митигация |
|------|-----------|
| iOS Safari WebAuthn quirks в PWA | Smoke early; fallback PIN обязателен |
| Путаница unlock vs re-login | Документировать flow; не звать password на каждый resume |
| RP ID mismatch staging/prod | Config per env; checklist |
| Discoverable credentials complexity | MVP: non-discoverable + allowCredentials из server list |

## 6) DoD

- [ ] Пользователь может зарегистрировать platform passkey в PWA.
- [ ] Cold start / resume: Face ID (системный sheet) разблокирует приложение без ввода PIN (PIN — fallback).
- [ ] Можно отозвать credential.
- [ ] Native Capacitor path не сломан.
- [ ] Lint/tests green; smoke на `beepru.ru` PWA.

## 7) Артефакты

- `docs/sprint-8-plan.md` (этот файл)
- `docs/sprint-8-checklist.md`
- `docs/api-sprint-8.md`
- По завершении: `docs/known-limitations-sprint-8.md`
