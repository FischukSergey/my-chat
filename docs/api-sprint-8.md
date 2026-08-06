# API Sprint 8 — WebAuthn / PWA unlock

Базовый path: `/api/v1`. Auth: Bearer access JWT для register/list/delete.  
`login/begin`+`finish` — для unlock assertion; доступ по access token **или** по user id + device hint (уточнить при реализации: MVP — требуется валидный refresh в secure storage + access или отдельный unlock session cookie — зафиксировать в коде и здесь).

Рекомендуемый MVP: все `/webauthn/*` за Bearer access (пользователь уже залогинен; WebAuthn только разблокирует локальный UI). Если access истёк — сначала silent refresh, затем WebAuthn unlock.

RP: `rp_id`, `origins` из конфига (prod: `beepru.ru` / `https://beepru.ru`).

Связанные: `docs/sprint-8-plan.md`, `docs/api-sprint-6.md` (auth).

---

## 1) POST /webauthn/register/begin

Начать регистрацию platform credential.

**Auth:** required.

**Request:** `{}` или `{ "name": "iPhone" }` (label).

**Response 200:** PublicKeyCredentialCreationOptions (JSON, как отдаёт библиотека; client передаёт в `navigator.credentials.create`).

Сервер сохраняет challenge в TTL store.

**Errors:** `401`, `429`, `400` если platform уже excess limit (если введём лимит).

---

## 2) POST /webauthn/register/finish

Завершить регистрацию.

**Auth:** required.

**Request:** credential attestation response (JSON от `credentials.create`).

**Response 200:**

```json
{
  "credential_id": "base64url…",
  "name": "iPhone",
  "created_at": "2026-08-06T12:00:00Z"
}
```

**Errors:** `400` invalid attestation / bad challenge, `401`, `409` duplicate credential id.

---

## 3) POST /webauthn/login/begin

Начать assertion (unlock).

**Auth:** required (MVP).

**Request:** `{}`

**Response 200:** PublicKeyCredentialRequestOptions (`allowCredentials` из credentials пользователя).

**Errors:** `401`, `404` если нет credentials (`no_credentials`), `429`.

---

## 4) POST /webauthn/login/finish

Проверить assertion.

**Auth:** required (MVP).

**Request:** credential assertion response.

**Response 200:**

```json
{
  "ok": true,
  "unlocked_at": "2026-08-06T12:01:00Z"
}
```

Клиент при `ok` открывает vault / пропускает PIN. JWT не обязательно ротировать (session уже есть).

**Errors:** `400` verification failed, `401`, `404`.

---

## 5) GET /webauthn/credentials

**Auth:** required.

**Response 200:**

```json
{
  "credentials": [
    {
      "credential_id": "…",
      "name": "iPhone",
      "created_at": "…",
      "last_used_at": "…"
    }
  ]
}
```

Без public key в ответе.

---

## 6) DELETE /webauthn/credentials/{credential_id}

**Auth:** required. Только свои credentials.

**Response:** `204` или `{ "ok": true }`.

**Errors:** `401`, `404`.

---

## 7) Клиентский flow (MVP)

```
password login → PIN setup
  → optional: register/begin → create() → register/finish

cold start:
  refresh tokens if needed
  → if webauthn_enabled:
       login/begin → get() → login/finish → unlock UI
     else:
       PIN unlock
```

Capacitor native: прежний LocalAuthentication path; WebAuthn — для `!Capacitor.isNativePlatform()`.
