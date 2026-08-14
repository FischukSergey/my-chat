# API Sprint 8 — PWA PIN unlock + push title

Sprint 8 **не добавляет** REST endpoints для PIN и **не включает** WebAuthn.

PIN проверяется только на устройстве. Auth HTTP как в `docs/api-sprint-6.md`.

Единственное server-изменение спринта: контракт **outbox / Web Push payload** для `message_new` (title = username отправителя).

Связанные: `docs/sprint-8-plan.md`, `docs/sprint-8-checklist.md`.

---

## 1) Server API (HTTP)

Новых REST routes **нет**.

| Было в черновике Sprint 8 (WebAuthn) | Статус |
|--------------------------------------|--------|
| `POST /webauthn/register|login/*` | **Отложено** (Sprint 8.1+) |
| `GET/DELETE /webauthn/credentials` | **Отложено** |
| Redis challenge store | **Не нужен** |

Клиент по-прежнему использует register / login / refresh / logout без изменений path/body.

---

## 2) Local storage keys (PWA)

Префикс согласован с существующими `my_chat_*` в `mobile/src/auth.ts`.

| Key | Contents | Notes |
|-----|----------|--------|
| `my_chat_pin_salt` | base64 salt | per-device |
| `my_chat_pin_hash` | base64 hash(PIN, salt) | never store plaintext PIN |
| `my_chat_pin_set` | `"1"` / отсутствует | быстрый флаг; можно выводить из наличия salt+hash |
| `my_chat_refresh_token` | plaintext **или** ciphertext | ciphertext если включён Should encrypt |
| `my_chat_access_token` | как сейчас | короткий TTL; encrypt optional |
| `my_chat_session_id` | как сейчас | |
| `my_chat_user_id` | как сейчас | |

Зафиксированная схема (Must): verifier = `SHA-256(salt || PIN)` (см. `mobile/src/pin.ts`).

Should encrypt refresh:

- `key = PBKDF2(PIN, salt, iterations ≥ 100_000, SHA-256) → AES-256-GCM`
- `my_chat_refresh_token = "enc:v1:" + base64(nonce || AES-GCM(key, refresh))`
- без верного PIN refresh не расшифровать → только Login + новый setup
- legacy plaintext refresh (без префикса) принимается при decrypt до миграции

---

## 3) Клиентские экраны / события

### 3.1 Setup PIN

**Когда:** сразу после успешного register или login, если PIN не задан на устройстве.

**UI:** ввод PIN → подтверждение → persist salt/hash → далее Home (и регистрация push и т.д. как сейчас).

**Ошибки UI:** mismatch confirm, неверная длина, нецифровые символы.

### 3.2 Unlock PIN

**Когда:**

1. Cold start: есть refresh (или encrypted blob) **и** PIN задан.
2. Resume: документ был `hidden` дольше grace period (`PIN_LOCK_GRACE_MS`, default **60000**).

**UI:** поле из 4 цифр; verify запускается автоматически при вводе четвёртой цифры. Отдельной кнопки «Разблокировать» нет. Кнопка «Выйти из аккаунта» остаётся. На native в biometric-режиме — «Разблокировать Face ID».

**Успех:** verify PIN → (decrypt refresh если нужно) → `apiRefresh` → Home.

**Неверный PIN:** поле очищается; счётчик попыток; после **5** → `clearAllTokens` + clear PIN keys → Login.

**Выйти:** clear tokens (+ clear PIN keys — см. plan §3.C).

### 3.3 Change PIN

**Когда:** Settings.

**Flow:** old PIN → verify → new PIN ×2 → re-hash; если refresh encrypted — перешифровать новым ключом.

### 3.4 Миграция уже залогиненных PWA

Если есть refresh, но PIN не задан (пользователи до Sprint 8):

→ показать Setup PIN **до** Home (не silent unlock).

---

## 4) Flow (MVP)

```
register / login (password)
  → if !pin_set: Setup PIN
  → Home

cold start:
  → if !has_session: Login
  → if has_session && !pin_set: Setup PIN
  → if has_session && pin_set: Unlock PIN → refresh → Home

background:
  → hidden longer than grace → next focus: Unlock PIN
  → visible / PIN unlock / WS reconnect: reload chat history (TTL stale UI)

logout:
  → clear tokens + clear PIN keys → Login
```

Capacitor native: cold start с биометрией (`startUnlock`) без изменений Must; PIN-экраны в native — optional.

---

## 5) Константы (рекомендуемые defaults)

| Name | Value |
|------|--------|
| PIN length | 4 digits (`PIN_LENGTH` в `mobile/src/pin.ts`) |
| Grace period | 60_000 ms (`PIN_LOCK_GRACE_MS`) |
| Max attempts | 5 (`PIN_MAX_ATTEMPTS`) |
| PBKDF2 iterations (encrypt) | 100_000 (`PBKDF2_ITERATIONS`) |

Изменения defaults — только через чеклист §1, не молча в коде.

---

## 6) Push / outbox contract (`message_new`)

### 6.1 Outbox payload (в БД `notification_outbox`)

Дополнительно к существующим полям (`event_type`, `user_id`, `message_id`, `dialog_id`, `sender_id`, `preview`, `unread_count`, …):

```json
{
  "event_type": "message_new",
  "sender_id": "…",
  "sender_username": "alice",
  "preview": "текст сообщения (для логов/будущего; не в UI push title)",
  "dialog_id": "…",
  "message_id": "…",
  "unread_count": 1
}
```

`sender_username` — lookup при `enqueueOutbox`. Если username не найден: fallback `"user"` (не подставлять сырой UUID в title без необходимости).

`preview` можно продолжать писать (совместимость / отладка), но **не** использовать как notification title.

### 6.2 Web Push JSON (то, что видит SW)

| Поле | Значение |
|------|----------|
| `title` | `sender_username` |
| `body` | `"Новое сообщение"` (константа, как сейчас) |
| `badge` | актуальный unread |
| `dialog_id` | id диалога |
| `message_id` | id сообщения |

`badge_sync` без изменений (`type` + `badge`).

Системная строка ОС вроде «from MyChat» — из `manifest.json` `name`/`short_name`, не из этого payload.

### 6.3 UI чата (не API)

Фон ленты сообщений: бирюзовый tint + watermark — только клиентский CSS; контракта API нет.
