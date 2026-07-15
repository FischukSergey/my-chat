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

- [x] Реализовать `DeviceRepository` (upsert/register, disable/unregister, list active devices).
- [x] Реализовать `NotificationOutboxRepository` (enqueue, claim batch, mark sent, mark failed/retry).
- [x] Добавить дедупликацию outbox-задач (dedup key).
- [x] Добавить unit-тесты репозиториев `devices` и `notification_outbox`.

Примечание: `DeviceRepository` — `internal/store/device_repository.go`; `NotificationOutboxRepository` — `internal/store/notification_outbox_repository.go`. Модели `Device`, `NotificationOutbox` добавлены в `internal/store/models.go`. Дедупликация реализована через `ON CONFLICT (dedup_key) DO NOTHING`. Тесты — `internal/store/repositories_integration_test.go` (build tag `integration`).

## 4) API `main-service` (devices)

- [x] Реализовать `POST /api/v1/devices/register`.
- [x] Реализовать `POST /api/v1/devices/unregister`.
- [x] Добавить DTO и валидацию (`platform`, `push_token`).
- [x] Подключить auth middleware для новых ручек.
- [x] Описать ошибки в едином формате (`code`, `message`, `details`).

Примечание: `internal/handlers/device/handler.go`, `internal/services/device/service.go`. Маршруты зарегистрированы в auth-группе `app/mainservice/app.go`. Формат ошибок расширен полем `details` согласно `docs/api-sprint-2.md`.

## 5) Chat-service и outbox публикация

- [x] При `SendMessage` публиковать задачу в outbox, если получатель offline по WS.
- [x] Не публиковать push-задачу, если получатель online.
- [x] Включать в payload: `message_id`, `dialog_id`, `sender_id`, `preview`, `unread_count`.
- [x] Добавить unit-тесты offline/online веток публикации.

Примечание: `outboxPublisher` интерфейс добавлен в `services/chat/service.go`. При `receiverOnline == false` вызывается `enqueueOutbox` — строит payload согласно `docs/api-sprint-2.md`, dedup_key: `message_new:<message_id>:<receiver_id>`. `BuildPreview` нормализует переносы строк и обрезает до 120 рун. Тесты: `TestSendMessage_ReceiverOffline_EnqueuesOutbox`, `TestSendMessage_ReceiverOnline_NoOutbox`, `TestBuildPreview_*`.

## 6) `notification-worker`

- [x] Реализовать polling outbox (`pending/failed`, `next_attempt_at <= now`).
- [x] Реализовать обработку батчами.
- [x] Добавить abstraction push-provider.
- [x] Реализовать `dev-log` provider для local/dev.
- [x] Реализовать `noop/fake` provider для тестов.
- [x] Добавить retry policy (exponential backoff, max attempts).
- [x] Обновлять статусы outbox (`pending` -> `sent` / `failed`).
- [x] Логировать `push_attempt` в структурированном виде.

Примечание: push-provider abstraction — `internal/clients/push/` (`provider.go`, `devlog.go`, `noop.go`). Worker с poll-loop, retry/backoff и структурными логами — `internal/services/notification/worker.go`; unit-тесты — `internal/services/notification/worker_test.go`. Bootstrap с подключением к БД и выбором provider — `internal/app/notificationworker/app.go`. Конфигурация worker — `NotificationWorkerConfig` в `internal/config/config.go`; пример — `configs/config.notification-worker.local.example.yaml`. Валидация `JWTConfig` ослаблена до `omitempty`, обязательность проверяется вручную в `App.New` каждого сервиса.

## 7) Badge и realtime синхронизация

- [x] Зафиксировать backend как source of truth для unread/badge.
- [x] При `read` пересчитывать unread и отправлять `badge_updated` через WS.
- [x] При push включать актуальный `badge` в payload.
- [x] Проверить консистентность с `GET /api/v1/me/unread-count`.

Примечание: `MarkRead` в `internal/services/chat/service.go` после записи в БД вызывает `CountUnread` и отправляет читателю `badge_updated` (формат по `docs/api-sprint-2.md` §7) — best-effort, сбой подсчёта не откатывает `MarkRead`. В `internal/clients/push/provider.go` добавлено явное поле `Badge int` (равно `UnreadCount` в Sprint 2); заполняется в `internal/services/notification/worker.go` и логируется в `internal/clients/push/devlog.go`. Тест `TestMarkRead_SendsBadgeUpdatedToReader` и обновлённый `TestMarkRead_NotifiesSender` — в `internal/services/chat/service_test.go`.

## 8) Debug UI и документация

- [x] Добавить в `/debug` шорткат `devices/register`.
- [x] Добавить в `/debug` шорткат `devices/unregister`.
- [x] Отобразить результат push/outbox сценария в debug-логе.
- [x] Обновить `docs/api-sprint-1.md` или вынести отдельный API-док для Sprint 2.
- [x] Обновить ручной сценарий проверки (`docs/debug-manual-test.md`) под push/badge flow.

Примечание: в `internal/handlers/debug/handler.go` добавлены шорткаты **Device register** и **Device unregister** (выбор платформы, ввод `push_token`, кнопки `POST devices/register` / `POST devices/unregister`). Результаты отображаются в JS-логе debug UI. В `docs/api-sprint-2.md` добавлен раздел §9 с описанием WS-события `badge_updated`. В `docs/debug-manual-test.md` добавлен сценарий Sprint 2 (register device → send offline → worker push → reconnect → mark read → badge_updated).

## 9) Локальная инфраструктура

- [x] Добавить `notification-worker` в `deploy/local/docker-compose.local.yml`.
- [x] Добавить/проверить локальный конфиг `notification-worker`.
- [x] Проверить запуск окружения: `postgres + auth-proxy + main-service + notification-worker`.
- [x] Проверить базовый smoke e2e сценарий в local.

Примечание: `task local:up` поднимает 5 контейнеров (postgres, auth-proxy, main-service, notification-worker, message-expirer), все переходят в `Healthy`. Исправлен баг двойного `json.Marshal` в `notification_outbox_repository.go:Enqueue` (payload хранился как base64-строка вместо JSON-объекта). Исправлены Dockerfile-ы: убран явный `GOARCH=amd64` (причина SIGSEGV в Go 1.24 + Docker Desktop for Mac с ядром &lt;5.11). Smoke e2e пройден: login → send → unread=1 → mark read → unread=0 → device register → send (outbox) → worker delivery (dev-log лог показывает `dev_push_sent`) → device unregister.

## 10) Тесты и качество

- [x] Unit-тесты на Device API handlers.
- [x] Unit-тесты на outbox publisher.
- [x] Unit-тесты retry/backoff логики worker.
- [x] Integration-тест: offline recipient -> outbox task created.
- [x] Integration-тест: worker обрабатывает outbox и помечает задачу `sent`.
- [x] Integration-тест: `read` синхронизирует unread/badge.
- [x] Проверить `task fmt`.
- [x] Проверить `task lint`.
- [x] Проверить `task test`.

Примечание: добавлены `internal/handlers/device/handler_test.go` (13 unit-тестов — Register/Unregister: success, 401, bad JSON, invalid platform, empty/long token, service error). Добавлена публичная утилита `middleware.ContextWithUserID` для инжекции userID в тестах. В `internal/services/chat/integration_test.go` добавлены `TestIntegration_OfflineRecipient_OutboxTaskCreated` (проверяет, что задача попадает в БД) и `TestIntegration_ReadSynchronizesUnreadBadge` (3 сообщения → unread=3 → mark read одного → unread=2, идемпотентность). Добавлен `internal/services/notification/integration_test.go` с `TestIntegration_WorkerProcessesOutbox_MarksSent` и `TestIntegration_WorkerProcessesOutbox_NoDevices_MarksSent`. Исправлен `store/migrate.go`: добавлена защита от параллельного запуска через `pg_advisory_lock`. Все тесты (unit + integration) зелёные, lint — 0 issues.

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
