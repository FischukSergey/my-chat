# API Sprint 11 — встроенные стикеры

HTTP и WS **без breaking changes**. Стикер — это обычное сообщение, у которого `body` соответствует конвенции ниже.

Связанные: `docs/sprint-11-plan.md`, `docs/api-sprint-6.md` (send/list), `docs/api-sprint-7.md` (preview).

---

## 1) Решения

| Тема | Значение |
|------|----------|
| Новый endpoint | нет |
| Send / list / WS | как Sprint 6: `body` — строка |
| Код стикера | `sticker:{pack}/{id}` |
| `pack`, `id` | только `[a-z0-9_-]+`, без `/`, без пробелов |
| Встроенный пак | `default` |
| Файл на клиенте | `/stickers/{pack}/{id}.png` |
| Сервер | хранит и отдаёт код как есть |
| Preview API | сырой `BuildPreview(body)` (клиент подменяет на «Стикер») |

Пример `body`: `sticker:default/wave`

Не стикер: `смотри sticker:default/wave`, `STICKER:default/wave`, `sticker:../x`.

---

## 2) HTTP (без изменений)

```
POST /api/v1/dialogs/{dialog_id}/messages
Authorization: Bearer <access_token>
{ "body": "sticker:default/wave" }
```

Ответ и `message_new` — как у текста: поле `body` содержит тот же код.

`GET …/messages` и `GET /dialogs` → `last_message.body_preview` может быть `sticker:default/wave`. Клиент Sprint 11 показывает «Стикер».

---

## 3) Клиент

- Отправлять только id из встроенного каталога.
- Рендер: точное совпадение regexp `^sticker:([a-z0-9_-]+)/([a-z0-9_-]+)$`.
- Нет файла / нет в каталоге — не ломать пузырь (плейсхолдер или код).
- Unsend: тот же `DELETE /api/v1/messages/{id}`.

---

## 4) Вне scope

Новые роуты, `type`/`mime` в JSON, upload, CDN, Lottie, правка push-текста, Sprint 9 ciphertext.
