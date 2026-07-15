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

- [ ] Добавить claim `session_id` в access и refresh tokens.
- [ ] Реализовать `IssueAccessWithSession` / `IssueRefreshWithSession`.
- [ ] Реализовать парсинг `session_id` из токена.
- [ ] Добавить unit-тесты JWT с session claims.

## 5) Auth service (`internal/services/auth/`)

- [ ] Реализовать `Login` — создание session + выдача token pair.
- [ ] Реализовать `Refresh` — validation, rotation, новая пара.
- [ ] Реализовать `Logout` — revoke session по refresh token.
- [ ] Реализовать reuse detection в `Refresh`.
- [ ] Опционально: проверка `users.status = active` при login.
- [ ] Добавить structured audit logs: `auth_login`, `auth_refresh`, `auth_logout`, `auth_reuse_detected`.
- [ ] Добавить unit-тесты auth service.

## 6) Auth handlers и `auth-proxy`

- [ ] Рефакторинг `internal/handlers/auth/handler.go` — делегирование в auth service.
- [ ] Обновить error codes: `session_expired`, `session_revoked`, `session_compromised`.
- [ ] Подключить PostgreSQL в `internal/app/authproxy/app.go`.
- [ ] Запуск `Migrate()` при старте auth-proxy.
- [ ] Обновить `configs/config.auth-proxy.local.example.yaml` (секция database).
- [ ] Добавить unit-тесты auth handlers.

## 7) Локальная инфраструктура

- [ ] Обновить `deploy/local/docker-compose.local.yml`: auth-proxy `depends_on: postgres`.
- [ ] Проверить запуск: `postgres + auth-proxy + main-service + notification-worker`.
- [ ] Smoke: login через auth-proxy создаёт запись в `auth_sessions`.

## 8) Debug UI

- [ ] Сохранять `refresh_token` в debug UI (localStorage).
- [ ] Добавить шорткат **Refresh** (`POST /api/v1/auth/refresh`).
- [ ] Добавить шорткат **Logout** (`POST /api/v1/auth/logout`).
- [ ] Добавить шорткат **Reuse test** (повторный refresh со старым token).
- [ ] Опционально: auto-refresh access при 401.
- [ ] Обновить `docs/debug-manual-test.md` — auth-сценарии Sprint 3.

## 9) Мобильный клиент (Capacitor)

- [ ] Scaffold проекта в `mobile/` (Ionic/Capacitor или React/Capacitor).
- [ ] Экран Login — dev login по `user_id`.
- [ ] Secure storage wrapper для refresh token (Keychain / Keystore).
- [ ] Biometric plugin — gate перед чтением refresh.
- [ ] HTTP client с auto-refresh при 401 / exp access.
- [ ] Cold start flow: biometric → refresh → API call.
- [ ] Logout — wipe secure storage + server revoke.
- [ ] Обработка `session_compromised` → wipe + redirect login.
- [ ] Обработка смены биометрии → wipe + redirect login.
- [ ] Smoke: `GET /api/v1/me/unread-count` после unlock.
- [ ] README в `mobile/` с инструкцией сборки.

## 10) Тесты и качество

- [ ] Unit-тесты: auth service (login, refresh, logout, reuse).
- [ ] Unit-тесты: auth handlers.
- [ ] Integration-тест: login → refresh → old refresh invalid.
- [ ] Integration-тест: logout → refresh fails.
- [ ] Integration-тест: reuse detection → family revoked.
- [ ] Проверить `task fmt`.
- [ ] Проверить `task lint`.
- [ ] Проверить `task test`.
- [ ] Проверить `task test:integration`.

## 11) Критерии готовности (DoD)

- [ ] Refresh ротируется при каждом обновлении; старый token invalid.
- [ ] Logout revoke session на сервере.
- [ ] Reuse detection работает и логируется.
- [ ] Debug UI воспроизводит полный auth lifecycle.
- [ ] Мобильный клиент: secure storage + biometric unlock на симуляторе.
- [ ] Cold start → biometric → refresh → API call успешен.
- [ ] Документация Sprint 3 актуализирована.

## 12) Демо

- [ ] Подготовить тестовых пользователей в local.
- [ ] Backend demo: login → refresh → logout → reuse detection.
- [ ] Mobile demo: cold start → biometric → unread count.
- [ ] Зафиксировать known limitations Sprint 3 (`docs/known-limitations-sprint-3.md`).

---

**Sprint 3 — IN PROGRESS**
