# Ручной сценарий проверки Sprint 1

Сценарий: `login → connect WS → send → receive → read → unread`.

Требования: запущен `main-service` и `auth-proxy`, доступен PostgreSQL.
Откройте `/debug` в **двух вкладках** браузера (условно — вкладка **A** и вкладка **B**).

---

## 0) Предварительная настройка

В обеих вкладках убедитесь что поля указывают на корректные адреса:
- **Base URL HTTP** — `http://localhost:8080` (main-service)
- **Auth URL** — `http://localhost:33081` (auth-proxy)

### Тестовые данные (предзагружены в БД)

| | ID |
|---|---|
| User A | `11111111-1111-1111-1111-111111111111` |
| User B | `22222222-2222-2222-2222-222222222222` |
| Dialog | `dddddddd-dddd-dddd-dddd-dddddddddddd` |

Если данных нет (свежая БД), выполните:

```sql
INSERT INTO users (id) VALUES
  ('11111111-1111-1111-1111-111111111111'),
  ('22222222-2222-2222-2222-222222222222')
ON CONFLICT DO NOTHING;

INSERT INTO dialogs (id, user_a_id, user_b_id)
VALUES (
  'dddddddd-dddd-dddd-dddd-dddddddddddd',
  '11111111-1111-1111-1111-111111111111',
  '22222222-2222-2222-2222-222222222222'
) ON CONFLICT DO NOTHING;
```

---

## 1) Login

### Вкладка A

1. В разделе **4) Шорткаты → Login** введите `user_id` первого пользователя,
   например `11111111-1111-1111-1111-111111111111`.
2. Нажмите **Login (сохранить токен)**.
3. В логе появится строка `AUTH token saved`, поле **Access token** в разделе 1 заполнится автоматически.

### Вкладка B

Повторите шаги 1–3 для второго пользователя,
например `22222222-2222-2222-2222-222222222222`.

---

## 2) Подключение WebSocket

### Обе вкладки (по очереди)

1. В разделе **3) WebSocket** URL уже заполнен (`ws://localhost:8080/ws/connect`).
2. Нажмите **Подключить WS**.
3. Токен из поля Access token автоматически добавится как `?token=...` в URL при подключении.
4. В логе появится `WS connected`, статус изменится на `ws: connected`.

---

## 3) Отправка сообщения

### Вкладка A

1. В разделе **4) Шорткаты → Send message** введите `dialog_id` диалога между двумя пользователями.
2. Введите текст сообщения в поле **body**, например `hello`.
3. Нажмите **POST send message**.
4. В логе вкладки A появится `SEND -> 201 {...}` с идентификатором созданного сообщения.
   Поле **message_id** в шорткате Mark read заполнится автоматически.

---

## 4) Получение события message_new / message_delivered

### Вкладка B

В логе должно появиться WS-событие вида:

```json
{
  "event": "message_new",
  "data": {
    "message_id": "...",
    "dialog_id": "...",
    "sender_id": "11111111-1111-1111-1111-111111111111",
    "body": "hello",
    "created_at": "..."
  },
  "ts": "..."
}
```

### Вкладка A

Если вкладка B была онлайн в момент отправки, в логе вкладки A появится событие `message_delivered`:

```json
{
  "event": "message_delivered",
  "data": {
    "message_id": "...",
    "dialog_id": "...",
    "user_id": "22222222-2222-2222-2222-222222222222",
    "delivered_at": "..."
  },
  "ts": "..."
}
```

---

## 5) Прочтение сообщения

### Вкладка B

1. В разделе **4) Шорткаты → Mark read** поле `message_id` заполнено (скопируйте из лога если нет).
2. Нажмите **POST mark read**.
3. В логе появится `READ -> 204`.

### Вкладка A

В логе должно появиться WS-событие:

```json
{
  "event": "message_read",
  "data": {
    "message_id": "...",
    "dialog_id": "...",
    "user_id": "22222222-2222-2222-2222-222222222222",
    "read_at": "..."
  },
  "ts": "..."
}
```

---

## 6) Проверка счётчика непрочитанных

### Вкладка B (до прочтения)

Нажмите **GET /me/unread-count** — ожидается `{"unread_count": 1}` или больше.

### Вкладка B (после прочтения)

Нажмите ещё раз — ожидается `{"unread_count": 0}`.

---

---

# Ручной сценарий проверки Sprint 2 — push & badge flow

Сценарий: `register device → send message (offline) → worker sends push → read → badge_updated WS`.

Дополнительное требование: запущен `notification-worker`.

---

## Sprint 2 — 0) Регистрация устройства (Device Register)

### Вкладка B (получатель, пользователь 22222222-…)

1. Убедитесь, что вы залогинены (токен сохранён).
2. В разделе **4) Шорткаты → Device register**:
   - **platform** — выберите `ios` (или `android` / `web`).
   - **push_token** — введите `fake-push-token-local`.
3. Нажмите **POST devices/register**.
4. В логе появится `DEVICE register -> 201 {"device_id": "..."}`.

---

## Sprint 2 — 1) Отправка сообщения без активного WS (offline сценарий)

### Вкладка B

1. Отключите WS: нажмите **Отключить WS** (или закройте вкладку и откройте новую без подключения).

### Вкладка A (отправитель)

1. В **Send message** введите `dialog_id` и текст сообщения, нажмите **POST send message**.
2. В логе вкладки A появится `SEND -> 201 {...}`.

### Лог notification-worker (консоль)

Так как вкладка B offline, в `main-service` создана запись в `notification_outbox`.  
Через несколько секунд `notification-worker` обработает задачу:

```
INF processed outbox task task_id=... user_id=22222222-... platform=ios push_token=fake-push-token-local
```

(Точный формат зависит от реализации; при использовании `dev-log` provider строка выводится в stdout.)

---

## Sprint 2 — 2) Проверка badge_updated через WS

### Вкладка B (снова подключитесь к WS)

1. Нажмите **Подключить WS**.
2. Прочитайте отправленное сообщение: **POST mark read**.
3. В логе WS вкладки B **должно появиться** событие `badge_updated`:

```json
{
  "event": "badge_updated",
  "data": {
    "user_id": "22222222-2222-2222-2222-222222222222",
    "unread_count": 0,
    "badge": 0,
    "reason": "message_read"
  },
  "ts": "..."
}
```

4. Параллельно в логе вкладки A появится `message_read`.

---

## Sprint 2 — 3) Удаление устройства (Device Unregister)

### Вкладка B

1. В разделе **4) Шорткаты → Device unregister** убедитесь, что `platform` и `push_token` заполнены теми же значениями.
2. Нажмите **POST devices/unregister**.
3. В логе появится `DEVICE unregister -> 204 (ok)`.

---

## Sprint 2 — Ожидаемый итог

| Шаг | Вкладка | Ожидание |
|-----|---------|----------|
| Device register | B | `DEVICE register -> 201` в логе |
| Send (B offline) | A | `SEND -> 201`; outbox-задача создана |
| Worker отправляет push | — | строка в логах notification-worker |
| WS reconnect + mark read | B | `badge_updated` в WS-логе |
| message_read | A | WS-событие в логе |
| Device unregister | B | `DEVICE unregister -> 204` |

---

## Ожидаемый итог

| Шаг | Вкладка | Ожидание |
|-----|---------|----------|
| Login | A, B | `token saved` в логе |
| WS connect | A, B | `ws: connected` |
| Send message | A | `SEND -> 201`, `message_id` заполнен |
| Receive message_new | B | WS-событие в логе |
| Receive message_delivered | A | WS-событие в логе (если B онлайн) |
| Mark read | B | `READ -> 204` |
| Receive message_read | A | WS-событие в логе |
| Unread count | B | `0` после прочтения |

 ---

---

# Ручной сценарий проверки Sprint 3 — auth lifecycle

Сценарий: `login → refresh → reuse detection → logout`.

Требования: запущены `auth-proxy` и `main-service`, доступен PostgreSQL.

---

## Sprint 3 — 0) Предварительная настройка

Убедитесь что в разделе **1) Базовые настройки** указаны корректные адреса:
- **Base URL HTTP** — `http://localhost:8080` (main-service)
- **Auth URL** — `http://localhost:33081` (auth-proxy)

Токены автоматически сохраняются в `localStorage` при каждом Login / Refresh.

---

## Sprint 3 — 1) Login

1. В разделе **4) Шорткаты — Auth → Login** введите `user_id`:
   `11111111-1111-1111-1111-111111111111`
2. Нажмите **Login (сохранить токен)**.
3. В логе появится:
   ```
   AUTH login 11111111-... -> 200 {"access_token":"...","refresh_token":"...","session_id":"..."}
   AUTH access + refresh token сохранены (localStorage)
   ```
4. Поле **Access token** в разделе 1 заполнится автоматически.
5. Поле **Refresh token** заполнится автоматически.
6. **Session ID** в разделе 1 отобразит первые 8 символов session_id.

---

## Sprint 3 — 2) Refresh (ротация сессии)

1. Нажмите **Refresh** в разделе **4) Шорткаты — Auth**.
2. В логе появится:
   ```
   AUTH refresh -> 200 {"access_token":"...","refresh_token":"...","session_id":"..."}
   AUTH новые токены сохранены; старый refresh готов для reuse test
   ```
3. Access token и Session ID обновятся на новые значения.
4. Бейдж **Reuse test** изменится на `есть старый токен`.

---

## Sprint 3 — 3) Reuse test (обнаружение повторного использования)

> После шага 2 у вас есть `prevRefreshToken` — токен, который уже был ротирован.

1. Нажмите **Reuse test** в разделе **4) Шорткаты — Auth**.
2. Ожидаемый результат в логе:
   ```
   AUTH reuse test: повторная отправка старого refresh-токена...
   AUTH reuse test -> 401 {"error":{"code":"session_compromised","message":"..."}}
   AUTH ✓ session_compromised — reuse detection сработал, family отозвана
   AUTH токены очищены (family revoked)
   ```
3. Все токены очищаются — вся session family отозвана на сервере.
4. Проверка в БД:
   ```sql
   SELECT id, revoked_at IS NOT NULL AS revoked FROM auth_sessions
   WHERE user_id = '11111111-1111-1111-1111-111111111111'
   ORDER BY created_at;
   ```
   Все записи должны иметь `revoked = true`.

---

## Sprint 3 — 4) Logout

1. Выполните Login заново (шаг 1).
2. Нажмите **Logout** в разделе **4) Шорткаты — Auth**.
3. В логе появится:
   ```
   AUTH logout -> 204 сессия отозвана
   AUTH токены очищены из localStorage
   ```
4. Access token и Refresh token очищаются.
5. Попытка повторного Refresh вернёт `session_revoked`:
   - Нажмите **Refresh** → логи покажут `session_revoked` или пустое поле (токен уже очищен).

---

## Sprint 3 — 5) Auto-refresh при 401

1. Выполните Login.
2. В разделе **2) HTTP запрос** выберите `GET`, путь `/api/v1/me/unread-count`.
3. Нажмите **Отправить + auto-refresh при 401**.
4. Если access token истёк (TTL 900 сек) — в логе появится:
   ```
   AUTH 401 получен — пробуем auto-refresh...
   AUTH refresh -> 200 {...}
   AUTH auto-refresh успешен, повторяем запрос
   HTTP GET /api/v1/me/unread-count -> 200 {...}
   ```

---

## Sprint 3 — Ожидаемый итог

| Шаг | Ожидание |
|-----|----------|
| Login | `200`, токены в localStorage, Session ID показан |
| Refresh | `200`, новые токены, старый доступен для reuse test |
| Reuse test | `401 session_compromised`, family отозвана, токены очищены |
| Logout | `204`, сессия отозвана на сервере, localStorage очищен |
| Auto-refresh | При 401 автоматически обновляет токены и повторяет запрос |
