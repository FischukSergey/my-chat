# Sprint 1 — детальный план (MVP foundation)

Источник: базовый план из `docs/chat-architecture-plan.md`.

## 1) Цель спринта

Собрать минимально рабочий backend-контур чата для двоих и получить воспроизводимую отладку через браузерный debug-клиент без мобильного приложения.

К концу спринта должно быть:
- рабочий `main-service` с базовым API;
- базовые сценарии auth (MVP, без production-hardening);
- вертикальный срез: `login -> send message -> receive message -> mark read -> unread count`;
- debug-страница `/debug` для ручной проверки HTTP/WS сценариев.

## 2) Границы Sprint 1

### Входит в Sprint 1
- каркас сервисов и конфигурации (уже начат);
- модель БД для MVP (`users`, `dialogs`, `messages`, `message_receipts`);
- HTTP endpoints для login/refresh/logout и сообщений (MVP);
- WebSocket endpoint для realtime-доставки;
- базовая обработка статусов сообщений;
- базовая observability (`/health`, логирование, простые метрики при необходимости);
- ручная верификация через `/debug`.

### Не входит в Sprint 1
- push/APNs/FCM;
- badge на иконке мобильного приложения;
- Face ID;
- полноценная TTL-очистка и `message-expirer` (это Sprint 2+);
- сложные RBAC-политики и production-grade security hardening.

## 3) Sprint backlog (детализация задач)

## A. Данные и хранилище
- Спроектировать SQL миграции:
  - `users`;
  - `dialogs` (уникальная пара пользователей);
  - `messages` (поля времени/статуса);
  - `message_receipts` (`delivered_at`, `read_at`).
- Добавить пакет `internal/store`:
  - инициализация подключения к PostgreSQL;
  - запуск миграций при старте `main-service` (для local/dev);
  - репозитории:
    - `chat` (создание/чтение сообщений),
    - `user` (MVP lookup),
    - `dialog` (поиск/создание диалога).

## B. Auth (MVP)
- Определить временную auth-стратегию:
  - вариант 1: локальный JWT issuer внутри `auth-proxy`;
  - вариант 2: stub-авторизация для dev (фиксированные пользователи).
- Реализовать endpoints:
  - `POST /api/v1/auth/login`;
  - `POST /api/v1/auth/refresh`;
  - `POST /api/v1/auth/logout`.
- Подключить middleware извлечения user-id из токена для защищенных endpoint'ов.

## C. Chat API (HTTP)
- Реализовать:
  - `GET /api/v1/dialogs/{id}/messages`;
  - `POST /api/v1/dialogs/{id}/messages`;
  - `POST /api/v1/messages/{id}/read`;
  - `GET /api/v1/me/unread-count`.
- Контракты DTO:
  - request/response структуры;
  - единая схема ошибок (`code`, `message`, `details`).
- Валидация входных данных:
  - limit/offset/before;
  - длина текста сообщения;
  - корректность UUID.

## D. Realtime (WebSocket)
- Реализовать `GET /ws/connect`.
- Добавить реестр подключений:
  - map `user_id -> соединения`;
  - безопасный доступ (mutex).
- События MVP:
  - `message_new`;
  - `message_delivered` (минимальный вариант);
  - `message_read`.
- Обеспечить корректный disconnect/reconnect для local сценариев.

## E. Debug web client
- Расширить `/debug` под сценарии Sprint 1:
  - заготовки запросов auth;
  - кнопки-шорткаты на send/read/unread;
  - явный лог входящих WS-событий.
- Подготовить test-scripts (ручной сценарий) в README раздел.

## F. Конфиг, запуск, DX
- Уточнить конфиги для local:
  - `main-service`;
  - `auth-proxy`.
- Обновить `deploy/local/docker-compose.local.yml`:
  - PostgreSQL;
  - `main-service`;
  - (опционально) `auth-proxy`.
- Убедиться, что команды стабильны:
  - `task fmt`;
  - `task lint` (docker);
  - `task test`;
  - `task build`.

## G. Тестирование
- Unit tests:
  - сервисы чата;
  - валидация DTO;
  - генерация/проверка auth токенов.
- Integration tests (минимум):
  - login -> send -> list -> read -> unread.
- Smoke tests через `/debug`.

## 4) Разбивка по дням (ориентир на 10 рабочих дней)

### День 1
- зафиксировать MVP auth-подход;
- финализировать SQL схему таблиц;
- подготовить миграции.

### День 2
- подключение БД + запуск миграций;
- базовые репозитории `dialogs/messages`.

### День 3
- endpoints auth: login/refresh/logout (минимум);
- middleware user context.

### День 4
- `POST /dialogs/{id}/messages`;
- `GET /dialogs/{id}/messages`.

### День 5
- `POST /messages/{id}/read`;
- `GET /me/unread-count`.

### День 6
- `GET /ws/connect`;
- отправка `message_new` при создании сообщения.

### День 7
- `message_delivered` и `message_read` события;
- обработка reconnect базового уровня.

### День 8
- расширение `/debug` под шорткаты Sprint 1;
- ручной прогон основных сценариев.

### День 9
- unit + integration тесты;
- стабилизация ошибок/контрактов.

### День 10
- технический долг, фиксы;
- демо-подготовка и freeze спринта.

## 5) Definition of Done (DoD) для Sprint 1

Спринт считается завершенным, если:
- сценарий `login -> send -> receive -> read -> unread` проходит end-to-end;
- WS доставка работает минимум для двух одновременно подключенных пользователей;
- все ключевые сценарии повторяются через `/debug`;
- `task lint` и `task test` проходят стабильно;
- задокументированы API-контракты и ограничения MVP.

## 6) Демо-сценарий (для проверки результата)

1. Запуск local окружения (PostgreSQL + сервисы).
2. Два пользователя логинятся (user A / user B).
3. Оба открывают WS.
4. A отправляет сообщение B.
5. B получает `message_new` в realtime.
6. B отмечает сообщение прочитанным.
7. A видит событие `message_read`.
8. Проверяется `unread_count` у обоих.
9. Повтор сценария через `/debug` (без мобильного клиента).

## 7) Риски Sprint 1 и меры

- Риск: затяжка на auth.
  - Мера: сначала dev-friendly auth (простая, но рабочая), потом hardening.

- Риск: рассинхрон HTTP и WS контрактов.
  - Мера: единые DTO и типы событий, договоренные заранее.

- Риск: сложность отладки realtime.
  - Мера: `/debug` + структурированные логи по connection/user/message.

- Риск: нестабильные миграции.
  - Мера: отдельный smoke-тест миграций в CI и локально.

## 8) Артефакты по итогам спринта

- миграции БД и репозитории store;
- минимальный auth flow;
- chat HTTP API + WS endpoint;
- рабочий debug web client;
- набор автотестов MVP-сценария;
- обновленная документация API/сценариев.
