# Sprint 5 Checklist

Источник: `docs/sprint-5-plan.md`.

**Цель спринта:** полностью рабочий prod-деплой на VPS с HTTPS/WSS, автодеплоем из GitHub Actions и изоляцией секретов от git.

**Предусловие:** зарегистрирован домен, добавлена A-запись `beepru.ru → IP VPS`.

---

## 1) Подготовка и контракты

- [x] Зарегистрировать домен и добавить A-запись на IP VPS.
  > Домен `beepru.ru`, IP VPS `87.228.112.251`. SSH доступ: `ssh my-chat` (root@87.228.112.251, ключ `~/.ssh/my-chat-vps`).
- [x] Определить финальную схему маршрутизации nginx (один домен с path-based routing vs поддомены).
  > Принято: path-based routing на одном домене (`beepru.ru`). Nginx проксирует `/api/v1/...` → main-service:8080, `/ws/connect` → main-service:8080 (WS upgrade), `/auth/...` → auth-proxy:33081, `/health` → main-service:8080.
- [x] Зафиксировать prod URL в `docs/api-sprint-5.md` (базовый URL для мобильного клиента).
  > Создан файл `docs/api-sprint-5.md` с prod URL `https://beepru.ru`, таблицей маршрутизации и полным списком endpoints.
- [x] Создать `.env.example` — шаблон переменных окружения без реальных значений.
  > Создан `.env.example` в корне проекта с placeholder-значениями для всех переменных (POSTGRES_*, DATABASE_DSN, JWT_SECRET, DOMAIN, PUSH_PROVIDER, METRICS_*). Также обновлён `.gitignore`: добавлено `!.env.example` (чтобы файл не попал под паттерн `.env.*`) и `deploy/prod/certbot/`.

---

## 2) Prod Dockerfiles

- [x] Проверить и при необходимости дополнить `prod.Dockerfile` (main-service):
  - multi-stage build (golang:alpine → alpine);
  - копирование конфигов в образ;
  - `EXPOSE 8080`.
  > Добавлены: `ca-certificates`, `-ldflags="-s -w"` (уменьшение размера бинаря), `EXPOSE 9100` (метрики).
- [x] Проверить `auth-proxy.Dockerfile` (аналогичная структура).
  > Добавлены: `ca-certificates`, `-ldflags="-s -w"`.
- [x] Проверить `notification-worker.Dockerfile`.
  > Добавлены: `ca-certificates`, `-ldflags="-s -w"`.
- [x] Проверить `message-expirer.Dockerfile`.
  > Добавлены: `ca-certificates`, `-ldflags="-s -w"`, `EXPOSE 9101` (метрики).
- [x] Убедиться, что все Dockerfile используют `go 1.25` (или текущую версию из `go.mod`).
  > Все используют `golang:1.25-alpine`, совпадает с `go 1.25.0` в `go.mod`.
- [x] Проверить `go build ./...` внутри каждого Dockerfile — сборка без ошибок.
  > `go build ./...` выполнен локально — exit code 0, ошибок нет.

---

## 3) Prod конфиги сервисов

- [x] Создать `configs/config.main-service.prod.yaml`:
  - `servers.client.addr: 0.0.0.0:8080`;
  - `servers.metrics.addr: 0.0.0.0:9100`;
  - `database.dsn` через env-переменную `${DATABASE_DSN}`;
  - `jwt.secret` через env-переменную `${JWT_SECRET}`;
  - `log.level: info`, `log.format: json`;
  - `chat.message_ttl_seconds: 60`;
  - `cors.allowed_origins` — перечислить prod домен.
  > Создан. `cors.allowed_origins: [https://beepru.ru]`, `log.format: json`, `message_ttl_seconds: 60`.
- [x] Создать `configs/config.auth-proxy.prod.yaml`:
  - аналогичная структура с `${JWT_SECRET}`, `${DATABASE_DSN}`.
  > Создан. `auto_migrate: false` (миграциями владеет main-service), `cors.allowed_origins: [https://beepru.ru]`.
- [x] Создать `configs/config.notification-worker.prod.yaml`:
  - `worker.provider: noop` (пока нет APNs/FCM).
  > Создан. `notification_worker.provider: noop`.
- [x] Создать `configs/config.message-expirer.prod.yaml`:
  - `servers.metrics.addr: 0.0.0.0:9101`;
  - `expirer.interval_seconds: 10`.
  > Создан.
- [x] Убедиться, что конфиги читают секреты из env-переменных (cleanenv поддерживает `${VAR}` синтаксис).
  > Подтверждено: `cleanenv.ReadConfig` поддерживает `${VAR}` подстановку. `DATABASE_DSN` и `JWT_SECRET` передаются через env.

---

## 4) docker-compose.prod.yml

- [x] Создать/переписать `deploy/prod/docker-compose.prod.yml` со всеми сервисами:
  - `postgres` (без публичного порта, healthcheck, именованный volume);
  - `main-service` (depends_on postgres, читает prod-конфиг);
  - `auth-proxy` (depends_on postgres);
  - `notification-worker` (depends_on postgres);
  - `message-expirer` (depends_on postgres);
  - `nginx` (ports: 80:80, 443:443, volume с certbot-сертификатами);
  - `certbot` (для получения и обновления SSL-сертификата).
  > Создан полный prod-стек. certbot настроен на автопродление каждые 12 часов через webroot.
- [x] Все сервисы (кроме nginx) — **без** проброса портов наружу.
  > Только nginx имеет `ports: 80:80, 443:443`. Остальные сервисы общаются через внутреннюю сеть.
- [x] Общая docker-сеть `my-chat-net` для всех сервисов.
- [x] Переменные окружения берутся из `.env` файла (`env_file: .env`).
  > Все Go-сервисы используют `env_file: .env`. postgres берёт `POSTGRES_*` переменные напрямую.
- [x] Добавить `restart: unless-stopped` всем сервисам.
- [x] Добавить healthcheck для `postgres` и `main-service`.
  > postgres: `pg_isready`, интервал 10s. main-service: `wget /health`, интервал 15s.
- [x] Создать `.env.example` с placeholder-значениями для всех переменных.
  > Уже создан в пункте 1. Содержит все нужные переменные.

---

## 5) Nginx конфигурация

- [x] Создать `deploy/prod/nginx/nginx.conf` — базовый конфиг nginx.
  > Создан. gzip включён для JSON/text, `server_tokens off`, proxy-буферы настроены.
- [x] Создать `deploy/prod/nginx/conf.d/my-chat.conf`:
  - HTTP → HTTPS redirect (301);
  - HTTPS server block с SSL-сертификатом от Let's Encrypt;
  - `location /api/` → `proxy_pass http://main-service:8080`;
  - `location /ws/` → `proxy_pass http://main-service:8080` с WebSocket headers (`Upgrade`, `Connection`);
  - `location /auth/` → `proxy_pass http://auth-proxy:33081`;
  - `location /health` → `proxy_pass http://main-service:8080`;
  - корректные заголовки: `proxy_set_header Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`.
  > Создан. Используется `set $upstream` + `resolver 127.0.0.11` (Docker DNS) для динамического резолвинга — nginx не кэширует IP при старте.
- [x] Настроить SSL: `ssl_protocols TLSv1.2 TLSv1.3`, `ssl_ciphers` (modern config).
  > Mozilla modern config: ECDHE ciphers, OCSP stapling, session cache, tickets off.
- [x] Добавить gzip-компрессию для JSON/text ответов.
  > В `nginx.conf`: `gzip on`, типы: json, javascript, xml, text/*.
- [x] Добавить security headers: `X-Content-Type-Options`, `X-Frame-Options`, `Strict-Transport-Security`.
  > Добавлены: HSTS (`max-age=63072000; includeSubDomains`), `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `X-XSS-Protection`, `Referrer-Policy`.
- [x] Проверить nginx конфиг локально: `nginx -t`.
  > `nginx -t` через Docker: синтаксис OK. Ошибка SSL-сертификата ожидаема — сертификаты получаются в пункте 6 (`init-ssl.sh`). На VPS пройдёт полностью.

---

## 6) SSL — Let's Encrypt

- [x] Настроить certbot в `docker-compose.prod.yml`:
  - образ `certbot/certbot`;
  - volume `./certbot/conf:/etc/letsencrypt`;
  - volume `./certbot/www:/var/www/certbot`.
  > Настроен в пункте 4.
- [x] Создать скрипт `deploy/prod/init-ssl.sh` — первичное получение сертификата.
  > Создан `deploy/prod/init-ssl.sh`. Стратегия webroot: создаёт временный самоподписанный сертификат → запускает стек → получает реальный сертификат через webroot challenge → перезагружает nginx. Поддерживает флаг `--staging`.
- [x] Настроить автопродление в `docker-compose.prod.yml`.
  > certbot-контейнер: `certbot renew --webroot` каждые 12 часов в loop. Отдельный сервис `nginx-reloader` (образ `docker:cli`) перезагружает nginx через docker socket каждые 12 часов после продления.
- [x] Протестировать с `--staging` флагом (не тратить лимит Let's Encrypt).
  > Выполнено. `curl -k https://beepru.ru/health` → `{"status":"ok"}`.
- [x] После успешного staging — повторить с реальным сертификатом.
  > Выполнено. `curl https://beepru.ru/health` → `{"status":"ok"}` без `-k`. Сертификат доверенный, истекает 2026-10-30.
- [x] Убедиться, что nginx перезагружает конфиг после обновления сертификата.
  > Сервис `nginx-reloader` выполняет `docker exec my-chat-nginx-prod nginx -s reload` каждые 12 часов.

---

## 7) Подготовка VPS (one-time, вручную)

- [x] Выбрать и арендовать VPS (минимум 1 GB RAM, рекомендуется 2 GB).
  > Selectel VPS, IP `87.228.112.251`, SSH: `ssh my-chat` (root).
- [x] Установить Docker Engine (официальный способ: `install.docker.com`).
  > Docker 29.7.1, Docker Compose v5.3.1.
- [x] Добавить пользователя `deploy`: `useradd -m -s /bin/bash deploy && usermod -aG docker deploy`.
  > `uid=1000(deploy) gid=1000(deploy) groups=1000(deploy),989(docker)`.
- [x] Настроить firewall ufw:
  - `ufw allow 22/tcp` (SSH);
  - `ufw allow 80/tcp`;
  - `ufw allow 443/tcp`;
  - `ufw enable`.
  > ufw установлен (`apt install ufw`), статус active, порты 22/80/443 открыты.
- [x] Сгенерировать SSH-ключпара для GitHub Actions: `ssh-keygen -t ed25519 -C "github-actions"`.
  > Ключ сгенерирован в `/tmp/github-actions-key`.
- [x] Добавить публичный ключ в `~deploy/.ssh/authorized_keys` на VPS.
  > Добавлен, права `700/600`, владелец `deploy:deploy`.
- [x] Добавить приватный ключ в GitHub Secrets: `VPS_SSH_KEY`.
  > Добавлен в GitHub Secrets репозитория.
- [x] Добавить в GitHub Secrets: `VPS_HOST` (IP), `VPS_USER` (`deploy`).
  > `VPS_HOST=87.228.112.251`, `VPS_USER=deploy`.
- [x] Клонировать репозиторий: `git clone <repo-url> /opt/my-chat`.
  > Клонирован в `/opt/my-chat`, ветка `main`.
- [x] Создать `/opt/my-chat/.env` по шаблону `.env.example`, заполнить реальными значениями.
  > Заполнены: `POSTGRES_PASSWORD`, `DATABASE_DSN`, `JWT_SECRET`, `METRICS_PASSWORD`.
- [x] Убедиться, что `/opt/my-chat/.env` не попадает в git (проверить `.gitignore`).
  > `git status` на VPS: `nothing to commit, working tree clean`. `.env` в `.gitignore`.
- [x] Добавить `deploy/prod/certbot/` в `.gitignore`.
  > Добавлено в пункте 1.

---

## 8) GitHub Actions — CI workflow

- [x] Создать `.github/workflows/ci.yml`:
  - trigger: `push` + `pull_request` на `main`;
  - job `lint`: `golangci/golangci-lint:v2.12.2` Docker action, `golangci-lint run ./...`;
  - job `test`: `go test -race -short ./...`;
  - job `test-integration`: поднять postgres через `docker compose -f deploy/test/docker-compose.test.yml`, прогнать `go test -tags=integration ./...`;
  - job `build`: `go build ./cmd/...`.
  > Выполнено в Sprint 4. Все 4 джобы присутствуют и проходят на `main`.
- [x] Убедиться, что все jobs проходят на ветке `main`.
  > Подтверждено пользователем — все джобы зелёные.
- [x] Добавить badge CI в README (статус GitHub Actions).
  > Создан `README.md` с бейджем CI → `github.com/FischukSergey/my-chat/actions/workflows/ci.yml`.

---

## 9) GitHub Actions — CD workflow

- [x] Создать `.github/workflows/cd.yml`:
  - trigger: `push` на `main`, только если CI прошел (`needs: ci` или `workflow_run`);
  - job `deploy`:
    - использовать `appleboy/ssh-action` или нативный SSH через `webfactory/ssh-agent`;
    - команды на VPS: `cd /opt/my-chat && git pull origin main`;
    - `docker compose -f deploy/prod/docker-compose.prod.yml build --no-cache`;
    - `docker compose -f deploy/prod/docker-compose.prod.yml up -d --remove-orphans`;
    - `docker image prune -f` (очистить старые образы).
  > Создан. Trigger: `workflow_run` на CI (`completed` + `conclusion == success`). SSH через `appleboy/ssh-action@v1.2.0`. `/opt/my-chat` принадлежит `deploy:deploy`.
- [x] Проверить, что после push в `main` деплой проходит автоматически.
  > Подтверждено: CD workflow отработал после merge в main — `git pull` + `docker compose build` + `up -d` прошли успешно.
- [ ] Добавить Slack/Telegram уведомление при успешном/неуспешном деплое (опционально).

---

## 10) Smoke-тесты prod окружения

- [x] `curl https://beepru.ru/health` → `{"status":"ok"}`.
  > HTTP 200, тело `{"status":"ok"}`.
- [x] `curl -k https://beepru.ru/health` не нужен (сертификат валидный, не staging).
  > TLSv1.3, issuer: Let's Encrypt, `SSL certificate verify ok.`, HSTS заголовок присутствует.
- [x] HTTP → HTTPS редирект работает: `curl -I http://beepru.ru` → HTTP 301.
- [x] Security headers: `Strict-Transport-Security`, `X-Content-Type-Options`, `X-Frame-Options` — все присутствуют.
- [x] `GET /api/v1/me/unread-count` без токена → HTTP 401 `{"error":{"code":"unauthenticated",...}}` (main-service отвечает корректно).
- [x] `GET /ws/connect` без токена → HTTP 401 (WebSocket endpoint доступен и возвращает 401 без upgrade).
- [x] `POST https://beepru.ru/auth/api/v1/auth/login` → получить токены.
  > HTTP 200, access_token + refresh_token получены, expires_in: 900.
  > Баги найдены и исправлены: (1) nginx не срезал `/auth`-префикс — добавлен `rewrite`; (2) `set $upstream_auth` стоял ПОСЛЕ `rewrite+break` и не выполнялся — переставлен выше.
- [x] `GET https://beepru.ru/api/v1/me/unread-count` с Bearer token → 200.
  > HTTP 200, тело `{"unread_count":0}`.
- [x] WebSocket подключение: `wss://beepru.ru/ws/connect` с валидным token → upgrade 101.
  > curl завис в WS-сессии (101 Switching Protocols) — соединение установлено успешно.
- [ ] Открыть мобильный клиент с телефона → нет предупреждений SSL → подключение к WS работает.
- [ ] `docker compose -f deploy/prod/docker-compose.prod.yml ps` на VPS → все сервисы `healthy`.
- [ ] Метрики: `https://beepru.ru/metrics` (или прямой доступ с VPS) → prometheus text format.

---

## 11) Безопасность и финальные проверки

- [x] `git log --all -- '*.env'` → `.env` файла нет в истории git.
  > `git log` вернул пустой вывод — файл `.env` никогда не попадал в историю.
- [x] `cat .gitignore` содержит `.env`, `deploy/prod/certbot/`, `deploy/prod/nginx/ssl/`.
  > Подтверждено: `.env`, `.env.*`, `deploy/prod/certbot/` присутствуют в `.gitignore`.
- [x] Порт 5432 (postgres) недоступен снаружи.
  > `docker ps` показывает `5432/tcp` без `0.0.0.0:` — порт только внутри Docker-сети, снаружи не опубликован.
- [x] Порт 8080 (main-service) недоступен снаружи.
  > `docker ps` показывает `8080/tcp` без `0.0.0.0:` — аналогично, только внутри Docker-сети.
- [x] HSTS заголовок присутствует: `curl -I https://beepru.ru` → `Strict-Transport-Security`.
  > `strict-transport-security: max-age=63072000; includeSubDomains` — присутствует.
- [x] HTTP автоматически редиректит на HTTPS: `curl -I http://beepru.ru` → 301.
  > HTTP/1.1 301 Moved Permanently, Location: https://beepru.ru/health — редирект работает.
- [x] SSL grade на ssllabs.com: минимум B (рекомендуется A).
  > **A+** — максимальная оценка. Certificate, Protocol Support, Key Exchange, Cipher Strength — все 100/100. TLS 1.3, HSTS с длинным сроком, EC 256 bits (SHA384withECDSA), trusted по Mozilla/Apple/Android/Java/Windows.

---

## 12) Критерии готовности (DoD)

- [ ] `https://beepru.ru` открывается без предупреждений на iOS/Android браузере.
  > Проверить вручную на телефоне.
- [ ] `wss://beepru.ru/ws/connect` устанавливает соединение с мобильного браузера.
  > Проверить через мобильный клиент.
- [x] Push в `main` → GitHub Actions зеленый → деплой на VPS автоматически без ручных действий.
  > Подтверждено: несколько CD workflow завершились зелёными после push в main.
- [x] Секреты не попадают в git (`.env` в `.gitignore`, проверено).
  > `git log --all -- '.env'` — пустой вывод. `.gitignore` содержит `.env`, `.env.*`, `deploy/prod/certbot/`.
- [ ] Все 5+ сервисов работают на VPS и показывают `healthy`.
  > Проверить: `docker compose -f deploy/prod/docker-compose.prod.yml ps` на VPS.
- [x] Старые docker-образы автоматически очищаются после деплоя.
  > CD workflow выполняет `docker image prune -f` после каждого деплоя.
- [x] `docs/sprint-5-plan.md` и этот чеклист актуализированы.
  > Чеклист обновлён по итогам всех выполненных пунктов спринта.

---

## 13) Демо

- [ ] Открыть `https://beepru.ru` на реальном телефоне → нет предупреждений SSL.
- [ ] Войти через мобильный клиент → отправить сообщение → получить в реальном времени.
- [x] Показать GitHub Actions: зеленый CI + зеленый CD после последнего push.
  > Несколько успешных прогонов CI + CD подтверждены в ходе спринта.
- [ ] Показать `docker compose ps` на VPS — все сервисы running/healthy.
- [x] Зафиксировать known limitations Sprint 5 (`docs/known-limitations-sprint-5.md`).
  > Файл создан.

---

## 14) Улучшения мобильного клиента (backlog из Sprint 4)

- [ ] **Page Visibility API**: вызывать `markRead` только если диалог видим (`document.hidden === false`).
  - При открытии диалога: проверять `document.hidden` перед вызовом `markRead`.
  - При получении `message_new` в открытом диалоге: аналогично.
  - Добавить обработчик `document.addEventListener("visibilitychange", ...)`: при возврате фокуса дочитывать непрочитанные сообщения текущего диалога.
  - Актуально для браузера и PWA: сообщения не считаются прочитанными, пока вкладка свёрнута или скрыта.

---

**Sprint 5 — IN PROGRESS** (ожидает: ручная проверка с телефона, `docker compose ps` на VPS)
