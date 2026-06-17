# Sprint 2 — детальный план (уведомления + badge)

Источник: `docs/chat-architecture-plan.md`, `docs/sprint-1-plan.md`, `docs/sprint-1-checklist.md`, `docs/known-limitations-sprint-1.md`.

## 1) Цель спринта

Довести backend до состояния, когда офлайн-получатель получает push-уведомление о новом сообщении, а счетчик непрочитанных сообщений (badge) синхронизирован и является серверным source of truth.

К концу Sprint 2 должно быть:
- API регистрации устройства для push;
- рабочий `notification-worker` (минимум с dev-провайдером и очередью задач);
- серверный расчет и обновление badge;
- событие `badge_updated` в realtime;
- воспроизводимый e2e-сценарий `send while offline -> push queued/sent -> unread/badge updated`.

## 2) Входные условия (что уже готово после Sprint 1)

- Реализован базовый chat-flow: `login -> send -> receive -> read -> unread`.
- Есть `main-service`, `auth-proxy`, `ws` и debug UI `/debug`.
- В проекте уже добавлены бинари `notification-worker` и `message-expirer`, но это bootstrap без бизнес-логики.
- Зафиксированы ограничения Sprint 1:
  - нет push/APNs/FCM;
  - нет device registry;
  - нет синхронизации badge;
  - нет heartbeat/reconnect-recovery для WS.

## 3) Границы Sprint 2

### Входит в Sprint 2

- Backend-реализация push-канала и badge-синхронизации.
- Device API и хранилище девайсов.
- Внутренний pipeline отправки уведомлений через `notification-worker`.
- Debug-инструменты для ручной проверки push/badge сценариев.
- Unit/integration тесты на новые сценарии.

### Не входит в Sprint 2

- Полная production-интеграция с APNs/FCM в реальном окружении (секреты/сертификаты/реальные мобильные токены).
- Биометрия и hardening сессий (план Sprint 3).
- TTL/`message_deleted` и `message-expirer` функциональность (план Sprint 4).
- Полный observability-stack (Prometheus/OpenTelemetry), кроме минимально необходимых метрик/логов для push.

## 4) Sprint backlog (детализация задач)

## A. Модель данных и миграции

- Добавить таблицу `devices`:
  - `id uuid pk`;
  - `user_id uuid not null`;
  - `platform text not null` (`ios`, `android`, `web`);
  - `push_token text not null`;
  - `enabled bool not null default true`;
  - `last_seen_at timestamptz`;
  - уникальность `(user_id, platform, push_token)`.
- Добавить таблицу `notification_outbox`:
  - `id uuid pk`;
  - `event_type text not null` (`message_new`);
  - `user_id uuid not null`;
  - `payload jsonb not null`;
  - `attempt int not null default 0`;
  - `status text not null` (`pending`, `sent`, `failed`);
  - `next_attempt_at timestamptz not null default now()`;
  - `last_error text null`;
  - `created_at/updated_at`.
- Добавить индексы:
  - `notification_outbox(status, next_attempt_at)`;
  - `devices(user_id, enabled)`.

## B. Device API в `main-service`

- Реализовать `POST /api/v1/devices/register`.
  - Вход: `platform`, `push_token`.
  - Поведение: upsert устройства и обновление `last_seen_at`.
- Реализовать `POST /api/v1/devices/unregister`.
  - Вход: `platform`, `push_token`.
  - Поведение: мягкое отключение (`enabled=false`) вместо физического удаления.
- Обновить API-документацию и debug shortcuts для новых ручек.

## C. Outbox-публикация из chat-сервиса

- При `SendMessage`:
  - если получатель offline по WS, писать задачу в `notification_outbox`;
  - если получатель online, push не публиковать (MVP правило Sprint 2).
- Payload outbox содержит:
  - `message_id`, `dialog_id`, `sender_id`, `preview`, `unread_count`.
- Обеспечить идемпотентность публикации:
  - уникальный dedup key (например, по `message_id + receiver_id + event_type`).

## D. Реализация `notification-worker`

- Добавить poll-loop по `notification_outbox`:
  - выборка `pending/failed` с `next_attempt_at <= now()`;
  - обработка батчами.
- Добавить push-provider abstraction:
  - `dev-log provider` (обязательный для local/test);
  - `noop/fake provider` для unit-тестов.
- Реализовать retry policy:
  - exponential backoff;
  - ограничение числа попыток (например, 5);
  - `failed` после исчерпания попыток.
- Логировать каждый `push_attempt` в структурированном виде.

## E. Badge и realtime синхронизация

- Зафиксировать правило: backend — единственный источник истины для unread/badge.
- При успешном read:
  - пересчитать unread count;
  - отправить `badge_updated` через WS активным сессиям пользователя.
- При успешной отправке push:
  - включать актуальный `badge` в payload.
- Добавить fallback endpoint для ручной синхронизации (если нужен): `GET /api/v1/me/unread-count` уже используется как re-sync.

## F. Local/dev инфраструктура

- Обновить `deploy/local/docker-compose.local.yml`:
  - добавить `notification-worker` сервис;
  - подключить конфиг и переменные для dev provider.
- Проверить локальный сценарий с поднятыми:
  - `postgres`;
  - `main-service`;
  - `auth-proxy`;
  - `notification-worker`.

## G. Тестирование и качество

- Unit tests:
  - device repository + handlers;
  - outbox publisher;
  - retry/backoff логика worker.
- Integration tests:
  - offline recipient -> outbox task created;
  - worker consumes task -> status becomes `sent`;
  - read -> `unread_count`/`badge_updated` consistency.
- Обновить manual debug сценарий для двух вкладок + имитации offline.

## 5) Разбивка по дням (ориентир на 10 рабочих дней)

### День 1
- Утвердить контракты Device API, outbox payload и `badge_updated`.
- Подготовить SQL-миграции `devices` и `notification_outbox`.

### День 2
- Реализовать репозитории `devices` и `notification_outbox`.
- Добавить базовые unit-тесты репозиториев.

### День 3
- Реализовать `POST /api/v1/devices/register`.
- Реализовать `POST /api/v1/devices/unregister`.

### День 4
- Интегрировать outbox-публикацию в `SendMessage` для offline получателя.
- Добавить покрытие тестами по offline/online веткам.

### День 5
- Реализовать poll-loop в `notification-worker`.
- Добавить dev provider + `noop` provider.

### День 6
- Реализовать retry/backoff и статусы `pending/sent/failed`.
- Добавить структурированные логи попыток отправки.

### День 7
- Реализовать `badge_updated` при `read`.
- Проверить консистентность `unread_count` и payload badge.

### День 8
- Обновить `/debug`:
  - шорткаты device register/unregister;
  - визуализация outbox/push результатов (минимально через лог).
- Подготовить/обновить ручной test-runbook.

### День 9
- Написать integration e2e для push/badge сценария.
- Стабилизировать ошибки, валидацию и контракты.

### День 10
- Буфер на фиксы и техдолг.
- Freeze и демо Sprint 2.

## 6) Definition of Done (DoD) для Sprint 2

Спринт считается завершенным, если:
- пользователь может зарегистрировать устройство через API;
- при offline-получателе создается outbox-задача и обрабатывается worker;
- push-пайплайн проходит минимум через dev-provider без ручных правок БД;
- unread/badge корректно синхронизируются после `read`;
- `task lint` и `task test` проходят стабильно;
- документация и ручной debug сценарий обновлены.

## 7) Демо-сценарий Sprint 2

1. Поднять local окружение с `notification-worker`.
2. Пользователь B регистрирует device token.
3. Отключить WS у B (симуляция offline).
4. Пользователь A отправляет сообщение B.
5. Проверить, что задача появилась в outbox и обработана worker.
6. Проверить, что в push payload присутствует актуальный badge.
7. Вернуть B online, отметить сообщение как read.
8. Проверить `unread_count=0` и получение `badge_updated`.

## 8) Риски Sprint 2 и меры

- Риск: невозможно полноценно валидировать APNs/FCM в local.
  - Мера: dev-provider + контрактные тесты payload, интеграция с реальными провайдерами выносится в stage.

- Риск: дубли push при retry/повторной публикации.
  - Мера: dedup key, идемпотентные операции и явные статусы outbox.

- Риск: рассинхрон badge между WS и push.
  - Мера: единая функция пересчета unread на сервере и периодический re-sync через `GET /api/v1/me/unread-count`.

- Риск: рост сложности без наблюдаемости.
  - Мера: минимум метрик/логов по outbox и попыткам push уже в рамках спринта.

## 9) Артефакты по итогам спринта

- миграции `devices` и `notification_outbox`;
- Device API (`register`/`unregister`);
- рабочий `notification-worker` (dev provider + retries);
- outbox-интеграция при offline доставке;
- событие `badge_updated` и синхронизация unread/badge;
- обновленные docs и интеграционные тесты.
