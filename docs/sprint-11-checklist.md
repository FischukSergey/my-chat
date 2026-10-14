# Sprint 11 Checklist

Источник: `docs/sprint-11-plan.md`.

**Цель спринта:** встроенный пак стикеров в PWA; отправка тем же `POST …/messages` с кодом в `body`.

**Предусловия:**
- Sprint 8 PWA-чат и Sprint 10 unsend работают.
- Sprint 9 (шифрование) отложен, не смешивать DoD.

**Статус:** DONE  
**Порядок:** после Sprint 10; Sprint 9 не открывать.

---

## 1) Подготовка и контракты

- [x] Утвердить: код `sticker:{pack}/{id}` в `body`; без новых HTTP-роутов; один пак `default`.
- [x] Утвердить источник ассетов (Fluent Emoji MIT **или** свои) + файл `NOTICE`.
- [x] Подготовить `docs/api-sprint-11.md`.

Примечание: решения — `docs/sprint-11-plan.md` §3.A. Ассеты — Fluent Emoji 3D, MIT, `mobile/public/stickers/NOTICE`.

---

## 2) Ассеты

- [x] 12–16 файлов в `mobile/public/stickers/default/{id}.webp` (или `.png`).
- [x] `mobile/public/stickers/NOTICE` (лицензия / атрибуция).
- [x] Каталог id совпадает с именами файлов.

Примечание: 16 PNG (256×256), Fluent 3D: эмоции smile/happy/joy/love/neutral/sad/cry/angry/wow/think + wave/thumbsup/heart/thanks/fire/party.

---

## 3) Клиент PWA

- [x] `parseStickerBody` / каталог `STICKERS`.
- [x] Кнопка + сетка у композера; тап → `sendMessage` с кодом.
- [x] Пузырь: картинка для валидного кода, иначе текст.
- [x] Неизвестный / битый код — плейсхолдер или код, лента жива.
- [x] Home: превью «Стикер».
- [x] Unsend и TTL скрывают стикер-пузырь как текстовый.

Примечание: `mobile/src/stickers.ts`; пузырь 144px; img.onerror → текст кода; unsend/TTL без отдельных веток.

---

## 4) Сервер

- [x] Не менять API (Must).
- [ ] (Should) `BuildPreview` → «Стикер» для кода — только если решим не оставлять это клиенту.

Примечание: сервер не меняли; preview на клиенте.

---

## 5) Тесты и качество

- [x] Юнит на парсер кода (валидный / чужой префикс / инъекция).
- [x] `task lint`; `task test` если тронут Go (обычно нет).
- [x] Manual: Alice шлёт стикер → у Bob картинка; Home «Стикер»; unsend/TTL ок.

Примечание: `stickers.test.ts` 8 тестов green; `npm run build` OK; `task lint` green (Go не меняли). Local smoke 2026-09-27; prod (`beepru.ru`) подтверждён пользователем.

---

## 6) Документация и закрытие

- [x] `docs/chat-architecture-plan.md` — Sprint 11.
- [x] `docs/known-limitations-sprint-11.md` при закрытии.
- [x] Чеклист → **DONE**.

---

## 7) DoD

- [x] Стикер уходит существующим send; оба видят картинку.
- [x] Home не показывает сырой код.
- [x] Нет S3 / новых роутов / загрузки файлов.
- [x] Lint/tests green; smoke в PWA.

Примечание: Should `BuildPreview` на сервере не делали (клиентская подмена). Smoke local + prod 2026-09-27.

---

**Sprint 11 — DONE**
