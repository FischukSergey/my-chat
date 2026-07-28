# Sprint 6 — детальный план (Production Polish)

Источник: `docs/chat-architecture-plan.md` §7–10 + known-limitations Sprint 1–3 + Sprint 5 deployment scope.

## 1) Цель спринта

Перевести чат из demo-режима в полноценный продакшн (iOS PWA, добавляется на домашний экран через Safari):

- пользователи входят через реальный логин/пароль (не UUID);
- push-уведомления реально доставляются через стандартный Web Push API (VAPID) — Apple Developer Account не нужен;
- WebSocket устойчив к разрывам сети (heartbeat);
- badge на иконке всегда актуален через `navigator.setAppBadge`.

К концу Sprint 6 приложение готово к передаче реальным пользователям: они открывают `https://yourdomain.com` в Safari → «Добавить на экран "Домой"» → PWA готова к использованию.

## 2) Входные условия (что готово после Sprint 4 + 5)

- Все backend-сервисы задеплоены на VPS через HTTPS/WSS.
- Auth: сессии, ротация refresh, биометрия (Face ID через Capacitor/WebAuthn).
- Chat: отправка/чтение сообщений, TTL, WS-события.
- Push: `notification-worker` работает с `noop` / `devlog` провайдером.
- Мобильный клиент: экраны Login, Home, Chat; работает через HTTPS.
- Логин: клиент отправляет UUID напрямую (dev-mode).
- PWA: нет Service Worker, нет `manifest.json`, нет Web Push подписки.

## 3) Ключевые задачи спринта

### A. PWA-инфраструктура клиента

**Текущая проблема:** клиент не является полноценным PWA — нет `manifest.json`, нет Service Worker, браузер не предлагает «добавить на экран». Push-уведомления требуют Service Worker.

**Решение:**

1. Добавить `mobile/public/manifest.json`:
   - `name`, `short_name`, `start_url: "/"`, `display: "standalone"`;
   - `background_color`, `theme_color`;
   - иконки `192x192` и `512x512` (PNG).
2. Добавить `<link rel="manifest" href="/manifest.json">` в `index.html`.
3. Добавить `mobile/public/sw.js` — Service Worker:
   - обработчик события `push` → `showNotification(...)`;
   - обработчик события `notificationclick` → открыть нужный диалог;
   - обработчик `push` с `type: "badge_sync"` → `self.navigator?.setAppBadge(badge)` без уведомления.
4. Зарегистрировать Service Worker в `main.ts`: `navigator.serviceWorker.register('/sw.js')`.
5. Проверить через Chrome DevTools → Application → Manifest: «Add to Homescreen» работает.

---

### B. Web Push провайдер (VAPID)

**Текущая проблема:** `notification-worker` использует `devlog` / `noop` — реальных уведомлений нет (Known Limitation Sprint 2 #1). APNs не нужен — Web Push API абстрагирует доставку для Safari, Chrome, Firefox.

**Решение:**

1. Сгенерировать VAPID-ключи (один раз):
   ```bash
   go run github.com/SherClockHolmes/webpush-go/cmd/vapid@latest
   # или через openssl ecparam -genkey -name prime256v1
   ```
   Результат: `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY` — хранить в `.env`.

2. Добавить зависимость: `go get github.com/SherClockHolmes/webpush-go`.

3. Реализовать `internal/clients/push/webpush.go`:
   - реализует `Provider` интерфейс;
   - `Name() string` → `"webpush"`;
   - `Send(ctx, msg)`:
     - десериализовать `device.PushSubscription` (JSON: `endpoint`, `p256dh`, `auth`);
     - сформировать payload: `{ "title": preview, "body": "Новое сообщение", "badge": N, "dialog_id": ..., "message_id": ... }`;
     - для `EventType == "badge_sync"`: `{ "type": "badge_sync", "badge": N }` — без title/body (silent);
     - `webpush.SendNotification(payload, subscription, options)`.

4. Добавить `WebPushConfig` в `internal/config/config.go`:
   - `VAPIDPrivateKey string`;
   - `VAPIDPublicKey string`;
   - `Subject string` (email или URL, требование VAPID: `mailto:you@example.com`).

5. `NotificationWorkerConfig`: добавить `Provider: "webpush"` | `"devlog"` | `"noop"`.

6. Фабрика провайдера в `App.New`: выбирать провайдер по конфигу.

**Требования:** только VAPID-ключи — генерируются локально, никаких Apple/Google аккаунтов.

---

### C. Обновить таблицу `devices` — хранить PushSubscription

**Текущая проблема:** колонка `push_token TEXT` рассчитана на APNs/FCM токен. Web Push требует хранения всего объекта `PushSubscription` (endpoint + ключи шифрования).

**Решение:**

1. Миграция `011_device_push_subscription.sql`:
   - `ALTER TABLE devices ADD COLUMN IF NOT EXISTS push_subscription JSONB`;
   - старый `push_token` оставить для обратной совместимости (или переименовать).

2. Обновить `DeviceRepository.Register` — принимать и сохранять `push_subscription`.

3. Обновить модель `Device` в `store/models.go`: добавить `PushSubscription string` (JSON-строка).

4. Обновить endpoint `POST /api/v1/devices/register`:
   - принимать `{ "platform": "web", "push_subscription": { "endpoint": "...", "keys": { "p256dh": "...", "auth": "..." } } }`;
   - `platform: "web"` — новое значение платформы.

5. Добавить endpoint `GET /api/v1/push/vapid-public-key` (публичный):
   - возвращает `{ "public_key": "..." }`;
   - клиент запрашивает его при инициализации PWA для `subscribe()`.

---

### D. Реальная аутентификация (логин/пароль)

**Текущая проблема:** `POST /api/v1/auth/login` принимает `user_id` (UUID) без пароля — это dev-режим (Known Limitation Sprint 1, Sprint 3 #6).

**Решение:**

1. Миграция `010_user_credentials.sql`:
   - добавить `username TEXT UNIQUE NOT NULL` в `users`;
   - добавить `password_hash TEXT NOT NULL` в `users`.

2. Endpoint `POST /api/v1/users/register` (новый):
   - принимает `{ "username": "...", "password": "..." }`;
   - сохраняет `bcrypt(password, cost=12)`;
   - возвращает `user_id`.

3. Изменить `POST /api/v1/auth/login`:
   - принимать `{ "username": "...", "password": "..." }`;
   - `bcrypt.CompareHashAndPassword`;
   - сохранить логику session/JWT без изменений.

4. Обновить PWA-клиент:
   - экран Login: поля `username` + `password` вместо UUID;
   - добавить экран Register.

**Зависимости:** `golang.org/x/crypto/bcrypt`.

---

### E. WebSocket heartbeat (ping/pong)

**Текущая проблема:** нет обнаружения «мёртвых» соединений (Known Limitation Sprint 1 #2).

**Решение:**

1. В `internal/hub/conn.go`:
   - тикер 30 сек: `conn.WriteMessage(websocket.PingMessage, nil)`;
   - `SetPongHandler` — обновлять `ReadDeadline` при получении pong;
   - `ReadDeadline = now + 60 сек`;
   - при ошибке или таймауте — закрыть соединение, unregister из Hub.

2. Safari PWA отвечает на ping автоматически (нативный WS в браузере).

3. Добавить unit-тест: мёртвое соединение корректно закрывается.

---

### F. Актуальный badge

**Текущая проблема:** badge рассчитывается в момент постановки задачи в outbox, а не в момент отправки (Known Limitation Sprint 2 #4).

**Решение:** В `notification-worker`, перед `provider.Send`, делать актуальный `SELECT COUNT(*) ... WHERE unread` из БД и передавать в `Message.Badge`.

---

### G. Silent push для badge-sync между устройствами

**Текущая проблема:** прочтение на одном устройстве не обновляет badge на других (Known Limitation Sprint 2 #7).

**Решение:**

1. При `mark_read` — ставить в outbox silent push-задачу (`type: "badge_sync"`) на все остальные активные устройства читателя.
2. В Web Push провайдере: если `EventType == "badge_sync"` — payload без `title`/`body`, только `{ "type": "badge_sync", "badge": N }`.
3. В Service Worker (PWA): обработать `push` с `type == "badge_sync"` → `self.navigator?.setAppBadge(data.badge)` без показа уведомления.

---

### H. Device binding для refresh-токена

**Текущая проблема:** refresh-токен не привязан к конкретному устройству (Known Limitation Sprint 3 #2).

**Решение:**

1. `POST /api/v1/auth/login` — принимать опциональный заголовок `X-Device-ID`.
2. Сохранять `device_id` в `auth_sessions`.
3. `POST /api/v1/auth/refresh` — если сессия имеет `device_id`, сверять с `X-Device-ID` → `403 device_mismatch`.
4. PWA-клиент: хранить `device_id` в `localStorage`, отправлять в каждом auth-запросе.

---

## 4) Что не входит в Sprint 6

- Android-платформа.
- Нативное iOS-приложение в App Store (используем PWA).
- Apple Developer Account / APNs (не нужны для Web Push).
- Kubernetes / Helm.
- Групповые чаты.
- E2E-шифрование.

## 5) Зависимости и предварительные шаги

| Что нужно | Откуда взять |
|-----------|-------------|
| VAPID ключи | Генерируются один раз локально (`webpush-go` CLI или openssl) |
| HTTPS домен | Уже сделано в Sprint 5 |
| iOS 16.4+ на устройстве тестировщика | Web Push работает только с iOS 16.4+ |

Внешние аккаунты не требуются.

## 6) Технические решения (библиотеки)

| Задача | Библиотека |
|--------|-----------|
| Web Push | `github.com/SherClockHolmes/webpush-go` |
| bcrypt | `golang.org/x/crypto/bcrypt` |
| PWA manifest | Статический JSON-файл |
| Service Worker | Нативный браузерный API |

## 7) Definition of Done Sprint 6

- Регистрация и вход через username/password работают в PWA.
- Push-уведомление приходит на iPhone (iOS 16.4+) когда PWA добавлена на домашний экран.
- Badge на иконке обновляется при прочтении, включая другие устройства.
- WS автоматически закрывает мёртвые соединения (heartbeat).
- `docker compose ps` на VPS → все сервисы `healthy`.
- `task lint` и `task test` проходят без ошибок.
- VAPID ключи не попадают в git.

## 8) Риски и меры

| Риск | Мера |
|------|------|
| Web Push не работает в Safari без добавления на домашний экран | Показывать пользователю баннер «Добавьте на экран» при открытии в браузере |
| iOS < 16.4 не поддерживает Web Push | Проверять поддержку: `'PushManager' in window`; показывать fallback |
| VAPID ключи попадают в git | `.gitignore` для `.env`; только `.env.example` с плейсхолдерами |
| Push endpoint устарел (пользователь переустановил браузер) | Обрабатывать HTTP 404/410 от push-сервера → деактивировать подписку в БД |
| `navigator.setAppBadge` не поддерживается на iOS <16.4 | Проверять `'setAppBadge' in navigator` перед вызовом |
