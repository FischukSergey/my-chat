# Sprint 9 — детальный план (Message encryption at-rest)

Источник: решение хранить тела сообщений зашифрованными (Вариант A — envelope encryption / AES-GCM). Не путать с E2EE.

Порядок относительно других спринтов: **после Sprint 7** (список/preview уже стабильны), можно до или после Sprint 8 (PIN unlock ортогонален). Рекомендация: 7 → 9 → 8 или 7 → 8 → 9; не смешивать с 7/8 в одном DoD.

## 1) Цель спринта

Сообщения в PostgreSQL хранятся **в зашифрованном виде**. Дамп БД / бэкап SQL не содержит читаемый plaintext `body`.

- Сервер шифрует при записи, расшифровывает при чтении (REST, WS, preview списка, push payload).
- Клиентский API **без breaking changes**: по-прежнему отдаёт plaintext `body` / `body_preview` по TLS.
- Ключ — из env/secret (`MESSAGE_ENCRYPTION_KEY`), не в git.

К концу Sprint 9: новые сообщения только ciphertext в БД; старые plaintext либо смигрированы, либо читаются dual-read до backfill.

## 2) Входные условия

- Таблица `messages.body TEXT` (plaintext).
- Create / List / Get / WS `message_new` / push используют `body` как есть.
- Sprint 7 (желательно): `last_message.body_preview` строится из body — decrypt на сервере перед preview.
- Нет колонок `ciphertext` / `key_id` / `nonce`.

## 3) Ключевые задачи

### A. Криптосхема (Вариант A)

1. Алгоритм: **AES-256-GCM**.
2. Master key: 32 байта, в env как base64 или hex (`MESSAGE_ENCRYPTION_KEY`).
3. На сообщение: случайный 12-byte nonce; ciphertext = nonce || ciphertext||tag (или раздельные колонки — выбрать одно и зафиксировать в api/plan).
4. `key_id` (короткий string, напр. `v1`) — для будущей ротации без даунтайма.
5. Пакет: `internal/crypto/messagecrypto` (Encrypt/Decrypt), без логирования plaintext/key.
6. AAD (optional): `message_id` или `dialog_id` — чтобы ciphertext нельзя было переставить между строками.

### B. Схема БД

Рекомендуемый путь (минимальный churn):

1. Добавить:
   - `body_ciphertext BYTEA NULL` (или TEXT base64 — BYTEA предпочтительнее);
   - `body_key_id TEXT NULL`;
   - `body_nonce BYTEA NULL` — **если** nonce не префиксируется в ciphertext.
2. Либо один столбец `body_encrypted BYTEA` = `version|nonce|ciphertext+tag`, plaintext `body` постепенно deprecate.
3. Transition:
   - **Write:** всегда писать ciphertext; `body` либо пустой placeholder, либо NULL после миграции constraint.
   - **Read:** если ciphertext есть → decrypt; else → legacy plaintext `body` (dual-read).
4. One-shot backfill job/SQL+Go: все строки с plaintext → encrypt → clear plaintext.
5. После backfill: `body` DROP или оставить NULL-only; CHECK что ciphertext NOT NULL для новых.

Альтернатива «in-place»: шифровать в ту же `body` как base64 blob + флаг `body_enc bool` — проще миграция, хуже ясность. Предпочтение: явные колонки ciphertext.

### C. Точки интеграции

1. `chat.Service` CreateMessage — encrypt before store.
2. List/Get — decrypt before DTO.
3. WS hub payload — plaintext после decrypt (как сейчас для клиента).
4. Notification worker / push — decrypt перед телом уведомления (или generic «Новое сообщение» без preview — Should: сохранить preview как сейчас).
5. Sprint 7 dialog list preview — decrypt + truncate.
6. Expirer / soft-delete — без изменений семантики.

### D. Конфиг и prod

1. Config yaml + env override для key и `key_id`.
2. Local: ключ в `.env` / example; prod: только на VPS secret.
3. Старт сервиса: fail-fast если key отсутствует/неверной длины (prod); local может allow plaintext-only mode **только** для тестов — лучше всегда требовать ключ в integration.
4. Ротация ключа: out of scope Must; заложить `key_id` + возможность читать старый key из map (Should).

### E. Тесты

- Unit: encrypt/decrypt roundtrip, tamper fails, wrong key fails.
- Service/store: create → DB row без plaintext substring; list returns plaintext.
- Integration: send/list/WS path.
- Не коммитить реальные prod keys; test key фикстура в коде тестов.

## 4) Что не входит (out of scope)

- **E2EE** (клиентские ключи, сервер не читает).
- Per-dialog / per-user keys.
- Физический `DELETE` / `pg_cron` purge soft-deleted (отдельный backlog; можно Sprint 10).
- Шифрование других PII (username, devices).
- Transparent disk encryption VPS.
- Смена клиентского контракта API.

## 5) Риски

| Риск | Митигация |
|------|-----------|
| Потеря ключа = потеря истории | Бэкап secret отдельно от DB dump; документировать |
| Push/preview всё ещё plaintext на устройстве | Ожидаемо для Variant A; не claim E2EE |
| Dual-read баги | Тесты legacy + encrypted; backfill asap |
| Key в git | `.env.example` placeholder; checklist CI grep |

## 6) DoD

- [ ] Новые сообщения в БД без читаемого plaintext body.
- [ ] REST/WS/push (и preview списка, если Sprint 7 уже есть) отдают корректный текст клиенту.
- [ ] Legacy строки читаются или смигрированы.
- [ ] Ключ не в git; prod key только на VPS.
- [ ] Lint/tests green.

## 7) Артефакты

- `docs/sprint-9-plan.md` (этот файл)
- `docs/sprint-9-checklist.md`
- `docs/api-sprint-9.md` (изменения хранения + отсутствие breaking HTTP)
- По завершении: `docs/known-limitations-sprint-9.md`
