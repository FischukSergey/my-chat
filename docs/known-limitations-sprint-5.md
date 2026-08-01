# Known Limitations — Sprint 5

Дата: 2026-08-02  
Статус: зафиксировано по итогам спринта

---

## 1. nginx: `set` должен стоять до `rewrite+break`

**Проблема:** При использовании `rewrite ... break` директива `set $var` стоящая после неё не выполняется, так как `break` прерывает обработку модуля `ngx_http_rewrite_module` (к которому относится и `set`). Переменная остаётся пустой, nginx возвращает HTTP 500.

**Решение:** Всегда размещать `set $upstream_var` **до** `rewrite ... break` в одном location-блоке.

**Файл:** `deploy/prod/nginx/conf.d/my-chat.conf`

---

## 2. Docker build context включает certbot/conf с root-правами

**Проблема:** Директория `deploy/prod/certbot/conf/accounts/` принадлежит `root` (создаётся certbot). При `docker compose build` контекст включал эту директорию → `permission denied` → сборка падала.

**Решение:** Добавлен `.dockerignore` с исключением `deploy/prod/certbot/`.

---

## 3. nginx не перезагружает конфиг при `docker compose up -d`

**Проблема:** nginx-конфиг монтируется как volume с хоста. `docker compose up -d` не перезапускает контейнер если изменился только файл в volume (только образ/env/volumes определение). Новый конфиг не применяется без явного `nginx -s reload`.

**Решение:** CD workflow явно выполняет `docker exec my-chat-nginx-prod nginx -s reload` после `docker compose up -d`.

---

## 4. `www.beepru.ru` не настроен

**Проблема:** Поддомен `www.beepru.ru` не сконфигурирован в nginx и не имеет CNAME-записи в DNS — показывает дефолтную страницу регистратора.

**Статус:** Out of scope для Sprint 5. Мобильный клиент работает с `beepru.ru` напрямую.

**Решение (при необходимости):** Добавить CNAME `www → beepru.ru` в DNS и `www.beepru.ru` в `server_name` nginx + перевыпустить сертификат с `--expand`.

---

## 5. Тестовый пользователь в prod-базе

**Проблема:** В prod-базе нет seed-данных. Для smoke-тестов auth-endpoint потребовалось вручную создать тестового пользователя через `psql`.

**Решение:** `INSERT INTO users (id, status) VALUES ('11111111-...', 'active')` выполнен вручную. Для автоматизации — добавить seed-миграцию или E2E-тест с регистрацией.

---

## 6. OCSP Stapling не работает (предупреждение nginx)

**Проблема:** nginx выдаёт предупреждение `ssl_stapling ignored, no OCSP responder URL in the certificate`. Let's Encrypt сертификаты поддерживают OCSP, но URL OCSP недоступен через Docker DNS резолвер во время старта nginx.

**Статус:** Не влияет на работу. Оценка SSL Labs — A+ (OCSP Must Staple = No).

**Решение (опционально):** Добавить `ssl_stapling_responder http://r3.o.lencr.org/;` или отключить `ssl_stapling off` для устранения предупреждения.

---

## 7. SSH-доступ на VPS заблокирован с некоторых IP

**Проблема:** SSH-соединение с локального Mac-разработчика периодически сбрасывалось / не устанавливалось (возможно, фаервол VPS или rate limiting). CD workflow через GitHub Actions работал стабильно.

**Статус:** Не блокирует работу — деплой выполняется через CD.
