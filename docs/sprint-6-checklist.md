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

- [ ] Утвердить схему `users` после добавления `username` + `password_hash`.
- [ ] Утвердить новый контракт `POST /api/v1/auth/login` (username/password вместо user_id).
- [ ] Утвердить контракт `POST /api/v1/users/register`.
- [ ] Утвердить формат `PushSubscription` в `POST /api/v1/devices/register` (поле `push_subscription` вместо `push_token`).
- [ ] Утвердить новый endpoint `GET /api/v1/push/vapid-public-key`.
- [ ] Подготовить `docs/api-sprint-6.md` с обновлёнными контрактами.

---

## 2) VAPID-ключи (one-time, выполняется один раз)

- [ ] Сгенерировать VAPID ключи:
  ```bash
  go run github.com/SherClockHolmes/webpush-go/cmd/vapid@latest
  ```
  Результат: `VAPID_PRIVATE_KEY` и `VAPID_PUBLIC_KEY`.
- [ ] Добавить в `.env` на VPS: `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT=mailto:you@example.com`.
- [ ] Добавить в `.env.example` плейсхолдеры (без реальных значений).
- [ ] Убедиться, что `.env` в `.gitignore`.

---

## 3) База данных — миграция credentials

- [ ] Создать `internal/store/migrations/010_user_credentials.sql`:
  - `ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT UNIQUE NOT NULL DEFAULT ''`;
  - `ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT ''`;
  - убрать `DEFAULT ''` после seed-данных (или в миграции сразу добавить тестовых пользователей).
- [ ] Добавить `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username)`.
- [ ] Создать seed-скрипт `deploy/local/seed-users.sql` с тестовыми пользователями (username + bcrypt-hash).
- [ ] Проверить идемпотентность миграции (повторный запуск без ошибок).
- [ ] Обновить модель `User` в `internal/store/models.go`: добавить `Username string`, `PasswordHash string`.

---

## 4) База данных — миграция PushSubscription

- [ ] Создать `internal/store/migrations/011_device_push_subscription.sql`:
  - `ALTER TABLE devices ADD COLUMN IF NOT EXISTS push_subscription JSONB`;
  - старый `push_token` оставить (nullable, для возможного будущего использования).
- [ ] Проверить идемпотентность миграции.
- [ ] Обновить модель `Device` в `store/models.go`: добавить `PushSubscription string`.
- [ ] Обновить `DeviceRepository.Register` (upsert) — сохранять `push_subscription`.

---

## 5) Registration endpoint

- [ ] Добавить зависимость: `go get golang.org/x/crypto`.
- [ ] Добавить `POST /api/v1/users/register` в `main-service`:
  - принимает `{ "username": "...", "password": "..." }`;
  - валидация: `username` 3–50 символов (латиница, цифры, `_`), `password` минимум 8 символов;
  - `bcrypt.GenerateFromPassword([]byte(password), 12)`;
  - `INSERT INTO users (id, username, password_hash, status)`;
  - возвращает `{ "user_id": "..." }` (201 Created).
- [ ] Обработка конфликта → 409 `username_taken`.
- [ ] Добавить handler `internal/handlers/user/handler.go`.
- [ ] Добавить service `internal/services/user/service.go`.
- [ ] Добавить маршрут в `app.go` (публичный, без auth middleware).
- [ ] Unit-тест: success, duplicate username, password too short.

---

## 6) Обновить Login endpoint

- [ ] Изменить `POST /api/v1/auth/login`: принимать `{ "username": "...", "password": "..." }`.
- [ ] В `auth.Service.Login`:
  - `UserRepository.FindByUsername(ctx, username)`;
  - `bcrypt.CompareHashAndPassword` → `ErrInvalidCredentials` при несовпадении.
- [ ] Добавить `UserRepository.FindByUsername` в store-слой.
- [ ] Добавить `ErrInvalidCredentials` и маппинг `401 invalid_credentials` в handler.
- [ ] Обновить unit-тесты `auth.Service.Login`.
- [ ] Обновить integration-тесты login.
- [ ] Удалить старый путь `user_id`-логина.
- [ ] Убедиться, что rate-limit middleware на login по-прежнему работает.

---

## 7) Web Push провайдер

- [ ] Добавить зависимость: `go get github.com/SherClockHolmes/webpush-go`.
- [ ] Создать `internal/clients/push/webpush.go`:
  - реализует `Provider` интерфейс;
  - `Name() string` → `"webpush"`;
  - `Send(ctx, msg)`:
    - десериализовать `device.PushSubscription` (JSON → `webpush.Subscription{}`);
    - если `msg.EventType != "badge_sync"`: payload `{ "title": preview, "body": "Новое сообщение", "badge": N, "dialog_id": ..., "message_id": ... }`;
    - если `msg.EventType == "badge_sync"`: payload `{ "type": "badge_sync", "badge": N }` (silent, без title/body);
    - `webpush.SendNotification(payload, &subscription, options)`.
- [ ] Добавить `WebPushConfig` в `internal/config/config.go`:
  - `VAPIDPrivateKey string`, `VAPIDPublicKey string`, `Subject string`.
- [ ] Добавить `WebPush WebPushConfig` в `NotificationWorkerConfig`.
- [ ] Обновить фабрику провайдера в `App.New`: `"webpush"` → инициализировать `push.NewWebPushProvider(cfg)`.
- [ ] Обновить конфиг `Provider: "webpush"` | `"devlog"` | `"noop"`.
- [ ] Обновить `configs/config.notification-worker.prod.yaml`: `provider: webpush`.
- [ ] Обработка ошибок HTTP 404/410 от push-сервера → деактивировать подписку в БД.
- [ ] Unit-тест: mock HTTP-сервер, проверка корректности payload для alert и badge_sync.

---

## 8) Endpoint VAPID public key + обновление devices/register

- [ ] Добавить `GET /api/v1/push/vapid-public-key` (публичный роут, без auth):
  - возвращает `{ "public_key": "<base64url>" }`;
  - значение берётся из `cfg.Notifications.WebPush.VAPIDPublicKey`.
- [ ] Обновить `POST /api/v1/devices/register`:
  - принимать `{ "platform": "web", "push_subscription": { "endpoint": "...", "keys": { "p256dh": "...", "auth": "..." } } }`;
  - `platform: "web"` — добавить как допустимое значение.
- [ ] Обновить валидацию `platform` в handler: `"ios"` | `"android"` | `"web"`.
- [ ] Обновить unit-тест handler: тест регистрации с `platform=web` и `push_subscription`.

---

## 9) Актуальный badge при отправке push

- [ ] В `notification-worker`, перед `provider.Send`: запросить актуальный `unread_count` из БД.
- [ ] Добавить `MessageRepository.CountUnread(ctx, userID string) (int, error)` (если не существует).
- [ ] Обновить `Message.Badge` актуальным значением перед вызовом провайдера.
- [ ] Unit-тест: badge в push = актуальный `unread_count`.

---

## 10) Silent push для badge-sync между устройствами

- [ ] В `chat.Service.MarkRead` после пересчёта unread:
  - `DeviceRepository.ListActive(ctx, userID)` — все активные устройства читателя;
  - для каждого устройства (кроме текущей сессии) ставить в outbox задачу `type: "badge_sync"`.
- [ ] В Web Push провайдере: для `badge_sync` — payload без `title`/`body`, TTL = 60 сек (badge не нужен надолго).
- [ ] Unit-тест: `mark_read` создаёт `badge_sync` outbox-задачи для всех устройств читателя.

---

## 11) WebSocket heartbeat

- [ ] В `internal/hub/conn.go` (или аналоге) реализовать ping/pong:
  - тикер 30 сек: `conn.WriteMessage(websocket.PingMessage, nil)`;
  - `conn.SetPongHandler`: обновлять `ReadDeadline` при получении pong;
  - `conn.SetReadDeadline(now + 60 сек)` при каждом входящем сообщении;
  - при ошибке WriteMessage или таймауте ReadDeadline — закрыть соединение, unregister.
- [ ] Unit-тест: мёртвое соединение корректно закрывается.
- [ ] Unit-тест: pong получен → соединение остаётся живым.

---

## 12) Device binding для refresh-токена

- [ ] `POST /api/v1/auth/login` — принимать опциональный заголовок `X-Device-ID`.
- [ ] Сохранять в `auth_sessions.device_id` (поле уже есть).
- [ ] `POST /api/v1/auth/refresh` — если сессия имеет `device_id`, сверять с `X-Device-ID` → `403 device_mismatch`.
- [ ] Добавить `ErrDeviceMismatch` и маппинг `403` в handler.
- [ ] Unit-тест: refresh с другого device_id → 403.
- [ ] Unit-тест: refresh с правильным device_id → 200.

---

## 13) PWA-инфраструктура клиента

- [ ] Добавить `mobile/public/manifest.json`:
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
- [ ] Добавить иконки `mobile/public/icons/icon-192.png` и `icon-512.png`.
- [ ] Добавить `<link rel="manifest" href="/manifest.json">` и `<meta name="theme-color">` в `index.html`.
- [ ] Добавить `mobile/public/sw.js` — Service Worker:
  - `push` event → `showNotification()` для обычных уведомлений;
  - `push` event + `type == "badge_sync"` → `self.navigator?.setAppBadge(data.badge)` (без уведомления);
  - `notificationclick` event → открыть нужный диалог (`clients.openWindow`).
- [ ] Зарегистрировать SW в `main.ts`:
  ```ts
  if ('serviceWorker' in navigator) {
    await navigator.serviceWorker.register('/sw.js');
  }
  ```
- [ ] Показывать баннер пользователю если PWA не установлена (проверка `window.matchMedia('(display-mode: standalone)')`).
- [ ] Проверить в Chrome DevTools → Application → Manifest: статус OK.

---

## 14) Обновление мобильного клиента (PWA)

- [ ] Экран Login: заменить поле `User ID (UUID)` на `Username` + `Password`.
- [ ] Добавить экран Register: `Username`, `Password`, `Confirm Password` → `POST /api/v1/users/register`.
- [ ] После успешной регистрации — автоматический переход на Login.
- [ ] Обновить `mobile/src/api.ts`:
  - `login(username, password)` — изменить тело запроса;
  - добавить `register(username, password)`;
  - добавить `getVapidPublicKey()` → `GET /api/v1/push/vapid-public-key`;
  - добавить `registerDevice(platform, pushSubscription)`.
- [ ] Реализовать подписку на Web Push в `main.ts`:
  ```ts
  const reg = await navigator.serviceWorker.ready;
  const vapidKey = await api.getVapidPublicKey();
  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(vapidKey)
  });
  await api.registerDevice('web', sub.toJSON());
  ```
- [ ] Добавить `X-Device-ID` заголовок в login/refresh запросы (device_id хранить в `localStorage`).
- [ ] Обработать `PushNotification.requestPermission()` — запросить разрешение перед подпиской.
- [ ] Fallback: если `'PushManager' !in window` (iOS < 16.4) — показывать предупреждение.
- [ ] Проверить `npm run build` — 0 ошибок TypeScript.

---

## 15) Конфиги и инфраструктура

- [ ] Обновить `configs/config.notification-worker.prod.yaml`: `provider: webpush`.
- [ ] Добавить в `deploy/prod/.env.example`: `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT`.
- [ ] Обновить `deploy/prod/docker-compose.prod.yml`: передать VAPID env-переменные в `notification-worker` и `main-service` (для endpoint VAPID public key).
- [ ] Добавить seed-пользователей в prod (один раз): `docker compose exec postgres psql ...`.
- [ ] Убедиться, что `.env` содержит реальные VAPID ключи и не попадает в git.
- [ ] `task lint` — 0 issues.
- [ ] `task test` — все unit-тесты PASS.
- [ ] `task test:integration` — все integration-тесты PASS.

---

## 16) Тесты и качество

- [ ] Unit-тест `UserService.Register`: success, duplicate username, short password.
- [ ] Unit-тест `auth.Service.Login`: success, wrong password, user not found.
- [ ] Unit-тест Web Push провайдер: mock HTTP-сервер, корректный payload для alert.
- [ ] Unit-тест Web Push провайдер: payload для `badge_sync` (без title/body).
- [ ] Unit-тест Web Push: HTTP 410 от сервера → подписка деактивируется.
- [ ] Unit-тест heartbeat: мёртвое соединение закрывается.
- [ ] Unit-тест device binding: refresh с другого device_id → 403.
- [ ] Unit-тест silent push: `mark_read` создаёт badge_sync задачи для всех устройств.
- [ ] Integration-тест: register → login → register_device (web) → send → webpush отправляется.
- [ ] Проверить `task fmt`, `task lint`, `task test`, `task test:integration`.

---

## 17) Smoke-тесты на prod (реальный iPhone, iOS 16.4+)

- [ ] Открыть `https://yourdomain.com` в Safari → «Добавить на экран "Домой"».
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

- [ ] Открыть `https://yourdomain.com` в Safari → добавить на домашний экран.
- [ ] Зарегистрироваться → войти → разрешить push.
- [ ] Свернуть PWA → с другого аккаунта отправить сообщение → push-уведомление на iPhone.
- [ ] Тапнуть на уведомление → открывается нужный диалог.
- [ ] Прочитать сообщение → badge на иконке обнулился.
- [ ] Показать WS heartbeat в логах: `ping sent`, `pong received`.
- [ ] Показать GitHub Actions: зелёный CD → автодеплой на VPS.
- [ ] Зафиксировать known limitations Sprint 6 (`docs/known-limitations-sprint-6.md`).

---

**Sprint 6 — PLANNED**
