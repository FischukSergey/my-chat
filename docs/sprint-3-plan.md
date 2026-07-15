# Sprint 3 — детальный план (Face ID + session hardening)

Источник: `docs/chat-architecture-plan.md` §7, §10, §13; `docs/sprint-2-plan.md`, `docs/sprint-2-checklist.md`; `docs/known-limitations-sprint-1.md`.

## 1) Цель спринта

Перевести auth-flow из stateless JWT MVP в безопасную модель с server-side сессиями (ротация refresh, revoke) и добавить мобильный клиент с secure storage и биометрическим unlock.

К концу Sprint 3 должно быть:
- refresh-токены хранятся на клиенте только в secure storage (Keychain / Keystore);
- доступ к refresh защищён локальной биометрией (Face ID / Touch ID / Android biometrics);
- сервер ротирует refresh при каждом обновлении и инвалидирует старый токен;
- logout и reuse detection реально завершают сессию на сервере;
- auth-сценарии воспроизводимы через debug UI и мобильный клиент.

**Критерий готовности (из architecture plan):** вход в приложение защищён биометрией, refresh flow безопасен.

## 2) Входные условия (что уже готово после Sprint 2)

- Реализован полный chat-flow: `login -> send -> receive -> read -> unread`.
- Работают push/badge: device registry, notification-worker, outbox, `badge_updated`.
- `auth-proxy` выдаёт access + refresh JWT (HS256), но **без** session store в БД.
- `POST /api/v1/auth/logout` — best-effort: валидирует refresh, но не revoke.
- `POST /api/v1/auth/refresh` — выдаёт новую пару, **старый refresh остаётся валидным**.
- Debug UI `/debug` сохраняет только `access_token`; нет шорткатов refresh/logout.
- Мобильного клиента в репозитории **нет** — только backend + debug web client.
- Auth-логика живёт в `internal/handlers/auth/handler.go` без сервисного слоя (`internal/services/auth/` отсутствует).
- `auth-proxy` **не подключён** к PostgreSQL (в отличие от `main-service` и `notification-worker`).

## 3) Границы Sprint 3

### Входит в Sprint 3

- Backend session hardening: таблица сессий, ротация refresh, revoke, reuse detection.
- Сервисный слой `internal/services/auth/` и рефакторинг `auth-proxy`.
- Расширение JWT claims (`session_id`, опционально `family_id`).
- Audit-логирование критичных auth-событий (login, refresh, revoke, reuse detected).
- Debug UI: хранение refresh, шорткаты refresh/logout, сценарии rotation/revoke.
- Минимальный мобильный клиент (Capacitor): login, secure storage, biometric gate, auto-refresh.
- Unit/integration тесты auth-сценариев.
- Документация API Sprint 3 и ручной test-runbook.

### Не входит в Sprint 3

- TTL / `message_deleted` / `message-expirer` (Sprint 4).
- Production APNs/FCM (отложено с Sprint 2).
- Полноценный IdP / OAuth / парольная аутентификация.
- Rate limiting, RBAC, Prometheus/OpenTelemetry (кроме auth audit logs).
- Проверка `session_id` в middleware `main-service` при каждом запросе (опционально; см. §8 риски).
- Полный UI чата в мобильном приложении (достаточно auth-flow + smoke API call).

## 4) Архитектурные решения (зафиксировать в начале спринта)

### 4.1 Модель refresh-токена

**Рекомендация:** JWT refresh с обязательной server-side валидацией по hash в БД.

- Клиент получает opaque-by-value JWT refresh, но сервер хранит только `SHA-256(refresh_token)`.
- В claims refresh/access добавляется `session_id` (UUID).
- При refresh: lookup по hash → проверка `revoked_at IS NULL` и `expires_at > now()` → revoke старой записи → создать новую в той же `family_id`.

### 4.2 Таблица `auth_sessions`

```sql
auth_sessions (
  id            uuid PRIMARY KEY,          -- session_id в JWT
  user_id       uuid NOT NULL REFERENCES users(id),
  family_id     uuid NOT NULL,             -- для reuse detection
  token_hash    text NOT NULL UNIQUE,      -- SHA-256(refresh)
  device_id     uuid NULL,                 -- опциональная связь с devices
  created_at    timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL,
  revoked_at    timestamptz NULL,
  rotated_from  uuid NULL REFERENCES auth_sessions(id)
)
```

Индексы: `(user_id, revoked_at)`, `(token_hash)`, `(family_id)`.

### 4.3 Reuse detection

Если клиент повторно отправляет refresh-токен, который уже был rotated/revoked:
- сервер revoke **всю family** (все сессии с тем же `family_id`);
- ответ `401` с кодом `session_compromised`;
- audit log: `auth_reuse_detected`.

### 4.4 Мобильный клиент

**Рекомендация:** Ionic + Capacitor в каталоге `mobile/` (или `client/`) в том же репозитории.

Плагины:
- secure storage — `@capacitor/preferences` с secure flag или community secure-storage;
- biometrics — `@capacitor-community/biometric-auth` (или аналог).

Правило: refresh **никогда** не хранится в plain localStorage.

### 4.5 Связь биометрии и сервера

Face ID / Touch ID — **только локальная** защита доступа к refresh. Сервер не знает о биометрии.
При смене биометрии на устройстве (или `biometry lockout`) клиент:
1. удаляет refresh из secure storage;
2. вызывает `POST /auth/logout` (best-effort);
3. перенаправляет на экран login.

## 5) Sprint backlog (детализация задач)

## A. Контракты и документация

- Зафиксировать модель sessions, правила rotation/reuse/revoke.
- Подготовить `docs/api-sprint-3.md`:
  - обновлённые контракты login/refresh/logout;
  - новые коды ошибок (`session_expired`, `session_revoked`, `session_compromised`);
  - опционально: `POST /api/v1/auth/sessions/revoke-all` для logout со всех устройств.
- Обновить `docs/debug-manual-test.md` — auth-сценарии Sprint 3.

## B. Модель данных и миграции

- Добавить миграцию `007_auth_sessions.sql`.
- Добавить модель `AuthSession` в `internal/store/models.go`.
- Проверить идемпотентность миграции (повторный запуск на чистой и существующей БД).

## C. Store-слой

- Реализовать `AuthSessionRepository`:
  - `CreateSession(ctx, userID, familyID, tokenHash, expiresAt, deviceID?)`;
  - `FindByTokenHash(ctx, hash)`;
  - `RevokeSession(ctx, sessionID)`;
  - `RevokeFamily(ctx, familyID)`;
  - `RevokeAllForUser(ctx, userID)`;
  - `MarkRotated(ctx, oldSessionID, newSession)` — транзакция: revoke old + insert new.
- Unit/integration тесты репозитория.

## D. Auth service

- Создать `internal/services/auth/service.go`:
  - `Login(ctx, userID, deviceID?)` → token pair + session record;
  - `Refresh(ctx, refreshToken)` → rotation + новая пара;
  - `Logout(ctx, refreshToken)` → revoke session;
  - `RevokeAll(ctx, userID)` — опционально для «выйти везде»;
  - reuse detection внутри `Refresh`.
- Опционально: проверка `users.status = active` при login.
- Structured audit logs: `auth_login`, `auth_refresh`, `auth_logout`, `auth_reuse_detected`.

## E. JWT и handlers

- Расширить `internal/jwt/jwt.go`:
  - claim `session_id` в access и refresh;
  - helpers `IssueAccessWithSession`, `IssueRefreshWithSession`, `ParseSessionID`.
- Рефакторинг `internal/handlers/auth/handler.go` — делегирование в auth service.
- Единый формат ошибок (расширить `details` при необходимости).

## F. Bootstrap `auth-proxy`

- Подключить PostgreSQL к `auth-proxy` (как в `main-service`):
  - конфиг `database` в `configs/config.auth-proxy.local.example.yaml`;
  - `Migrate()` при старте;
  - wiring repository + service + handler в `internal/app/authproxy/app.go`.
- Обновить `deploy/local/docker-compose.local.yml`:
  - `auth-proxy` → `depends_on: postgres`;
  - healthcheck с учётом БД.

## G. Debug UI

- Поле/хранилище `refresh_token` в localStorage debug UI (допустимо для dev; не secure storage).
- Шорткаты:
  - **Refresh** — `POST /auth/refresh`, обновить access + refresh;
  - **Logout** — `POST /auth/logout`, очистить токены;
  - **Reuse test** — повторный refresh со старым token (ожидание 401).
- Auto-refresh access при 401 на защищённых запросах (опционально, улучшает отладку WS).

## H. Мобильный клиент (Capacitor)

- Scaffold проекта в `mobile/`:
  - экран Login (ввод `user_id` для dev, как в debug);
  - экран «Unlock» с biometric prompt при cold start;
  - secure storage wrapper для refresh;
  - HTTP client с auto-refresh при 401;
  - smoke: после unlock → `GET /api/v1/me/unread-count`.
- Обработка ошибок `session_compromised` → wipe tokens + redirect login.
- README в `mobile/` с инструкцией сборки для iOS Simulator / Android Emulator.

## I. Тестирование и качество

- Unit tests:
  - auth service (login, refresh rotation, logout, reuse detection);
  - auth handlers (success/error paths);
  - JWT session claims.
- Integration tests (build tag `integration`):
  - login → refresh → старый refresh invalid;
  - logout → refresh fails;
  - reuse detection → family revoked;
  - опционально: login → API call с access → refresh → API call.
- `task fmt`, `task lint`, `task test`, `task test:integration`.

## 6) Разбивка по дням (ориентир на 10 рабочих дней)

### День 1
- Утвердить архитектурные решения (§4): модель sessions, JWT+hash, reuse detection.
- Подготовить `docs/api-sprint-3.md`.
- Написать миграцию `007_auth_sessions.sql`.

### День 2
- Реализовать `AuthSessionRepository` + integration тесты.
- Расширить JWT claims (`session_id`).

### День 3
- Реализовать `internal/services/auth/` (Login, Refresh с rotation, Logout).
- Audit logging auth-событий.

### День 4
- Рефакторинг handlers и wiring в `auth-proxy`.
- Подключить PostgreSQL к `auth-proxy`, обновить docker-compose.

### День 5
- Unit-тесты auth service и handlers.
- Integration-тесты rotation/revoke/reuse.

### День 6
- Debug UI: refresh_token storage, шорткаты Refresh / Logout / Reuse test.
- Обновить `docs/debug-manual-test.md`.

### День 7
- Scaffold Capacitor-проекта в `mobile/`.
- Secure storage wrapper + login screen.

### День 8
- Biometric unlock flow (cold start gate перед чтением refresh).
- Auto-refresh HTTP client.

### День 9
- Smoke e2e на симуляторе: login → biometric → refresh → API call → logout.
- Обработка `session_compromised` и смены биометрии.

### День 10
- Буфер на фиксы и стабилизацию.
- Freeze, демо Sprint 3, `docs/known-limitations-sprint-3.md`.

## 7) Definition of Done (DoD) для Sprint 3

Спринт считается завершённым, если:

- [ ] При login создаётся запись в `auth_sessions`, refresh содержит `session_id`.
- [ ] При refresh старый refresh-токен перестаёт приниматься сервером.
- [ ] При logout refresh revoke в БД; повторный refresh возвращает 401.
- [ ] Reuse detection revoke family и логирует событие.
- [ ] Debug UI воспроизводит login → refresh → logout → failed refresh.
- [ ] Мобильный клиент: refresh в secure storage, biometric gate работает на симуляторе/эмуляторе.
- [ ] Cold start → biometric → auto-refresh → успешный API-запрос.
- [ ] `task lint` и `task test` проходят стабильно; integration-тесты auth зелёные.
- [ ] Документация Sprint 3 актуализирована.

## 8) Демо-сценарий Sprint 3

### Backend (debug UI)

1. Поднять local: `postgres + auth-proxy + main-service`.
2. Login user A → сохранены access + refresh.
3. Refresh → получена новая пара; старый refresh → 401.
4. Logout → refresh revoke.
5. Попытка refresh после logout → 401.
6. Reuse test: сохранить refresh, сделать refresh, повторить со старым → 401 + family revoked.

### Mobile

1. Установить app на iOS Simulator / Android Emulator.
2. Login user A → refresh сохранён в secure storage.
3. Закрыть app, открыть → biometric prompt → unlock.
4. App автоматически refresh access и показывает unread count.
5. Logout → повторный unlock → redirect на login (refresh отсутствует).

## 9) Риски Sprint 3 и меры

| Риск | Влияние | Мера |
|------|---------|------|
| Нет мобильного клиента к mid-sprint | Критерий «биометрия» не закрыт | Параллельный track: backend (дни 1–6) + mobile (дни 7–9) |
| auth-proxy без БД блокирует rotation | Нельзя начать hardening | День 4 — приоритет wiring PG |
| Access живёт после logout до exp (~15 мин) | Окно уязвимости | Короткий access TTL (уже 15 мин); опционально session check в middleware |
| Reuse detection ложно срабатывает | Mass logout | Тщательные integration-тесты; grace period не нужен при корректной ротации |
| Биометрия недоступна на CI | Нет автотестов biometrics | Manual QA на симуляторе; unit-тесты mock secure storage |
| Shared JWT secret, main-service не проверяет revoke | Access valid после logout | Документировать как known limitation или добавить optional session check |
| Refresh rotation + WS reconnect | WS падает при exp access | Клиент: refresh перед WS connect; debug: auto-refresh |

## 10) Артефакты по итогам спринта

- миграция `007_auth_sessions.sql`;
- `AuthSessionRepository` + `internal/services/auth/`;
- рефакторинг `auth-proxy` с PostgreSQL;
- расширенные JWT claims и rotation/revoke/reuse detection;
- debug UI: refresh/logout/reuse shortcuts;
- минимальный Capacitor-клиент в `mobile/`;
- `docs/api-sprint-3.md`, `docs/sprint-3-checklist.md`, `docs/known-limitations-sprint-3.md`;
- unit + integration тесты auth-flow.

## 11) Граф зависимостей задач

```
Контракты API ──→ Миграция auth_sessions ──→ AuthSessionRepository
                                                    ↓
                                          Auth Service (rotation/revoke)
                                                    ↓
                              auth-proxy refactor + audit logs
                                    ↓                    ↓
                          Debug refresh/logout      Mobile secure storage
                                    ↓                    ↓
                              Backend E2E tests    Biometric gate → refresh
                                    └────────┬───────────┘
                                             ↓
                                      Sprint 3 DoD
```

**Критический путь:** миграция → repository → auth service → auth-proxy wiring → тесты rotation/revoke → mobile biometric flow.
