# Sprint 1 Checklist

Источник: `docs/sprint-1-plan.md`.

## 1) Подготовка

- [x] Утвердить auth-стратегию MVP (локальный JWT или dev-stub).
- [x] Зафиксировать контракт событий WebSocket (`message_new`, `message_delivered`, `message_read`).
- [x] Утвердить структуру ошибок API (`code`, `message`, `details`).

Примечание: контракты Sprint 1 зафиксированы в `docs/api-sprint-1.md`.

## 2) База данных и миграции

- [x] Создать миграцию `users`.
- [x] Создать миграцию `dialogs` (уникальная пара пользователей).
- [x] Создать миграцию `messages`.
- [x] Создать миграцию `message_receipts`.
- [x] Реализовать инициализацию PostgreSQL в `internal/store`.
- [x] Реализовать запуск миграций при старте `main-service` (local/dev).

## 3) Репозитории и сервисы

- [x] Реализовать repository для `dialogs`.
- [x] Реализовать repository для `messages`.
- [x] Реализовать repository для `message_receipts`.
- [x] Реализовать сервисный слой для отправки/чтения сообщений.
- [x] Реализовать вычисление `unread_count`.

## 4) Auth (MVP)

- [x] Реализовать `POST /api/v1/auth/login`.
- [x] Реализовать `POST /api/v1/auth/refresh`.
- [x] Реализовать `POST /api/v1/auth/logout`.
- [x] Добавить middleware извлечения `user_id` из токена.
- [x] Добавить базовую валидацию auth-запросов.

## 5) HTTP API чата

- [x] Реализовать `POST /api/v1/dialogs/{id}/messages`.
- [x] Реализовать `GET /api/v1/dialogs/{id}/messages`.
- [x] Реализовать `POST /api/v1/messages/{id}/read`.
- [x] Реализовать `GET /api/v1/me/unread-count`.
- [x] Добавить DTO и валидацию входных параметров.

## 6) WebSocket realtime

- [ ] Реализовать `GET /ws/connect`.
- [ ] Реализовать реестр активных подключений по `user_id`.
- [ ] Отправлять событие `message_new` получателю.
- [ ] Отправлять событие `message_delivered` отправителю.
- [ ] Отправлять событие `message_read` отправителю.
- [ ] Добавить корректное закрытие соединений и cleanup.

## 7) Debug web client

- [ ] Поддержать health-check на странице `/debug`.
- [ ] Поддержать произвольные HTTP-запросы на странице `/debug`.
- [ ] Поддержать подключение/отправку/чтение WebSocket на странице `/debug`.
- [ ] Добавить шорткаты для auth/send/read/unread в debug UI.
- [ ] Подготовить ручной сценарий проверки в docs.

## 8) Конфигурация и локальный запуск

- [ ] Проверить все `configs/config.*.local.example.yaml`.
- [ ] Проверить запуск `main-service` с `-config`.
- [ ] Обновить `deploy/local/docker-compose.local.yml` для текущего сценария Sprint 1.
- [ ] Проверить локальный запуск end-to-end с PostgreSQL.

## 9) Тесты и качество

- [ ] Написать unit-тесты для сервисов сообщений.
- [ ] Написать unit-тесты для auth (MVP).
- [ ] Написать integration-тест `login -> send -> list -> read -> unread`.
- [ ] Проверить `task fmt`.
- [ ] Проверить `task lint` (docker).
- [ ] Проверить `task test`.

## 10) Критерии готовности (DoD)

- [ ] Сценарий `login -> send -> receive -> read -> unread` проходит end-to-end.
- [ ] Realtime работает для двух пользователей одновременно.
- [ ] Ключевые сценарии воспроизводимы через `/debug`.
- [ ] Документация актуализирована.

## 11) Демо

- [ ] Подготовить 2 тестовых пользователя.
- [ ] Запустить демонстрацию через `/debug` в двух вкладках.
- [ ] Зафиксировать known limitations Sprint 1.
