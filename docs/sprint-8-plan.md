# Sprint 8 — детальный план (PWA unlock / PIN + UX polish)

Источник: в установленном PWA (Safari Home Screen) `LocalAuthentication` / Capacitor Face ID недоступны. Сейчас cold start с сохранённым refresh делает **silent auto-login** (`startUnlockNoBiometric`) — отдельного локального gate (PIN / экран разблокировки как UX) нет. Пользователь видит по сути только Войти / Выйти.

Дополнительно (UX): push показывает превью текста в title (без имени отправителя); фон чата светлый/нейтральный.

## 1) Цель спринта

1. **Локальный PIN-unlock** в PWA:
   - обязательная установка PIN после регистрации / первого успешного login (если PIN ещё не задан на устройстве);
   - cold start: экран ввода PIN → только после успеха silent refresh → Home;
   - resume после сворачивания / ухода в background: повторный lock с **grace period** (не на каждый мгновенный `visibilitychange`);
   - смена PIN из Settings;
   - Capacitor native: существующий Face ID path **не ломать** (PIN в native — Should/optional).

2. **Push:** в уведомлении о новом сообщении в **title** — **username отправителя** (вместо превью текста сообщения). Body оставить служебным («Новое сообщение»). Превью текста в push **не** показывать (меньше утечки содержимого на lock screen).

3. **UI чата:** фон области переписки чуть темнее, с **бирюзовым** оттенком и лёгким **водяным знаком** (паттерн / логотип), без ухудшения читаемости пузырей.

К концу Sprint 8: на iPhone PWA без верного PIN чаты не открываются; после сворачивания приложение снова просит PIN (с учётом grace); push показывает «от кого»; чат визуально обновлён.

**WebAuthn / platform passkey (Face ID sheet в PWA) — out of scope этого спринта** → отдельный follow-up (Sprint 8.1 / позже), поверх PIN как fallback.

## 2) Входные условия

- Sprint 6–7: PWA на prod, Home = список диалогов, auth JWT + refresh в Preferences.
- Фактический cold start PWA: `hasRefresh` → `startUnlockNoBiometric` → auto Home (без PIN UI).
- Auth: access/refresh JWT, device_id, sessions.
- Redis / новые backend endpoints для PIN **не требуются** (PIN локален).

## 3) Ключевые задачи

### A. Локальная модель PIN (клиент)

Хранение только на устройстве (`Preferences` / аналог), **без** отправки PIN на сервер.

Рекомендуемые ключи (имена зафиксировать в коде + `docs/api-sprint-8.md`):

| Ключ | Назначение |
|------|------------|
| `my_chat_pin_hash` + `my_chat_pin_salt` | verifier PIN (не plaintext) |
| `my_chat_pin_set` | флаг «PIN настроен» |
| refresh / access / session | как сейчас; см. §3.B про усиление |

Политика PIN (Must):

- длина **4 цифры** (зафиксировано в чеклисте §1);
- подтверждение при setup (ввод дважды);
- lockout после N неверных попыток (например 5) → краткий cooldown или wipe local session + login (выбрать в §1 чеклиста; по умолчанию: **5 попыток → clear tokens → Login**).

### B. Уровень защиты refresh (Must / Should)

| Уровень | Описание | Scope |
|---------|----------|--------|
| **Gate UI** | PIN только открывает UI; refresh лежит в Preferences как сейчас | Must минимум |
| **Encrypt refresh** | refresh (и при желании access) в storage как ciphertext; ключ = KDF(PIN, salt) → AES-GCM | **Should** (предпочтительно в том же спринте, если успеваем) |

Без encrypt PIN защищает от случайного взгляда, не от извлечения storage. Для MVP Acceptable: Gate UI + явный known limitation. Целевое: Encrypt refresh.

### C. Семантика unlock vs login

| Сценарий | Механизм |
|----------|----------|
| Первый вход / смена аккаунта | username + password → JWT (как сейчас) |
| Setup PIN | сразу после register/login, если `!pin_set` |
| Cold start (есть session + pin_set) | экран PIN → verify → refresh JWT → Home |
| Resume после background | lock UI после grace → снова PIN (JWT может остаться в памяти до успешного PIN) |
| Logout | clear tokens; PIN verifier на устройстве: **оставить** (быстрее re-login setup) или **сбросить** — решение в чеклисте; рекомендация: **сбросить PIN при logout** вместе с токенами (проще threat model) |
| Capacitor native | Face ID как сейчас; не требовать PIN в Must |

### D. Клиент PWA — экраны и flow

1. **Setup PIN** — новый экран после успешного register/login при отсутствии PIN.
2. **Unlock PIN** — заменить бессмысленный auto-`startUnlockNoBiometric` на реальный ввод PIN; кнопка «Выйти из аккаунта» сохраняется.
3. **Grace period** при `visibilitychange` / `pagehide`:
   - уход в hidden → запомнить `locked_at` / стартовать таймер;
   - возврат: если hidden дольше **T** (рекомендация **60s**, конфиг константой) → `showScreen("unlock")` и не показывать Home/Chat до PIN;
   - T=0 = lock сразу при любом hide (слишком агрессивно для MVP — не default).
4. Пока locked: не слать `mark_read`, не показывать превью сообщений на unlock-экране.
5. Settings (минимум): «Сменить PIN» (старый → новый ×2).
6. Capacitor: ветка `isBiometricAvailable` / `startUnlock` без регрессий.

### E. Push: title = sender username (backend + SW)

Сейчас (`webpush.buildPayload`): `title = preview` (текст сообщения), `body = "Новое сообщение"`.

Целевое:

| Поле | Было | Станет |
|------|------|--------|
| `title` | preview тела | **username отправителя** |
| `body` | «Новое сообщение» | без изменений («Новое сообщение») |
| preview в payload | есть | можно оставить в outbox для логов/будущего, **не** мапить в title |

Реализация (Must):

1. В outbox payload `message_new` добавить `sender_username` (lookup username по `sender_id` при `enqueueOutbox`; fallback — короткий id / `"user"`, если username недоступен).
2. Worker → `push.Message` прокидывает username (новое поле или переиспользовать Preview-слот осознанно — лучше отдельное поле `SenderUsername`).
3. `WebPushProvider.buildPayload`: `Title = senderUsername`.
4. Обновить unit-тесты webpush / chat outbox.
5. SW: по-прежнему `data.title` / `data.body` — менять не обязательно, если payload корректный.
6. Служебная подпись ОС «from MyChat» (из `manifest.json`) **не трогаем**.

Без Redis. Нагрузка: один lookup username на offline-send — пренебрежимо.

### F. UI: фон чата (клиент)

Только `mobile` CSS/HTML (экран `#chat` / лента сообщений):

- фон чуть темнее текущего, бирюзовый tint (CSS variables; не «фиолетовый AI-default»);
- водяной знак: повторяющийся мягкий паттерн (SVG/CSS `background-image`, низкая opacity), не перекрывает текст пузырей;
- проверить светлые/тёмные пузыри на контраст (WCAG-ish на глаз);
- Home / login / unlock — **не** обязаны менять фон в Must (только область чата).

### G. Backend / infra (кроме push §E)

- **Нет** Redis, **нет** `/webauthn/*`, **нет** таблицы PIN на сервере.
- JWT login/refresh/logout без изменений контракта.
- PIN — клиентский; push title — единственное осмысленное server-изменение спринта.

### H. Тесты и smoke

- Unit: hash/verify PIN, grace timer; webpush title = username; outbox содержит `sender_username`.
- Ручной smoke iPhone PWA:
  1. register → setup PIN → Home;
  2. kill PWA → cold start → PIN → Home;
  3. свернуть >60s → вернуться → PIN;
  4. неверный PIN ×N → ожидаемое поведение;
  5. logout → login → снова setup PIN (если приняли wipe PIN on logout);
  6. offline-получатель: push title = username отправителя, body «Новое сообщение»;
  7. визуально: фон чата + watermark читаемы.
- `task fmt` / `lint` / `test` (+ integration если затронут enqueue).

## 4) Что не входит (out of scope)

- WebAuthn / passkeys / Face ID sheet в PWA (→ Sprint 8.1+).
- Redis / challenge store / серверная проверка PIN.
- Passwordless login без username/password.
- Замена Capacitor `LocalAuthentication` в native build.
- E2EE / at-rest encryption тел сообщений (→ Sprint 9).
- Сложный recovery PIN через email/SMS.
- Текст сообщения / preview в push title или body (осознанно не возвращаем в MVP).
- Полный редизайн Home / branding «from MyChat» в системной строке уведомления.

## 5) Риски

| Риск | Митигация |
|------|-----------|
| PIN только UI-gate, refresh в plaintext storage | Should: encrypt refresh; иначе known limitation |
| Lock на каждый tab switch бесит | Grace period 60s; настраиваемая константа |
| Забыли PIN | Logout + password login + новый setup PIN |
| Путаница с native Face ID | Документировать: PWA=PIN, native=биометрия |
| Документы Sprint 8 раньше описывали WebAuthn | Этот plan заменяет scope; старый WebAuthn API снят с Must |

## 6) DoD

- [ ] После register/login пользователь обязан задать PIN (если ещё не задан).
- [ ] Cold start с сессией требует PIN; без PIN Home не показывается.
- [ ] Resume после background дольше grace → снова PIN.
- [ ] Смена PIN работает; logout очищает сессию (и PIN — по принятому решению).
- [ ] Push `message_new`: title = username отправителя; body = «Новое сообщение».
- [ ] Фон чата: бирюзовый tint + watermark, читаемость ок.
- [ ] Capacitor Face ID path не сломан.
- [ ] Lint/tests green для затронутого; smoke на `beepru.ru` PWA.

## 7) Артефакты

- `docs/sprint-8-plan.md` (этот файл)
- `docs/sprint-8-checklist.md`
- `docs/api-sprint-8.md` (PIN keys + push payload contract)
- По завершении: `docs/known-limitations-sprint-8.md`
