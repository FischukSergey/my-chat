# Sprint 2 Checklist

Источник: `docs/sprint-2-plan.md`.

## 1) Подготовка и контракты

- [x] Утвердить контракты Device API (`register`, `unregister`).
- [x] Утвердить формат `notification_outbox` payload.
- [x] Утвердить событие WebSocket `badge_updated`.
- [x] Утвердить правила offline/online доставки для push (когда создавать outbox-задачу).

Примечание: контракты Sprint 2 зафиксированы в `docs/api-sprint-2.md`.

## 2) База данных и миграции

- [x] Создать миграцию таблицы `devices`.
- [x] Добавить уникальность `(user_id, platform, push_token)` в `devices`.
- [x] Создать миграцию таблицы `notification_outbox`.
- [x] Добавить индексы для обработки outbox (`status`, `next_attempt_at`).
- [x] Проверить миграции на чистой БД и при повторном запуске.

Примечание: добавлены `internal/store/migrations/005_devices.sql` и `internal/store/migrations/006_notification_outbox.sql`. Идемпотентность проверена двукратным прогоном всех миграций на временном PostgreSQL контейнере.

## 3) Репозитории и store-слой

- [ ] Реализовать `DeviceRepository` (upsert/register, disable/unregister, list active devices).
- [ ] Реализовать `NotificationOutboxRepository` (enqueue, claim batch, mark sent, mark failed/retry).
- [ ] Добавить дедупликацию outbox-задач (dedup key).
- [ ] Добавить unit-тесты репозиториев `devices` и `notification_outbox`.

## 4) API `main-service` (devices)

- [ ] Реализовать `POST /api/v1/devices/register`.
- [ ] Реализовать `POST /api/v1/devices/unregister`.
- [ ] Добавить DTO и валидацию (`platform`, `push_token`).
- [ ] Подключить auth middleware для новых ручек.
- [ ] Описать ошибки в едином формате (`code`, `message`, `details`).

## 5) Chat-service и outbox публикация

- [ ] При `SendMessage` публиковать задачу в outbox, если получатель offline по WS.
- [ ] Не публиковать push-задачу, если получатель online.
- [ ] Включать в payload: `message_id`, `dialog_id`, `sender_id`, `preview`, `unread_count`.
- [ ] Добавить unit-тесты offline/online веток публикации.

## 6) `notification-worker`

- [ ] Реализовать polling outbox (`pending/failed`, `next_attempt_at <= now`).
- [ ] Реализовать обработку батчами.
- [ ] Добавить abstraction push-provider.
- [ ] Реализовать `dev-log` provider для local/dev.
- [ ] Реализовать `noop/fake` provider для тестов.
- [ ] Добавить retry policy (exponential backoff, max attempts).
- [ ] Обновлять статусы outbox (`pending` -> `sent` / `failed`).
- [ ] Логировать `push_attempt` в структурированном виде.

## 7) Badge и realtime синхронизация

- [ ] Зафиксировать backend как source of truth для unread/badge.
- [ ] При `read` пересчитывать unread и отправлять `badge_updated` через WS.
- [ ] При push включать актуальный `badge` в payload.
- [ ] Проверить консистентность с `GET /api/v1/me/unread-count`.

## 8) Debug UI и документация

- [ ] Добавить в `/debug` шорткат `devices/register`.
- [ ] Добавить в `/debug` шорткат `devices/unregister`.
- [ ] Отобразить результат push/outbox сценария в debug-логе.
- [ ] Обновить `docs/api-sprint-1.md` или вынести отдельный API-док для Sprint 2.
- [ ] Обновить ручной сценарий проверки (`docs/debug-manual-test.md`) под push/badge flow.

## 9) Локальная инфраструктура

- [ ] Добавить `notification-worker` в `deploy/local/docker-compose.local.yml`.
- [ ] Добавить/проверить локальный конфиг `notification-worker`.
- [ ] Проверить запуск окружения: `postgres + auth-proxy + main-service + notification-worker`.
- [ ] Проверить базовый smoke e2e сценарий в local.

## 10) Тесты и качество

- [ ] Unit-тесты на Device API handlers.
- [ ] Unit-тесты на outbox publisher.
- [ ] Unit-тесты retry/backoff логики worker.
- [ ] Integration-тест: offline recipient -> outbox task created.
- [ ] Integration-тест: worker обрабатывает outbox и помечает задачу `sent`.
- [ ] Integration-тест: `read` синхронизирует unread/badge.
- [ ] Проверить `task fmt`.
- [ ] Проверить `task lint`.
- [ ] Проверить `task test`.

## 11) Критерии готовности (DoD)

- [ ] Устройство регистрируется/отключается через API.
- [ ] Для offline получателя создается outbox-задача.
- [ ] `notification-worker` обрабатывает outbox и завершает отправку.
- [ ] Badge и unread не расходятся после `read`.
- [ ] Debug-сценарий push/badge воспроизводим вручную.
- [ ] Документация Sprint 2 актуализирована.

## 12) Демо

- [ ] Подготовить тестовых пользователей и device token в local.
- [ ] Запустить демонстрацию `send while offline -> outbox -> push -> badge_updated`.
- [ ] Зафиксировать known limitations Sprint 2.
