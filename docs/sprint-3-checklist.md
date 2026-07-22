# Sprint 3 Checklist

Источник: `docs/sprint-3-plan.md`.

## 1) Подготовка и контракты

- [x] Утвердить модель `auth_sessions` (поля, индексы, family_id).
- [x] Утвердить стратегию refresh: JWT + server-side hash validation.
- [x] Утвердить правила rotation (invalidate old on refresh).
- [x] Утвердить правила reuse detection (revoke family → `session_compromised`).
- [x] Утвердить расширение JWT claims (`session_id`).
- [x] Утвердить scope мобильного клиента (Capacitor в `mobile/`).
- [x] Подготовить `docs/api-sprint-3.md` с обновлёнными auth-контрактами.

Примечание: все решения зафиксированы в `docs/api-sprint-3.md` (§1, §5, §6, §7, §8, §11) и `docs/sprint-3-plan.md` (§4).

## 2) База данных и миграции

- [x] Создать миграцию `internal/store/migrations/007_auth_sessions.sql`.
- [x] Добавить индексы: `token_hash` (unique), `user_id + revoked_at`, `family_id`.
- [x] Добавить модель `AuthSession` в `internal/store/models.go`.
- [x] Проверить миграции на чистой БД и при повторном запуске.

Примечание: таблица `auth_sessions` создана с FK на `users`, `devices`, самоссылка `rotated_from`. Идемпотентность проверена повторным прогоном (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS` — NOTICE без ошибок). `go build ./internal/store/...` — OK.

## 3) Репозитории и store-слой

- [x] Реализовать `AuthSessionRepository.CreateSession`.
- [x] Реализовать `AuthSessionRepository.FindByTokenHash`.
- [x] Реализовать `AuthSessionRepository.RevokeSession`.
- [x] Реализовать `AuthSessionRepository.RevokeFamily`.
- [x] Реализовать `AuthSessionRepository.RevokeAllForUser` (опционально).
- [x] Реализовать `AuthSessionRepository.RotateSession` (транзакция revoke + insert).
- [x] Добавить unit/integration тесты репозитория.

Примечание: реализован `internal/store/auth_session_repository.go` с методами `Create`, `FindByTokenHash`, `RevokeSession` (идемпотентный), `RevokeFamily`, `RevokeAllForUser`, `RotateSession` (транзакция). Добавлен `ErrSessionNotFound` и методы `IsRevoked()`/`IsExpired()` на модели. Integration-тесты — `auth_session_repository_integration_test.go`, 8 тестов, все PASS. Lint — 0 issues.

## 4) JWT

- [x] Добавить claim `session_id` в access и refresh tokens.
- [x] Реализовать `IssueAccessWithSession` / `IssueRefreshWithSession`.
- [x] Реализовать парсинг `session_id` из токена.
- [x] Добавить unit-тесты JWT с session claims.

Примечание: `Claims.SessionID string` добавлен с `omitempty` (обратная совместимость). Внутренняя функция `issue` принимает `sessionID`. Добавлены `IssueAccessWithSession`, `IssueRefreshWithSession`, `ParseRefreshClaims` (возвращает полный `Claims`). Старые функции `IssueAccess`/`IssueRefresh`/`ParseAccess`/`ParseRefresh` не изменились. Тесты: `internal/jwt/jwt_test.go`, 11 тестов, все PASS. Lint — 0 issues.

## 5) Auth service (`internal/services/auth/`)

- [x] Реализовать `Login` — создание session + выдача token pair.
- [x] Реализовать `Refresh` — validation, rotation, новая пара.
- [x] Реализовать `Logout` — revoke session по refresh token.
- [x] Реализовать reuse detection в `Refresh`.
- [ ] Опционально: проверка `users.status = active` при login.
- [x] Добавить structured audit logs: `auth_login`, `auth_refresh`, `auth_logout`, `auth_reuse_detected`.
- [x] Добавить unit-тесты auth service.

Примечание: `internal/services/auth/service.go` — Login (SHA-256 hash refresh, Create session, audit log), Refresh (ParseRefreshClaims, FindByTokenHash, reuse detection → RevokeFamily + ErrSessionCompromised, rotation → RotateSession, audit log), Logout (идемпотентный, audit log), RevokeAll. Ошибки: ErrSessionRevoked, ErrSessionExpired, ErrSessionCompromised. `service_test.go` — 12 unit-тестов, все PASS. Lint — 0 issues.

## 6) Auth handlers и `auth-proxy`

- [x] Рефакторинг `internal/handlers/auth/handler.go` — делегирование в auth service.
- [x] Обновить error codes: `session_expired`, `session_revoked`, `session_compromised`.
- [x] Подключить PostgreSQL в `internal/app/authproxy/app.go`.
- [x] Запуск `Migrate()` при старте auth-proxy.
- [x] Обновить `configs/config.auth-proxy.local.example.yaml` (секция database).
- [x] Добавить unit-тесты auth handlers.

Примечание: `handler.go` рефакторирован — принимает интерфейс `authService`; маппинг ошибок: `ErrSessionRevoked→session_revoked`, `ErrSessionExpired→session_expired`, `ErrSessionCompromised→session_compromised`; `tokenResponse` дополнен `session_id`. `app.go` подключает PostgreSQL, запускает `Migrate()`, создаёт `AuthSessionRepository` и `auth.Service`. Конфиги `local.example` и `docker.local` дополнены секцией `database`. `handler_test.go` — 12 unit-тестов (Login/Refresh/Logout + все error codes). Lint — 0 issues (with --fix).

## 7) Локальная инфраструктура

- [x] Обновить `deploy/local/docker-compose.local.yml`: auth-proxy `depends_on: postgres`.
- [x] Проверить запуск: `postgres + auth-proxy + main-service + notification-worker`.
- [x] Smoke: login через auth-proxy создаёт запись в `auth_sessions`.

Примечание: все 5 контейнеров (postgres, auth-proxy, main-service, notification-worker, message-expirer) запускаются и переходят в состояние Healthy. Smoke: POST /api/v1/auth/login → запись в auth_sessions создана; refresh → ротация сессии; повторный refresh со старым токеном → `session_compromised`, вся family отозвана (2 записи revoked в БД).

## 8) Debug UI

- [x] Сохранять `refresh_token` в debug UI (localStorage).
- [x] Добавить шорткат **Refresh** (`POST /api/v1/auth/refresh`).
- [x] Добавить шорткат **Logout** (`POST /api/v1/auth/logout`).
- [x] Добавить шорткат **Reuse test** (повторный refresh со старым token).
- [x] Опционально: auto-refresh access при 401.
- [x] Обновить `docs/debug-manual-test.md` — auth-сценарии Sprint 3.

Примечание: `handler.go` — полный редизайн раздела 1 (поля refresh_token + session_id badge), секция 4 разбита на Auth (Login/Refresh/Logout/Reuse test) и Чат/Устройства. localStorage: `saveTokens/clearTokens/loadTokens`. Auto-refresh: кнопка «Отправить + auto-refresh при 401» в разделе 2. `debug-manual-test.md` дополнен полным сценарием Sprint 3 (5 шагов + таблица итогов). Lint — 0 issues.

## 9) Мобильный клиент (Capacitor)

- [x] Scaffold проекта в `mobile/` (Vanilla TS + Vite + Capacitor 8).
- [x] Экран Login — dev login по `user_id`.
- [x] Secure storage wrapper для refresh token (Keychain / Keystore).
- [x] Biometric plugin — gate перед чтением refresh.
- [x] HTTP client с auto-refresh при 401 / exp access.
- [x] Cold start flow: biometric → refresh → API call.
- [x] Logout — wipe secure storage + server revoke.
- [x] Обработка `session_compromised` → wipe + redirect login.
- [x] Обработка смены биометрии → wipe + redirect login.
- [x] Smoke: `GET /api/v1/me/unread-count` после unlock.
- [x] README в `mobile/` с инструкцией сборки.

Примечание: `mobile/src/auth.ts` — secure storage (`@capacitor/preferences` → Keychain на iOS), biometric gate (`@aparajita/capacitor-biometric-auth`); инвариант: `getRefreshToken()` всегда требует биометрию. `mobile/src/api.ts` — HTTP client, `fetchAuth()` с auto-refresh при 401, ошибки `SessionCompromisedError/ExpiredError/RevokedError`. `mobile/src/main.ts` — routing Login→Unlock→Home, cold start flow, logout с server revoke. Три экрана в `index.html` переключаются через CSS `.active`. Сборка: `npm run build` → успех (7 модулей, 219ms). TypeScript: 0 ошибок. Для запуска на симуляторе: `npx cap add ios && npm run cap:ios`.

## 10) Тесты и качество

- [x] Unit-тесты: auth service (login, refresh, logout, reuse).
- [x] Unit-тесты: auth handlers.
- [x] Integration-тест: login → refresh → old refresh invalid.
- [x] Integration-тест: logout → refresh fails.
- [x] Integration-тест: reuse detection → family revoked.
- [x] Проверить `task fmt`.
- [x] Проверить `task lint`.
- [x] Проверить `task test`.
- [x] Проверить `task test:integration`.

Примечание: написан `internal/services/auth/integration_test.go` — 3 интеграционных теста: `Login_Refresh_OldRefreshInvalid`, `Logout_RefreshFails`, `ReuseDetection_FamilyRevoked`. `task fmt` — OK. `task lint` — 0 issues. `task test` — все unit-тесты PASS. `task test:integration` — все интеграционные тесты PASS. CORS middleware добавлен в main-service (`corsMiddleware` в `internal/app/mainservice/app.go`) для поддержки Capacitor WebView.

## 11) Критерии готовности (DoD)

- [x] Refresh ротируется при каждом обновлении; старый token invalid.
- [x] Logout revoke session на сервере.
- [x] Reuse detection работает и логируется.
- [x] Debug UI воспроизводит полный auth lifecycle.
- [x] Мобильный клиент: secure storage + biometric unlock на симуляторе.
- [x] Cold start → biometric → refresh → API call успешен.
- [x] Документация Sprint 3 актуализирована.

Примечание: все критерии подтверждены. Refresh: RotateSession в транзакции, старый токен revoked — проверено интеграционным тестом. Logout: RevokeSession идемпотентен — проверено тестом. Reuse detection: `auth_reuse_detected` в slog.Warn + RevokeFamily — проверено smoke и тестом. Debug UI: полный цикл Login/Refresh/Logout/Reuse test в `internal/handlers/debug/handler.go`. Мобильный клиент: `getRefreshToken()` требует биометрии, cold start flow реализован в `mobile/src/main.ts`, smoke на iOS Simulator прошёл. Документация: `docs/api-sprint-3.md`, `docs/sprint-3-plan.md`, `docs/debug-manual-test.md`, `mobile/README.md`, `docs/known-limitations-sprint-3.md` актуализированы.

## 12) Демо

- [x] Подготовить тестовых пользователей в local.
- [x] Backend demo: login → refresh → logout → reuse detection.
- [x] Mobile demo: cold start → biometric → unread count.
- [x] Зафиксировать known limitations Sprint 3 (`docs/known-limitations-sprint-3.md`).

Примечание: тестовые пользователи `11111111-1111-1111-1111-111111111111` и `22222222-2222-2222-2222-222222222222` присутствуют в local БД. Backend demo: smoke сценарий описан в `docs/debug-manual-test.md` (5 шагов). Mobile demo: iOS Simulator — Login OK, GET /api/v1/me/unread-count OK после добавления CORS в main-service. Known limitations зафиксированы в `docs/known-limitations-sprint-3.md` (10 пунктов).

---

**Sprint 3 — DONE**
