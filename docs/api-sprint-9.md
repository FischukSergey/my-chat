# API Sprint 9 — Message encryption at-rest

Цель: серверное шифрование тел сообщений. **HTTP/WS контракты для клиента не ломаются** — поля `body` / preview остаются plaintext по TLS.

Связанные: `docs/sprint-9-plan.md`, `docs/api-sprint-4.md` (messages), `docs/api-sprint-7.md` (preview).

---

## 1) Решения

| Тема | Решение |
|------|---------|
| Модель | Envelope encryption, AES-256-GCM (Вариант A) |
| Кто шифрует | main-service (и worker при чтении body для push) |
| E2EE | Нет (out of scope) |
| Breaking API | Нет |
| Ключ | `MESSAGE_ENCRYPTION_KEY` (32 bytes, base64/hex), `MESSAGE_ENCRYPTION_KEY_ID` (напр. `v1`) |

---

## 2) Хранение (внутреннее, не публичный API)

Таблица `messages` (целевое состояние после cutover):

| Колонка | Назначение |
|---------|------------|
| `body` | Legacy plaintext; NULL после backfill |
| `body_ciphertext` | AES-GCM ciphertext (+ tag); nonce — prefix или отдельная колонка |
| `body_key_id` | Идентификатор ключа (`v1`) |

Публичные JSON-поля **не** включают ciphertext.

Формат ciphertext (рекомендация для реализации):

```
body_ciphertext = nonce (12 bytes) || ciphertext_with_tag
```

AAD (рекомендация): `message_id` UUID bytes/string — зафиксировать в коде и тестах.

---

## 3) HTTP — без изменений shape

### POST /dialogs/{id}/messages

Request/response как в Sprint 4:

```json
{ "body": "привет" }
```

```json
{
  "message_id": "…",
  "dialog_id": "…",
  "sender_id": "…",
  "body": "привет",
  "created_at": "…"
}
```

Сервер: encrypt → store ciphertext; в response — исходный plaintext.

### GET /dialogs/{id}/messages

Элементы с `"body": "<plaintext>"` после decrypt (или legacy read).

### GET /dialogs (Sprint 7)

`last_message.body_preview` — plaintext после decrypt + truncate.

---

## 4) WebSocket / Push

- `message_new.data.body` — plaintext (как сейчас).
- Push notification title/body — plaintext на устройстве (ограничение Variant A; не заявлять как E2EE).

Ошибки decrypt на сервере: не отдавать мусор клиенту; `500` / skip push + лог без ciphertext dump.

---

## 5) Ошибки / операции (новые, внутренние)

Публичных новых error code для клиента не требуется.

Операционные:

- Missing/invalid key at startup → process exit (prod).
- Decrypt failure на legacy-corrupt row → log + exclude message / 500 на list (зафиксировать поведение в known-limitations).

---

## 6) Админ / backfill (не публичный REST)

One-shot или internal task: encrypt all plaintext rows. Не обязан быть HTTP endpoint; допустим `task` / CLI в `cmd/`.

---

## 7) Чего нет в контракте

- Клиент не шлёт и не принимает ciphertext.
- Нет API выдачи ключей пользователю.
- Нет passwordless/E2EE handshake.
