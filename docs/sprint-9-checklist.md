# Sprint 9 Checklist

Источник: `docs/sprint-9-plan.md`.

**Цель спринта:** at-rest шифрование тел сообщений (AES-256-GCM, Вариант A). API для клиента без breaking changes.

**Предусловия:**
- Chat create/list/WS работают.
- Желательно Sprint 7 (preview в списке) — иначе preview шифрования подключить при появлении list.
- Выбран формат хранения (см. plan §3.B): явный ciphertext + dual-read.

**Статус:** PLANNED  
**Порядок:** после Sprint 7; не смешивать DoD с 7/8.

---

## 1) Подготовка и контракты

- [x] Зафиксировать Вариант A (envelope AES-GCM) в plan.
- [x] Подготовить `docs/api-sprint-9.md` (хранение + non-breaking HTTP).
- [ ] Утвердить layout: `body_ciphertext` + `body_key_id` (+ nonce strategy).
- [ ] Утвердить имя env: `MESSAGE_ENCRYPTION_KEY`, `MESSAGE_ENCRYPTION_KEY_ID`.

---

## 2) Crypto-пакет

- [ ] `internal/crypto/messagecrypto` — Encrypt / Decrypt (AES-256-GCM).
- [ ] Поддержка `key_id`; опционально AAD = message_id.
- [ ] Unit-тесты: roundtrip, bad tag, wrong key.
- [ ] Запрет логировать key / plaintext в error paths.

---

## 3) Миграция БД

- [ ] Миграция: колонки ciphertext / key_id (и nonce, если отдельно).
- [ ] `body` остаётся для dual-read на переходный период.
- [ ] Индексы не на ciphertext (не нужны).
- [ ] Integration: migrate up на test DB.

---

## 4) Store + Service

- [ ] Create: encrypt → писать ciphertext; plaintext `body` не сохранять (или NULL).
- [ ] Get/List: dual-read decrypt | legacy plaintext.
- [ ] DTO наружу — только plaintext (как сейчас).
- [ ] Unit/integration service+repo.

---

## 5) WS, push, preview

- [ ] WS `message_new` — body после decrypt.
- [ ] Notification worker — decrypt перед payload (или без preview — зафиксировать в Примечании).
- [ ] Dialog list `body_preview` (если Sprint 7 в коде) — decrypt + truncate.
- [ ] Expirer / receipts — без регрессий.

---

## 6) Backfill и зачистка plaintext

- [ ] Job или one-shot: все legacy `body` → ciphertext, обнулить plaintext.
- [ ] После backfill на prod: запретить plaintext write (CHECK / код).
- [ ] Документировать rollback: только с ключом.

---

## 7) Конфиг и prod

- [ ] Config + `.env.example` (placeholder, не реальный ключ).
- [ ] Fail-fast при старте main-service/worker без валидного ключа (prod).
- [ ] Выставить ключ на VPS; не коммитить.
- [ ] Smoke: отправить сообщение → в DB только ciphertext → клиент видит текст.

---

## 8) Тесты и качество

- [ ] `task fmt`, `task lint`, `task test`, `task test:integration`.
- [ ] Проверка: `SELECT body FROM messages` не содержит тестовую фразу plaintext (после cutover).

---

## 9) Документация и закрытие

- [ ] Обновить `docs/chat-architecture-plan.md` (Sprint 9).
- [ ] `docs/known-limitations-sprint-9.md` (не E2EE; ключ = SPOF; push plaintext на устройстве; purge hard-delete — later).
- [ ] Чеклист → **DONE**.

---

## 10) DoD

- [ ] Новые сообщения в БД зашифрованы.
- [ ] Клиентский API без breaking changes.
- [ ] Legacy обработан (dual-read и/или backfill).
- [ ] Ключ не в git.
- [ ] Lint/tests green.

---

**Sprint 9 — PLANNED**
