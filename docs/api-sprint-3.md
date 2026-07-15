# API контракты Sprint 3 (Face ID + session hardening)

Источник: `docs/sprint-3-plan.md`, `docs/sprint-3-checklist.md`, `docs/api-sprint-1.md`.

## 1) Зафиксированные решения Sprint 3

- `auth-proxy` переходит на server-side session store (таблица `auth_sessions`).
- Refresh-токены хранят ссылку на сессию через claim `session_id`.
- Сервер хранит только `SHA-256(refresh_token)`, plaintext не сохраняется.
- При каждом `refresh`: старая запись revoke-ится, выдаётся новая (rotation).
- Reuse detection: повторное использование revoked refresh → revoke всей family → `401`.
- `logout` теперь реально завершает сессию на сервере.
- Добавляется endpoint `POST /api/v1/auth/sessions/revoke-all` (опциональный).
- JWT access и refresh расширяются claim `session_id`.
- Контракты Chat API (`main-service`) и Device API **не меняются** в Sprint 3.

## 2) Scope Sprint 3 по контрактам

В Sprint 3 входят:
- обновлённые контракты `login`, `refresh`, `logout` (session-aware);
- новые коды ошибок auth (`session_expired`, `session_revoked`, `session_compromised`);
- endpoint `POST /api/v1/auth/sessions/revoke-all`;
- поведение rotation и reuse detection;
- расширение JWT claims (`session_id`).

В Sprint 3 не входят:
- изменение Chat/Device/WS контрактов (`main-service`);
- проверка `session_id` в middleware `main-service` (опционально, planned risk);
- production APNs/FCM;
- OAuth / парольная аутентификация.

## 3) Общие соглашения

- Base path API: `/api/v1`.
- Формат данных: `application/json`.
- Идентификаторы: UUID в строковом виде.
- Время: RFC3339 (`2026-07-15T12:34:56Z`).
- Аутентификация защищённых endpoints:
  - `Authorization: Bearer <access_token>`.
- Сервис `auth-proxy` обслуживает все `/api/v1/auth/` маршруты.

## 4) Формат ошибок

Формат неизменен относительно Sprint 1–2:

```json
{
  "error": {
    "code": "session_revoked",
    "message": "refresh token has been revoked",
    "details": {}
  }
}
```

### Коды ошибок Sprint 3 (новые)

| Код | HTTP | Когда используется |
|-----|------|--------------------|
| `session_expired` | 401 | Refresh-токен просрочен (`expires_at < now`) |
| `session_revoked` | 401 | Refresh-токен отозван (logout или rotation) |
| `session_compromised` | 401 | Reuse detection: revoked-токен использован повторно; вся family revoked |

### Коды ошибок из Sprint 1–2 (остаются)

| Код | HTTP |
|-----|------|
| `invalid_argument` | 400 |
| `unauthenticated` | 401 |
| `forbidden` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `internal` | 500 |

## 5) JWT claims (расширение)

### Access token (изменение)

В Sprint 3 к существующим claims `user_id`, `token_type`, `iat`, `exp` добавляется:

| Claim | Тип | Описание |
|-------|-----|----------|
| `session_id` | `string` (UUID) | Идентификатор сессии в `auth_sessions` |

Пример payload (base64-decoded):

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111",
  "token_type": "access",
  "session_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
  "iat": 1752566000,
  "exp": 1752566900
}
```

### Refresh token (изменение)

Те же новые поля:

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111",
  "token_type": "refresh",
  "session_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
  "iat": 1752566000,
  "exp": 1752652400
}
```

Примечание: несмотря на наличие `session_id` в JWT, сервер **обязан** валидировать токен через `auth_sessions` (по hash), не полагаясь только на подпись.

## 6) Auth API (`auth-proxy`)

### `POST /api/v1/auth/login`

Назначение: вход пользователя, создание новой сессии, выдача token pair.

Request (без изменений):

```json
{
  "user_id": "11111111-1111-1111-1111-111111111111"
}
```

Правила валидации:
- `user_id` обязателен, валидный UUID.

Поведение (изменение Sprint 3):
- создаётся запись в `auth_sessions` с новым `session_id` и `family_id`;
- в БД сохраняется `SHA-256(refresh_token)`;
- access и refresh содержат `session_id` в claims.

Response `200`:

```json
{
  "access_token": "jwt-access",
  "refresh_token": "jwt-refresh",
  "token_type": "Bearer",
  "expires_in": 900,
  "session_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
}
```

Новое поле `session_id` в ответе — для клиента, который хочет отображать список активных сессий.

Ошибки:
- `400 invalid_argument` — `user_id` отсутствует или не UUID.
- `500 internal` — ошибка при создании сессии.

---

### `POST /api/v1/auth/refresh`

Назначение: ротация refresh-токена и выдача новой пары.

Request (без изменений по структуре):

```json
{
  "refresh_token": "jwt-refresh-old"
}
```

Поведение (полностью изменено в Sprint 3):

1. Парсить refresh JWT, извлечь `session_id` и `user_id`.
2. Вычислить `SHA-256(refresh_token)`.
3. Найти запись в `auth_sessions` по `token_hash`.
4. Проверки:
   - запись существует → иначе `401 session_revoked`;
   - `revoked_at IS NULL` → иначе **reuse detection** (см. §8);
   - `expires_at > now()` → иначе `401 session_expired`.
5. Атомарно (транзакция):
   - revoke старой записи (`revoked_at = now()`);
   - создать новую запись с новым `session_id`, тем же `family_id`, новым `token_hash`.
6. Выдать новую пару токенов с новым `session_id`.

Response `200`:

```json
{
  "access_token": "jwt-access-new",
  "refresh_token": "jwt-refresh-new",
  "token_type": "Bearer",
  "expires_in": 900,
  "session_id": "cccccccc-cccc-cccc-cccc-cccccccccccc"
}
```

Ошибки:
- `400 invalid_argument` — `refresh_token` отсутствует.
- `401 unauthenticated` — невалидная подпись JWT.
- `401 session_expired` — срок действия записи истёк.
- `401 session_revoked` — запись уже отозвана (без reuse; например, явный logout).
- `401 session_compromised` — reuse detection: токен был использован повторно, family revoked.
- `500 internal` — ошибка транзакции rotation.

---

### `POST /api/v1/auth/logout`

Назначение: завершение текущей сессии, revoke refresh-токена.

Request (без изменений по структуре):

```json
{
  "refresh_token": "jwt-refresh"
}
```

Поведение (изменение Sprint 3):

1. Парсить refresh JWT, извлечь `session_id`.
2. Найти запись в `auth_sessions` по `token_hash`.
3. Установить `revoked_at = now()`.
4. Если запись не найдена или уже revoked — всё равно вернуть `204` (идемпотентность).

Response `204`: пустое тело.

Примечание: существующий access-токен продолжает работать до `exp` (~15 мин). Это known limitation; устраняется опциональной проверкой `session_id` в middleware `main-service` (вне scope Sprint 3).

Ошибки:
- `400 invalid_argument` — `refresh_token` отсутствует.
- `401 unauthenticated` — невалидная подпись JWT.
- `500 internal` — ошибка записи в БД.

---

### `POST /api/v1/auth/sessions/revoke-all` *(опциональный)*

Назначение: завершение всех активных сессий пользователя («выйти везде»).

Требования: `Authorization: Bearer <access_token>`.

Request: пустое тело.

Поведение:
- revoke все записи в `auth_sessions` с `user_id = current_user` и `revoked_at IS NULL`.

Response `204`: пустое тело.

Ошибки:
- `401 unauthenticated` — невалидный access.
- `500 internal`.

---

## 7) Модель `auth_sessions`

Описание внутренней таблицы для понимания server-side поведения.

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | uuid | `session_id` в JWT claims |
| `user_id` | uuid | Владелец сессии |
| `family_id` | uuid | Группа для reuse detection |
| `token_hash` | text | `SHA-256(refresh_token)`, UNIQUE |
| `device_id` | uuid, nullable | Связь с `devices` (опционально) |
| `created_at` | timestamptz | Время создания |
| `expires_at` | timestamptz | Срок действия refresh |
| `revoked_at` | timestamptz, nullable | Время revoke; `NULL` = активна |
| `rotated_from` | uuid, nullable | FK на предыдущую запись (chain) |

## 8) Reuse Detection

Реuse — повторное использование refresh-токена, который уже был rotated или revoked.

Сценарий:

1. Клиент A сделал `refresh` → session `S1` revoked, создана `S2`.
2. Злоумышленник (или баг) отправляет refresh-токен сессии `S1`.
3. Сервер находит `S1` по hash → видит `revoked_at IS NOT NULL`.
4. Сервер revoke всю family (`family_id = S1.family_id`), включая `S2`.
5. Возвращает `401 session_compromised`.

После `session_compromised`:
- клиент обязан удалить refresh из secure storage;
- требуется повторный full login.

Audit log при reuse: `auth_reuse_detected` с полями `user_id`, `session_id`, `family_id`.

## 9) Правила истечения токенов

| Токен | TTL по умолчанию | Источник |
|-------|------------------|----------|
| Access | 900 сек (15 мин) | `config.jwt.access_token_ttl_sec` |
| Refresh | 86400 сек (24 ч) | `config.jwt.refresh_token_ttl_sec` |

- `expires_at` в `auth_sessions` соответствует `exp` в refresh JWT.
- После истечения `expires_at` запись можно физически не удалять — достаточно проверки в коде.

## 10) Audit Logging

В Sprint 3 сервис логирует следующие события через `slog` в структурированном виде:

| Event | Когда | Поля |
|-------|-------|------|
| `auth_login` | Успешный login | `user_id`, `session_id`, `family_id` |
| `auth_refresh` | Успешный refresh | `user_id`, `old_session_id`, `new_session_id` |
| `auth_logout` | Успешный logout | `user_id`, `session_id` |
| `auth_reuse_detected` | Reuse detection | `user_id`, `session_id`, `family_id` |

## 11) Поведение на клиенте (мобильный / debug)

### Secure storage (мобильный)

- Refresh-токен **никогда** не хранится в незащищённом хранилище (не localStorage, не cookies без Secure/HttpOnly).
- iOS: Keychain через Capacitor secure storage plugin.
- Android: Keystore / EncryptedSharedPreferences через Capacitor secure storage plugin.

### Biometric gate

- При cold start: если refresh присутствует в secure storage → запросить биометрию.
- При успехе: вызвать `POST /api/v1/auth/refresh` → сохранить новую пару.
- При неудаче биометрии: не открывать приложение, показать повтор или ввод пароля.
- При смене биометрии / lockout: удалить refresh → `POST /api/v1/auth/logout` (best-effort) → экран login.

### Auto-refresh access

- При `401` на любом защищённом запросе: попытаться обновить access через `refresh`.
- Если `refresh` тоже возвращает `401` (`session_revoked`, `session_compromised`, `session_expired`): wipe secure storage → redirect login.
- Одновременно не запускать два refresh (использовать мьютекс / очередь ожидающих запросов).

### Debug UI (localhost)

- Refresh-токен хранится в `localStorage` (допустимо только для dev).
- Шорткат **Refresh**: `POST /api/v1/auth/refresh` → обновить access + refresh в localStorage.
- Шорткат **Logout**: `POST /api/v1/auth/logout` → очистить оба токена.
- Шорткат **Reuse test**: сохранить текущий refresh, сделать refresh, затем отправить старый → ожидаемый ответ `401 session_compromised`.

## 12) E2E сценарии (тестовые кейсы)

### Сценарий 1: Login → Refresh → Logout

```
POST /auth/login {user_id}
  → 200 {access, refresh, session_id}

POST /auth/refresh {refresh}
  → 200 {access_new, refresh_new, session_id_new}

POST /auth/refresh {refresh}  ← старый refresh
  → 401 session_revoked

POST /auth/logout {refresh_new}
  → 204

POST /auth/refresh {refresh_new}
  → 401 session_revoked
```

### Сценарий 2: Reuse Detection

```
POST /auth/login {user_id}
  → 200 {access, refresh_A, session_id_A}

POST /auth/refresh {refresh_A}
  → 200 {access_new, refresh_B, session_id_B}

POST /auth/refresh {refresh_A}  ← повторный старый
  → 401 session_compromised

POST /auth/refresh {refresh_B}  ← текущий тоже revoked
  → 401 session_revoked (или session_compromised)
```

### Сценарий 3: Expired token

```
POST /auth/login {user_id}
  → 200 {access, refresh}

(ждать expires_at > now)

POST /auth/refresh {refresh}
  → 401 session_expired
```

### Сценарий 4: Mobile biometric cold start

```
App запуск → read refresh from Keychain → biometric prompt
  → success → POST /auth/refresh → сохранить новую пару
  → GET /api/v1/me/unread-count → 200 {unread_count}
```

## 13) Обратная совместимость

| Компонент | Sprint 1–2 контракт | Sprint 3 изменение |
|-----------|---------------------|--------------------|
| `POST /auth/login` response | без `session_id` | `session_id` добавлен (backward compatible) |
| `POST /auth/refresh` response | без `session_id` | `session_id` добавлен (backward compatible) |
| access JWT claims | без `session_id` | `session_id` добавлен; `middleware` игнорирует (не проверяет в Sprint 3) |
| refresh JWT claims | без `session_id` | `session_id` добавлен |
| `POST /auth/logout` | 204 без revoke | 204 с реальным revoke (backward compatible) |
| Chat / Device API | без изменений | без изменений |
| WS события | без изменений | без изменений |

**Важно:** debug-клиент, написанный под Sprint 1–2, продолжит работать — новые поля в ответах аддитивны.

## 14) Критерий совместимости

Любая реализация Sprint 3 должна соблюдать этот документ:
- handlers/services не меняют описанные здесь поля и значения без обновления файла;
- debug-сценарии используют именно эти endpoint/коды ошибок;
- integration-тесты Sprint 3 проверяют именно эти контракты и поведение.
