# Known Limitations — Sprint 11

Дата: 2026-09-27  
Статус: зафиксировано по итогам спринта (local + prod smoke подтверждён)

---

## 1. Стикер — код в `body`, не вложение

**Что сделано:** пак `default` (16 Fluent Emoji 3D PNG) в `mobile/public/stickers/`; send тем же `POST …/messages`; `body` = `sticker:{pack}/{id}`.

**Остаётся:** нет `type` / `attachment_id`, нет upload, нет своего пака пользователя. Старый клиент без Sprint 11 видит сырой код.

---

## 2. Preview «Стикер» только на клиенте

`GET /dialogs` по-прежнему отдаёт `body_preview` = код (`BuildPreview` не трогали). PWA подменяет на «Стикер». Push без изменений (title = username, body = «Новое сообщение»).

---

## 3. Нет анимации и каталога на сервере

Только статичный PNG 256×256, пузырь 144px. Lottie/GIF, CDN, серверный каталог — вне scope.

---

## 4. Вне scope (напоминание)

- Загрузка фото / S3 / MinIO.
- Реакции на сообщение.
- Правка push («прислал стикер»).
- Sprint 9 AES-GCM.
