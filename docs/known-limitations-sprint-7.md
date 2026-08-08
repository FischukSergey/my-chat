# Known Limitations — Sprint 7

Дата: 2026-08-07  
Статус: зафиксировано по итогам спринта

---

## 1. Несколько аккаунтов на одном устройстве → общие push

**Проблема:** Push таргетится по `user_id` → `devices.ListActive`. Upsert web-устройства уникален по `(user_id, endpoint)`. При логине разных пользователей с одного PWA один и тот же push endpoint регистрируется отдельно для каждого `user_id`. Logout **не** вызывает `devices/unregister` — старые записи остаются `enabled`.

**Следствие:** уведомления могут приходить на телефон для «старых» аккаунтов; сценарий alice→bob на одном устройстве может показать push для bob, даже если UI открыт как alice (если bob offline по WS).

**Решение (later):** `unregister` при logout / смене аккаунта; опционально disable devices другого user с тем же endpoint.

---

## 2. Global `CountUnread` не фильтрует soft-deleted

**Проблема:** `ReceiptRepository.CountUnread` (global badge / `/me/unread-count`) не исключает `messages.deleted_at IS NOT NULL`. Per-dialog unread в `ListByUserID` уже фильтрует soft-delete.

**Следствие:** после TTL-expire global badge может кратковременно расходиться с суммой unread в списке чатов.

**Решение (later):** добавить `AND m.deleted_at IS NULL` в `CountUnread` (и согласовать с worker badge).

---

## 3. Пагинация списка диалогов — out of scope

**Проблема:** `GET /api/v1/dialogs` возвращает полный список без cursor/limit.

**Статус:** Достаточно для 1:1 MVP. При росте числа чатов — cursor pagination.

---

## 4. Rate-limit на `GET /users/search` не добавлен

**Проблема:** Prefix-search username (auth required, min 2 символа) теоретически упрощает enumeration.

**Статус:** Принято для Sprint 7 (auth + min length). Rate-limit / CAPTCHA — optional later.

---

## 5. CD: права на `/opt/my-chat/.git` после git от root

**Проблема:** Если на VPS выполнять `git pull` от `root`, часть `.git/objects/<xx>/` оказывается `root:root`. CD под `deploy` падает с `insufficient permission for adding an object to repository database`.

**Решение:** `chown -R deploy:deploy /opt/my-chat`; не запускать git в prod-репо от root. Разовый инцидент 2026-08-07.

---

## 6. Вне scope (напоминание)

- Групповые чаты, аватарки, названия чатов.
- PWA local PIN unlock → закрыто в Sprint 8 (`docs/known-limitations-sprint-8.md`; WebAuthn — 8.1+).
- At-rest / E2EE шифрование сообщений → Sprint 9.
- Typing / presence / поиск по тексту сообщений.
