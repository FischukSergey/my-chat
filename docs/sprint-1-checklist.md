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

- [x] Реализовать `GET /ws/connect`.
- [x] Реализовать реестр активных подключений по `user_id`.
- [x] Отправлять событие `message_new` получателю.
- [x] Отправлять событие `message_delivered` отправителю.
- [x] Отправлять событие `message_read` отправителю.
- [x] Добавить корректное закрытие соединений и cleanup.

## 7) Debug web client

- [x] Поддержать health-check на странице `/debug`.
- [x] Поддержать произвольные HTTP-запросы на странице `/debug`.
- [x] Поддержать подключение/отправку/чтение WebSocket на странице `/debug`.
- [x] Добавить шорткаты для auth/send/read/unread в debug UI.
- [x] Подготовить ручной сценарий проверки в docs.

## 8) Конфигурация и локальный запуск

- [x] Проверить все `configs/config.*.local.example.yaml`.
- [x] Проверить запуск `main-service` с `-config`.
- [x] Обновить `deploy/local/docker-compose.local.yml` для текущего сценария Sprint 1.
- [x] Проверить локальный запуск end-to-end с PostgreSQL.

## 9) Тесты и качество

- [x] Написать unit-тесты для сервисов сообщений.
- [x] Написать unit-тесты для auth (MVP).
- [x] Написать integration-тест `login -> send -> list -> read -> unread`.
- [x] Проверить `task fmt`.
- [x] Проверить `task lint` (docker).
- [x] Проверить `task test`.

## 10) Критерии готовности (DoD)

- [x] Сценарий `login -> send -> receive -> read -> unread` проходит end-to-end.
- [x] Realtime работает для двух пользователей одновременно.
- [x] Ключевые сценарии воспроизводимы через `/debug`.
- [x] Документация актуализирована.

## 11) Демо

- [x] Подготовить 2 тестовых пользователя.
- [x] Запустить демонстрацию через `/debug` в двух вкладках.
- [x] Зафиксировать known limitations Sprint 1.
