# Known Limitations — Sprint 8

Дата: 2026-08-08  
Статус: зафиксировано по итогам спринта

---

## 1. PIN защищает UI и refresh ciphertext, не является серверным секретом

**Что сделано:** verifier = `SHA-256(salt||PIN)` в Preferences; refresh хранится как `enc:v1:` (PBKDF2 → AES-GCM). Без PIN UI и ciphertext не открываются.

**Остаётся:** PIN не уходит на сервер и не привязан к JWT family. Threat model — локальный доступ к устройству / storage, не фишинг пароля и не удалённый attacker с украденным refresh в plaintext (после encrypt — только с PIN). Access token в Preferences по-прежнему plaintext (короткий TTL).

**Решение (later):** optional encrypt access; WebAuthn/passkey как primary unlock (Sprint 8.1+).

---

## 2. WebAuthn / Face ID sheet в PWA — out of scope

**Проблема:** Safari Home Screen PWA не даёт Capacitor `LocalAuthentication`; platform passkey (WebAuthn) не внедряли.

**Статус:** PWA = PIN; native Capacitor = Face ID (+ PIN fallback если refresh уже `enc:v1:`). WebAuthn → Sprint 8.1+.

---

## 3. Текст сообщения не показывается в push

**Решение спринта (осознанно):** title = `sender_username`, body = «Новое сообщение». Поле `preview` остаётся в outbox для логов/будущего, **не** мапится в UI уведомления.

**Следствие:** на lock screen меньше утечки содержимого; пользователь не видит превью текста до открытия приложения.

**Решение (later):** опциональный user setting «показывать превью» — только если явно понадобится.

---

## 4. Logout при зашифрованном refresh — server revoke best-effort

**Проблема:** `apiLogout` требует plaintext refresh. Если blob `enc:v1:` и PIN не введён на logout-path без decrypt — revoke на сервере пропускается; локально tokens+PIN всё равно wipe.

**Следствие:** серверная session может жить до expiry/rotation, пока клиент уже «вышел».

**Решение (later):** при logout запрашивать PIN для decrypt → revoke; или endpoint revoke-by-access.

---

## 4a. 401 refresh в PWA больше не зовёт Face ID

**Было:** `fetchAuth` при протухшем access всегда вызывал `getRefreshToken()` → `BiometricAuth.authenticate()`. В PWA плагин отвечает `BiometryError: User cancelled`; параллельно WS после простоя рвался и переподключался со старым JWT.

**Сейчас:** session PIN после успешного unlock; `rotateSession` / `ensureFreshAccess` расшифровывают `enc:v1:` без биометрии; если PIN нет — снова экран Unlock, не Face ID. Native + plaintext refresh по-прежнему через биометрию.

---

## 5. Забыли PIN

**Поведение:** logout (wipe tokens + PIN verifier) → login паролем → Setup PIN заново. Отдельного recovery через email/SMS нет (out of scope).

---

## 6. Вне scope (напоминание)

- WebAuthn / passkeys в PWA (→ 8.1+).
- Redis / серверная проверка PIN.
- E2EE / at-rest тел сообщений → Sprint 9.
- Полный редизайн Home / системная строка «from MyChat» в push.
- Pull-to-refresh ленты чата (системный жест iOS в standalone PWA не работает; кастомный жест не делали — resume/WS reload закрывает stale TTL).
