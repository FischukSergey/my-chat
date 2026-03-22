# Sprint 1 Checklist

Источник: `docs/sprint-1-plan.md`.

## 1) Подготовка

- [ ] Утвердить auth-стратегию MVP (локальный JWT или dev-stub).
- [ ] Зафиксировать контракт событий WebSocket (`message_new`, `message_delivered`, `message_read`).
- [ ] Утвердить структуру ошибок API (`code`, `message`, `details`).

## 2) База данных и миграции

- [ ] Создать миграцию `users`.
- [ ] Создать миграцию `dialogs` (уникальная пара пользователей).
- [ ] Создать миграцию `messages`.
- [ ] Создать миграцию `message_receipts`.
- [ ] Реализовать инициализацию PostgreSQL в `internal/store`.
- [ ] Реализовать запуск миграций при старте `main-service` (local/dev).

## 3) Репозитории и сервисы

- [ ] Реализовать repository для `dialogs`.
- [ ] Реализовать repository для `messages`.
- [ ] Реализовать repository для `message_receipts`.
- [ ] Реализовать сервисный слой для отправки/чтения сообщений.
- [ ] Реализовать вычисление `unread_count`.

## 4) Auth (MVP)

- [ ] Реализовать `POST /api/v1/auth/login`.
- [ ] Реализовать `POST /api/v1/auth/refresh`.
- [ ] Реализовать `POST /api/v1/auth/logout`.
- [ ] Добавить middleware извлечения `user_id` из токена.
- [ ] Добавить базовую валидацию auth-запросов.

## 5) HTTP API чата

- [ ] Реализовать `POST /api/v1/dialogs/{id}/messages`.
- [ ] Реализовать `GET /api/v1/dialogs/{id}/messages`.
- [ ] Реализовать `POST /api/v1/messages/{id}/read`.
- [ ] Реализовать `GET /api/v1/me/unread-count`.
- [ ] Добавить DTO и валидацию входных параметров.

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
