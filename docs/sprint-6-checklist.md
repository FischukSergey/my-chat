# Sprint 6 Checklist

Источник: `docs/sprint-6-plan.md`.

**Цель спринта:** переход из demo-режима в полноценный продакшн — реальный логин/пароль, Web Push (VAPID) без Apple Developer Account, heartbeat WebSocket, PWA-манифест.

**Предусловия:**
- Sprint 4 завершён (мобильный клиент с экраном чата, GitHub Actions CI).
- Sprint 5 завершён (HTTPS на домене, CD-деплой).
- iOS 16.4+ на тестовом устройстве.
- VAPID-ключи сгенерированы (см. п. 5).

---

## 1) Подготовка и контракты

- [x] Утвердить схему `users` после добавления `username` + `password_hash`.
- [x] Утвердить новый контракт `POST /api/v1/auth/login` (username/password вместо user_id).
- [x] Утвердить контракт `POST /api/v1/users/register`.
- [x] Утвердить формат `PushSubscription` в `POST /api/v1/devices/register` (поле `push_subscription` вместо `push_token`).
- [x] Утвердить новый endpoint `GET /api/v1/push/vapid-public-key`.
- [x] Подготовить `docs/api-sprint-6.md` с обновлёнными контрактами.

Примечание: контракты зафиксированы в `docs/api-sprint-6.md`. Схема `users`: `username` UNIQUE + `password_hash` (bcrypt cost=12). Login — breaking change (`username`/`password`, ошибка `401 invalid_credentials`). Register — `POST /api/v1/users/register` на main-service (`201` + `user_id`, `409 username_taken`). Devices: для `platform=web` обязателен `push_subscription` JSON; `push_token` становится nullable. Добавлены `GET /api/v1/push/vapid-public-key` и device binding через `X-Device-ID` (`403 device_mismatch`).

---

## 2) VAPID-ключи (one-time, выполняется один раз)

- [x] Сгенерировать VAPID ключи:
  ```bash
  # cmd/vapid в webpush-go отсутствует; использовать helper:
  # private, public, err := webpush.GenerateVAPIDKeys()
  # либо: npx web-push generate-vapid-keys --json
  ```
  Результат: `VAPID_PRIVATE_KEY` и `VAPID_PUBLIC_KEY`.
- [x] Добавить в `.env` на VPS: `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT=mailto:you@example.com`.
- [x] Добавить в `.env.example` плейсхолдеры (без реальных значений).
- [x] Убедиться, что `.env` в `.gitignore`.

Примечание: на VPS (`/opt/my-chat/.env`) `VAPID_PRIVATE_KEY` / `VAPID_PUBLIC_KEY` / `VAPID_SUBJECT=mailto:admin@beepru.ru` уже присутствуют — не перезаписывались. В корневом `.env.example` добавлены плейсхолдеры VAPID. `.env` игнорируется (`.gitignore` + `!.env.example`). Пакет `webpush-go/cmd/vapid` не существует — генерация через `webpush.GenerateVAPIDKeys()`.

---

## 3) База данных — миграция credentials

- [x] Создать `internal/store/migrations/010_user_credentials.sql`:
  - `ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT UNIQUE NOT NULL DEFAULT ''`;
  - `ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT ''`;
  - убрать `DEFAULT ''` после seed-данных (или в миграции сразу добавить тестовых пользователей).
- [x] Добавить `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username)`.
- [x] Создать seed-скрипт `deploy/local/seed-users.sql` с тестовыми пользователями (username + bcrypt-hash).
- [x] Проверить идемпотентность миграции (повторный запуск без ошибок).
- [x] Обновить модель `User` в `internal/store/models.go`: добавить `Username string`, `PasswordHash string`.

Примечание: использован partial unique index `WHERE username != ''` — позволяет существующим строкам с `DEFAULT ''` пережить повторную миграцию без конфликта уникальности. Seed: `alice`/`bob` с паролем `password123` (bcrypt cost=12), upsert по `id`. `FindByID` обновлён — читает `username` и `password_hash`. Идемпотентность проверена двойным прогоном. Линтер — 0 issues.

---

## 4) База данных — миграция PushSubscription

- [x] Создать `internal/store/migrations/011_device_push_subscription.sql`:
  - `ALTER TABLE devices ADD COLUMN IF NOT EXISTS push_subscription JSONB`;
  - старый `push_token` оставить (nullable, для возможного будущего использования).
- [x] Проверить идемпотентность миграции.
- [x] Обновить модель `Device` в `store/models.go`: добавить `PushSubscription string`.
- [x] Обновить `DeviceRepository.Register` (upsert) — сохранять `push_subscription`.

Примечание: `push_token` снят NOT NULL через `DO $$` block (идемпотентно). Добавлен partial unique index `devices_web_endpoint_unique` по `(user_id, endpoint) WHERE platform='web'` — позволяет `ON CONFLICT` при upsert web-устройств. `Upsert` разделён на два пути: `upsertToken` (ios/android, conflict по push_token) и `upsertWeb` (web, conflict по endpoint из push_subscription JSON). `scanDevice` и `ListActive` обновлены — используют `COALESCE` для nullable полей. Линтер — 0 issues, сборка чистая.

---

## 5) Registration endpoint

- [x] Добавить зависимость: `go get golang.org/x/crypto`.
- [x] Добавить `POST /api/v1/users/register` в `main-service`:
  - принимает `{ "username": "...", "password": "..." }`;
  - валидация: `username` 3–50 символов (латиница, цифры, `_`), `password` минимум 8 символов;
  - `bcrypt.GenerateFromPassword([]byte(password), 12)`;
  - `INSERT INTO users (id, username, password_hash, status)`;
  - возвращает `{ "user_id": "..." }` (201 Created).
- [x] Обработка конфликта → 409 `username_taken`.
- [x] Добавить handler `internal/handlers/user/handler.go`.
- [x] Добавить service `internal/services/user/service.go`.
- [x] Добавить маршрут в `app.go` (публичный, без auth middleware).
- [x] Unit-тест: success, duplicate username, password too short.

Примечание: `golang.org/x/crypto` повышен из `indirect` до прямой зависимости (`v0.54.0`). `UserRepository.Create` + `ErrUsernameTaken` добавлены в store-слой; helper `isUniqueViolation` (pgcode `23505`) — в `store.go`. Сервис `internal/services/user` содержит regex-валидацию username (`^[a-zA-Z0-9_]{3,50}$`) и bcrypt cost=12. Публичный маршрут зарегистрирован до `router.Group` с auth middleware. 6 unit-тестов (success, 409, password short, invalid username, bad JSON, svc error) — все PASS. `task lint` — 0 issues.

---

## 6) Обновить Login endpoint

- [x] Изменить `POST /api/v1/auth/login`: принимать `{ "username": "...", "password": "..." }`.
- [x] В `auth.Service.Login`:
  - `UserRepository.FindByUsername(ctx, username)`;
  - `bcrypt.CompareHashAndPassword` → `ErrInvalidCredentials` при несовпадении.
- [x] Добавить `UserRepository.FindByUsername` в store-слой.
- [x] Добавить `ErrInvalidCredentials` и маппинг `401 invalid_credentials` в handler.
- [x] Обновить unit-тесты `auth.Service.Login`.
- [x] Обновить integration-тесты login.
- [x] Удалить старый путь `user_id`-логина.
- [x] Убедиться, что rate-limit middleware на login по-прежнему работает.

Примечание: `UserRepository.FindByUsername` добавлен в store-слой. `auth.Service.Login` теперь принимает `(username, password string, deviceID *string)` — находит пользователя по username, проверяет статус, сравнивает bcrypt-хеш; при несоответствии возвращает `ErrInvalidCredentials`. User enumeration защищён: `ErrUserNotFound` маппируется в тот же `ErrInvalidCredentials`. Handler: `loginRequest{Username, Password}`, маппинг `401 invalid_credentials` / `403 user_inactive` / `500 internal`. Старый `user_id`-путь удалён полностью. Rate-limit middleware остался на том же маршруте в `authproxy/app.go` — работает без изменений. Unit-тесты: добавлены `TestLogin_WrongPassword_ReturnsErrInvalidCredentials` и `TestLogin_UserNotFound_ReturnsErrInvalidCredentials`, `TestLogin_InvalidCredentials_Returns401`. Integration-тесты: `insertTestUser` вставляет пользователя с bcrypt-хешем (MinCost), Login вызывается через `username/password`. `task lint` — 0 issues.

---

## 7) Web Push провайдер

- [x] Добавить зависимость: `go get github.com/SherClockHolmes/webpush-go`.
- [x] Создать `internal/clients/push/webpush.go`:
  - реализует `Provider` интерфейс;
  - `Name() string` → `"webpush"`;
  - `Send(ctx, msg)`:
    - десериализовать `device.PushSubscription` (JSON → `webpush.Subscription{}`);
    - если `msg.EventType != "badge_sync"`: payload `{ "title": preview, "body": "Новое сообщение", "badge": N, "dialog_id": ..., "message_id": ... }`;
    - если `msg.EventType == "badge_sync"`: payload `{ "type": "badge_sync", "badge": N }` (silent, без title/body);
    - `webpush.SendNotificationWithContext(payload, &subscription, options)`.
- [x] Добавить `WebPushConfig` в `internal/config/config.go`:
  - `VAPIDPrivateKey string`, `VAPIDPublicKey string`, `Subject string`.
- [x] Добавить `WebPush WebPushConfig` в `NotificationWorkerConfig`.
- [x] Обновить фабрику провайдера в `App.New`: `"webpush"` → инициализировать `push.NewWebPushProvider(cfg)`.
- [x] Обновить конфиг `Provider: "webpush"` | `"dev-log"` | `"noop"` (validate в config.go).
- [x] Обновить `configs/config.notification-worker.prod.yaml`: `provider: webpush`.
- [x] Обработка ошибок HTTP 404/410 от push-сервера → деактивировать подписку в БД.
- [x] Unit-тест: mock HTTP-сервер, проверка корректности payload для alert и badge_sync.

Примечание: `webpush-go v1.4.0` добавлен в `go.mod`. Провайдер в `internal/clients/push/webpush.go`: `buildPayload` разделяет alert и badge_sync; HTTP 404/410 → `ErrSubscriptionGone`; в worker при `errors.Is(err, push.ErrSubscriptionGone)` — `devices.DisableByID` (новый метод в store) + `continue` без retry. `WebPushConfig` с env-тегами `VAPID_*`. Prod-конфиг теперь `provider: webpush` + секции `web_push:` с env-переменными. При отсутствии VAPID-ключей — graceful fallback на `dev-log` с предупреждением. `task lint` — 0 issues, `task test` — все PASS (в т.ч. `internal/clients/push`).

---

## 8) Endpoint VAPID public key + обновление devices/register

- [x] Добавить `GET /api/v1/push/vapid-public-key` (публичный роут, без auth):
  - возвращает `{ "public_key": "<base64url>" }`;
  - значение берётся из `cfg.Notifications.WebPush.VAPIDPublicKey`.
- [x] Обновить `POST /api/v1/devices/register`:
  - принимать `{ "platform": "web", "push_subscription": { "endpoint": "...", "keys": { "p256dh": "...", "auth": "..." } } }`;
  - `platform: "web"` — добавить как допустимое значение.
- [x] Обновить валидацию `platform` в handler: `"ios"` | `"android"` | `"web"`.
- [x] Обновить unit-тест handler: тест регистрации с `platform=web` и `push_subscription`.

Примечание: `internal/handlers/push/handler.go` — новый handler `GET /api/v1/push/vapid-public-key`; ключ берётся из `cfg.NotificationWorker.WebPush.VAPIDPublicKey` (env `VAPID_PUBLIC_KEY`). `device/handler.go`: `registerRequest` получил поле `PushSubscription json.RawMessage`; валидация разветвлена по `platform` — для `web` обязателен `push_subscription`, для `ios`/`android` — `push_token`. `mainservice/app.go`: публичный маршрут `GET /api/v1/push/vapid-public-key` зарегистрирован. Тесты: `TestRegister_Web_WithPushSubscription`, `TestRegister_Web_MissingPushSubscription` добавлены; `TestRegister_TokenPlatformsAccepted` заменил `AllPlatformsAccepted` (web вынесен в отдельный happy-path тест); `TestRegister_ServiceError` переключён на `platform=ios`. `internal/handlers/push/handler_test.go`: `TestVapidPublicKey_ReturnsKey`, `TestVapidPublicKey_EmptyKey`. `task lint` — 0 issues, `task test` — все PASS.

---

## 9) Актуальный badge при отправке push

- [x] В `notification-worker`, перед `provider.Send`: запросить актуальный `unread_count` из БД.
- [x] Добавить `MessageRepository.CountUnread(ctx, userID string) (int, error)` (если не существует).
- [x] Обновить `Message.Badge` актуальным значением перед вызовом провайдера.
- [x] Unit-тест: badge в push = актуальный `unread_count`.

Примечание: `CountUnread` уже существовал на `ReceiptRepository`. В `notification.Worker` добавлен интерфейс `unreadRepository{CountUnread}` и поле `unread`; `NewWorker` принимает его третьим аргументом. В `processTask` перед циклом по устройствам вызывается `w.unread.CountUnread(ctx, task.UserID)` — значение используется как `Badge`; при ошибке — graceful fallback на `payload.UnreadCount` с `log.Warn`. `notificationworker/app.go`: создаётся `receiptRepo` и передаётся в `NewWorker`. Unit-тесты: `TestWorker_BadgeEqualsActualUnreadCount` (badge=7 при реальном unread=7), `TestWorker_BadgeFallsBackToPayloadOnUnreadError` (badge=1 из payload при ошибке). Integration-тест обновлён. `task lint` — 0 issues, `task test` — все PASS.

---

## 10) Silent push для badge-sync между устройствами

- [x] В `chat.Service.MarkRead` после пересчёта unread:
  - `DeviceRepository.ListActive(ctx, userID)` — все активные устройства читателя;
  - для каждого устройства (кроме текущей сессии) ставить в outbox задачу `type: "badge_sync"`.
- [x] В Web Push провайдере: для `badge_sync` — payload без `title`/`body`, TTL = 60 сек (badge не нужен надолго).
- [x] Unit-тест: `mark_read` создаёт `badge_sync` outbox-задачи для всех устройств читателя.

Примечание: Web Push провайдер для `badge_sync` (payload без title/body, TTL=60s) реализован в пункте 7. В `chat.Service.MarkRead` добавлен вызов `s.enqueueBadgeSync(ctx, messageID, userID)` после `sendBadgeUpdated`. `enqueueBadgeSync` — best-effort: публикует одну задачу `event_type=badge_sync` для читателя в outbox; notification-worker распределяет по всем активным устройствам пользователя (аналогично `message_new`). Dedup key `badge_sync:<messageID>:<userID>` исключает дубли при параллельных read-запросах. Актуальный badge worker запрашивает из БД (пункт 9). `TestMarkRead_EnqueuesBadgeSyncForReader` проверяет факт постановки задачи с `event_type=badge_sync` и `user_id=reader`. `task lint` — 0 issues, `task test` — все PASS.

---

## 11) WebSocket heartbeat

- [x] В `internal/hub/conn.go` (или аналоге) реализовать ping/pong:
  - тикер 30 сек: `conn.WriteMessage(websocket.PingMessage, nil)`;
  - `conn.SetPongHandler`: обновлять `ReadDeadline` при получении pong;
  - `conn.SetReadDeadline(now + 60 сек)` при каждом входящем сообщении;
  - при ошибке WriteMessage или таймауте ReadDeadline — закрыть соединение, unregister.
- [x] Unit-тест: мёртвое соединение корректно закрывается.
- [x] Unit-тест: pong получен → соединение остаётся живым.

Примечание: используется `coder/websocket` (не gorilla), поэтому API адаптирован: `conn.Ping(ctx)` вместо `WriteMessage(PingMessage)`; библиотека обрабатывает pong автоматически. Создан `internal/hub/conn.go`: интерфейс `wsConn{Read, Ping}`, функция `RunConn(ctx, conn, cfg)` — запускает heartbeat-горутину (`runHeartbeat`) и read-loop; при ошибке ping отменяет контекст через `WithCancelCause`; `ConnConfig{HeartbeatInterval=30s, PingTimeout=5s}` настраивается для тестов. `ws/handler.go`: read-loop заменён вызовом `hub.RunConn(ctx, rawConn, hub.DefaultConnConfig())`. Тесты: `TestRunConn_DeadConn_ClosesAfterPingFailure` — ping возвращает ошибку → RunConn завершается; `TestRunConn_PongReceived_ConnectionStaysAlive` — ping=nil × 2+, затем Read-ошибка → RunConn завершается. Race detector: OK (pingCount через `atomic.Int64`). `task lint` — 0 issues, `task test` — все PASS.

---

## 12) Device binding для refresh-токена

 - [x] `POST /api/v1/auth/login` — принимать опциональный заголовок `X-Device-ID`.
 - [x] Сохранять в `auth_sessions.device_id` (поле уже есть).
 - [x] `POST /api/v1/auth/refresh` — если сессия имеет `device_id`, сверять с `X-Device-ID` → `403 device_mismatch`.
 - [x] Добавить `ErrDeviceMismatch` и маппинг `403` в handler.
 - [x] Unit-тест: refresh с другого device_id → 403.
 - [x] Unit-тест: refresh с правильным device_id → 200.

Примечание: `ErrDeviceMismatch` добавлен в `internal/services/auth/service.go`. `Refresh` получил новый аргумент `deviceID *string`; если `session.DeviceID != nil` и переданный ID не совпадает (или nil) — возвращается `ErrDeviceMismatch`. `handler.go`: интерфейс обновлён, `deviceIDFromRequest(r)` читает заголовок `X-Device-ID`; для login и refresh передаётся в сервис; `respondAuthError` маппит `ErrDeviceMismatch` → 403 `device_mismatch`. CORS в `mainservice/app.go` и `authproxy/middleware.go` расширен `X-Device-ID`. Тесты: `TestRefresh_DeviceMismatch_ReturnsErrDeviceMismatch`, `TestRefresh_CorrectDeviceID_Succeeds` (service); `TestRefresh_WrongDeviceID_Returns403`, `TestRefresh_CorrectDeviceID_Returns200` (handler). `task lint` — 0 issues, `task test` — все PASS.

---

## 13) PWA-инфраструктура клиента

 - [x] Добавить `mobile/public/manifest.json`:
  ```json
  {
    "name": "MyChat",
    "short_name": "MyChat",
    "start_url": "/",
    "display": "standalone",
    "background_color": "#ffffff",
    "theme_color": "#000000",
    "icons": [
      { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" },
      { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png" }
    ]
  }
  ```
 - [x] Добавить иконки `mobile/public/icons/icon-192.png` и `icon-512.png`.
 - [x] Добавить `<link rel="manifest" href="/manifest.json">` и `<meta name="theme-color">` в `index.html`.
 - [x] Добавить `mobile/public/sw.js` — Service Worker:
  - `push` event → `showNotification()` для обычных уведомлений;
  - `push` event + `type == "badge_sync"` → `self.navigator?.setAppBadge(data.badge)` (без уведомления);
  - `notificationclick` event → открыть нужный диалог (`clients.openWindow`).
 - [x] Зарегистрировать SW в `main.ts`:
  ```ts
  if ('serviceWorker' in navigator) {
    await navigator.serviceWorker.register('/sw.js');
  }
  ```
 - [x] Показывать баннер пользователю если PWA не установлена (проверка `window.matchMedia('(display-mode: standalone)')`).
 - [ ] Проверить в Chrome DevTools → Application → Manifest: статус OK.

Примечание: `vite.config.ts` получил `publicDir: "../public"` — все статические файлы хранятся в `mobile/public/`. `manifest.json`: `theme_color: #007aff` (iOS blue). Иконки `icon-192.png`/`icon-512.png` — сгенерированные PNG 192×192 и 512×512 (синий фон, белая буква M). `sw.js`: `push` event → проверка `event_type === "badge_sync"` → `navigator.setAppBadge(badge)` без уведомления; иначе `showNotification()`; `notificationclick` → `clients.matchAll` + `focus`/`openWindow` + `postMessage({type:"open_dialog",...})`. `index.html`: `<meta name="theme-color">`, `<link rel="manifest">`, HTML-блок баннера `#install-banner` со стилями. `main.ts`: `initPWA()` — `navigator.serviceWorker.register('/sw.js')` + слушатель `beforeinstallprompt`/`appinstalled`; `initInstallBannerButtons()` — кнопки «Установить»/«Закрыть». `tsc --noEmit` — 0 ошибок, `vite build` — OK. Проверка Chrome DevTools — вручную.

---

## 14) Обновление мобильного клиента (PWA)

 - [x] Экран Login: заменить поле `User ID (UUID)` на `Username` + `Password`.
 - [x] Добавить экран Register: `Username`, `Password`, `Confirm Password` → `POST /api/v1/users/register`.
 - [x] После успешной регистрации — автоматический переход на Login.
 - [x] Обновить `mobile/src/api.ts`:
  - `login(username, password)` — изменить тело запроса;
  - добавить `register(username, password)`;
  - добавить `getVapidPublicKey()` → `GET /api/v1/push/vapid-public-key`;
  - добавить `registerDevice(platform, pushSubscription)`.
 - [x] Реализовать подписку на Web Push в `main.ts`.
 - [x] Добавить `X-Device-ID` заголовок в login/refresh запросы (device_id хранить в `localStorage`).
 - [x] Обработать `Notification.requestPermission()` — запросить разрешение перед подпиской.
 - [x] Fallback: если `'PushManager' !in window` (iOS < 16.4) — показывать предупреждение в лог.
 - [x] Проверить `npm run build` — 0 ошибок TypeScript.

Примечание: `api.ts` — `apiLogin(username, password)` + `X-Device-ID` в login/refresh; `apiRegister`; `getVapidPublicKey`; `registerDevice('web', sub.toJSON())`; `getOrCreateDeviceId()` (localStorage `my_chat_device_id`); `extractUserIdFromJwt` — декодирует JWT без проверки подписи, извлекает `user_id`. `index.html` — login: `username-input` + `password-input` + кнопка «Зарегистрироваться»; добавлен экран `#register` с полями reg-username/password/confirm + `input[type=password]` в стилях. `main.ts` — `ScreenName` расширен `"register"`; `handleLogin` читает username+password, сохраняет user_id из JWT; `handleRegister` — POST /api/v1/users/register + redirect на login; `subscribePush()` — проверка PushManager, `Notification.requestPermission()`, `getSubscription()` (skip if exists), `urlBase64ToUint8Array`, `pushManager.subscribe`, `registerDevice`; сохранение username в localStorage; `showLoginScreen` восстанавливает username. `tsc --noEmit` — 0 ошибок, `npm run build` — OK.

---

## 15) Конфиги и инфраструктура

 - [x] Обновить `configs/config.notification-worker.prod.yaml`: `provider: webpush`.
 - [x] Добавить в `.env.example`: `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT`.
 - [x] Обновить `deploy/prod/docker-compose.prod.yml`: передать VAPID env-переменные в `notification-worker` и `main-service` (для endpoint VAPID public key).
 - [ ] Добавить seed-пользователей в prod (один раз): `docker compose exec postgres psql ...`.
 - [x] Убедиться, что `.env` содержит реальные VAPID ключи и не попадает в git (`.gitignore`: `.env`, `.env.*`, `!.env.example`).
 - [x] `task lint` — 0 issues.
 - [x] `task test` — все unit-тесты PASS.
 - [x] `task test:integration` — все integration-тесты PASS.

Примечание: `configs/config.notification-worker.prod.yaml` уже имел `provider: webpush` и VAPID env-теги из предыдущих спринтов. `.env.example` уже содержал VAPID-переменные. `configs/config.main-service.prod.yaml` дополнен секцией `notification_worker.web_push.vapid_public_key: ${VAPID_PUBLIC_KEY}` — для endpoint `GET /api/v1/push/vapid-public-key`. `deploy/prod/docker-compose.prod.yml`: `main-service` получил `environment.VAPID_PUBLIC_KEY`; `notification-worker` — `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT`. `.gitignore` уже содержит `.env` и `.env.*` (кроме `.env.example`). Seed-пользователей нужно добавить вручную через `docker compose exec postgres psql`. `task lint` — 0 issues, `task test` — все PASS.

---

## 16) Тесты и качество ✅

- [x] Unit-тест `UserService.Register`: success, duplicate username, short password.
  — `internal/services/user/service_test.go`: `TestRegister_Success`, `TestRegister_DuplicateUsername_ReturnsErrUsernameTaken`, `TestRegister_ShortPassword_ReturnsErrPasswordTooShort`, `TestRegister_InvalidUsername_ReturnsErrInvalidUsername`.
- [x] Unit-тест `auth.Service.Login`: success, wrong password, user not found.
  — Уже реализовано в Sprint 6 ранее: `TestLogin_ReturnsTokenPairAndCreatesSession`, `TestLogin_WrongPassword_ReturnsErrInvalidCredentials`, `TestLogin_UserNotFound_ReturnsErrInvalidCredentials` (`internal/services/auth/service_test.go`).
- [x] Unit-тест Web Push провайдер: mock HTTP-сервер, корректный payload для alert.
  — `internal/clients/push/webpush_test.go`: `TestBuildPayload_Alert`, `TestWebPushProvider_Send_Success`.
- [x] Unit-тест Web Push провайдер: payload для `badge_sync` (без title/body).
  — `internal/clients/push/webpush_test.go`: `TestBuildPayload_BadgeSync`.
- [x] Unit-тест Web Push: HTTP 410 от сервера → подписка деактивируется.
  — `internal/clients/push/webpush_test.go`: `TestWebPushProvider_Send_SubscriptionGone_on_410`, `TestWebPushProvider_Send_SubscriptionGone_on_404`.
- [x] Unit-тест heartbeat: мёртвое соединение закрывается.
  — `internal/hub/conn_test.go`: `TestRunConn_DeadConn_ClosesAfterPingFailure` (выполнено в пункте 11).
- [x] Unit-тест device binding: refresh с другого device_id → 403.
  — `internal/services/auth/service_test.go`: `TestRefresh_DeviceMismatch_ReturnsErrDeviceMismatch`; `internal/handlers/auth/handler_test.go`: `TestRefresh_WrongDeviceID_Returns403` (выполнено в пункте 12).
- [x] Unit-тест silent push: `mark_read` создаёт badge_sync задачи для всех устройств.
  — `internal/services/chat/service_test.go`: `TestMarkRead_EnqueuesBadgeSyncForReader` (выполнено в пункте 10).
- [x] Integration-тест: register → login → register_device (web) → send → webpush отправляется.
  — `internal/services/notification/integration_test.go`: `TestIntegration_RegisterLoginDeviceWeb_WebPushSent` — создаёт user+password, web-device с push_subscription, outbox-задачу, прогоняет worker с NoopProvider-трекером, проверяет вызов Send для platform=web.
- [x] Проверить `task fmt`, `task lint`, `task test`, `task test:integration`.
  — `task fmt` — OK; `task lint` — 0 issues; `task test` — all PASS; `task test:integration` — all PASS.

---

## 17) Smoke-тесты на prod (реальный iPhone, iOS 16.4+)

- [ ] Открыть `https://beepru.ru` в Safari → «Добавить на экран "Домой"».
- [ ] Открыть PWA с домашнего экрана → разрешить push-уведомления.
- [ ] **Регистрация**: ввести username/password → успешно → войти.
- [ ] **Push**: с другого аккаунта/устройства отправить сообщение → push-уведомление приходит на iPhone.
- [ ] **Badge**: на иконке PWA появляется число непрочитанных.
- [ ] **Badge sync**: прочитать сообщение на одном устройстве → badge на другом сбросился.
- [ ] **Heartbeat**: выключить WiFi на 90 сек → включить → WS reconnect без зависших соединений на сервере.
- [ ] **Security**: неверный пароль → 401; >10 попыток → 429.
- [ ] **Device binding**: использовать refresh-токен с другого устройства → 403.
- [ ] `docker compose ps` на VPS → все сервисы `healthy`.

---

## 18) Критерии готовности (DoD)

- [ ] PWA добавляется на домашний экран iOS через Safari.
- [ ] Push-уведомление приходит на iPhone (iOS 16.4+) когда PWA на домашнем экране.
- [ ] Логин через username/password работает.
- [ ] Badge на иконке актуален, обновляется при прочтении на другом устройстве.
- [ ] WS heartbeat: мёртвые соединения автоматически закрываются.
- [ ] `task lint` — 0 issues, `task test` — все PASS.
- [ ] VAPID ключи не попадают в git.
- [ ] Документация Sprint 6 актуализирована.

---

## 19) Демо

- [ ] Открыть `https://beepru.ru` в Safari → добавить на домашний экран.
- [ ] Зарегистрироваться → войти → разрешить push.
- [ ] Свернуть PWA → с другого аккаунта отправить сообщение → push-уведомление на iPhone.
- [ ] Тапнуть на уведомление → открывается нужный диалог.
- [ ] Прочитать сообщение → badge на иконке обнулился.
- [ ] Показать WS heartbeat в логах: `ping sent`, `pong received`.
- [ ] Показать GitHub Actions: зелёный CD → автодеплой на VPS.
- [ ] Зафиксировать known limitations Sprint 6 (`docs/known-limitations-sprint-6.md`).

---

**Sprint 6 — PLANNED**
