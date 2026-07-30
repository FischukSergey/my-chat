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

- [ ] Проверить и при необходимости дополнить `prod.Dockerfile` (main-service):
  - multi-stage build (golang:alpine → alpine);
  - копирование конфигов в образ;
  - `EXPOSE 8080`.
- [ ] Проверить `auth-proxy.Dockerfile` (аналогичная структура).
- [ ] Проверить `notification-worker.Dockerfile`.
- [ ] Проверить `message-expirer.Dockerfile`.
- [ ] Убедиться, что все Dockerfile используют `go 1.25` (или текущую версию из `go.mod`).
- [ ] Проверить `go build ./...` внутри каждого Dockerfile — сборка без ошибок.

---

## 3) Prod конфиги сервисов

- [ ] Создать `configs/config.main-service.prod.yaml`:
  - `servers.client.addr: 0.0.0.0:8080`;
  - `servers.metrics.addr: 0.0.0.0:9100`;
  - `database.dsn` через env-переменную `${DATABASE_DSN}`;
  - `jwt.secret` через env-переменную `${JWT_SECRET}`;
  - `log.level: info`, `log.format: json`;
  - `chat.message_ttl_seconds: 300`;
  - `cors.allowed_origins` — перечислить prod домен.
- [ ] Создать `configs/config.auth-proxy.prod.yaml`:
  - аналогичная структура с `${JWT_SECRET}`, `${DATABASE_DSN}`.
- [ ] Создать `configs/config.notification-worker.prod.yaml`:
  - `worker.provider: noop` (пока нет APNs/FCM).
- [ ] Создать `configs/config.message-expirer.prod.yaml`:
  - `servers.metrics.addr: 0.0.0.0:9101`;
  - `expirer.interval_seconds: 10`.
- [ ] Убедиться, что конфиги читают секреты из env-переменных (cleanenv поддерживает `${VAR}` синтаксис).

---

## 4) docker-compose.prod.yml

- [ ] Создать/переписать `deploy/prod/docker-compose.prod.yml` со всеми сервисами:
  - `postgres` (без публичного порта, healthcheck, именованный volume);
  - `main-service` (depends_on postgres, читает prod-конфиг);
  - `auth-proxy` (depends_on postgres);
  - `notification-worker` (depends_on postgres);
  - `message-expirer` (depends_on postgres);
  - `nginx` (ports: 80:80, 443:443, volume с certbot-сертификатами);
  - `certbot` (для получения и обновления SSL-сертификата).
- [ ] Все сервисы (кроме nginx) — **без** проброса портов наружу.
- [ ] Общая docker-сеть `my-chat-net` для всех сервисов.
- [ ] Переменные окружения берутся из `.env` файла (`env_file: .env`).
- [ ] Добавить `restart: unless-stopped` всем сервисам.
- [ ] Добавить healthcheck для `postgres` и `main-service`.
- [ ] Создать `.env.example` с placeholder-значениями для всех переменных.

---

## 5) Nginx конфигурация

- [ ] Создать `deploy/prod/nginx/nginx.conf` — базовый конфиг nginx.
- [ ] Создать `deploy/prod/nginx/conf.d/my-chat.conf`:
  - HTTP → HTTPS redirect (301);
  - HTTPS server block с SSL-сертификатом от Let's Encrypt;
  - `location /api/` → `proxy_pass http://main-service:8080`;
  - `location /ws/` → `proxy_pass http://main-service:8080` с WebSocket headers (`Upgrade`, `Connection`);
  - `location /auth/` → `proxy_pass http://auth-proxy:33081`;
  - `location /health` → `proxy_pass http://main-service:8080`;
  - корректные заголовки: `proxy_set_header Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`.
- [ ] Настроить SSL: `ssl_protocols TLSv1.2 TLSv1.3`, `ssl_ciphers` (modern config).
- [ ] Добавить gzip-компрессию для JSON/text ответов.
- [ ] Добавить security headers: `X-Content-Type-Options`, `X-Frame-Options`, `Strict-Transport-Security`.
- [ ] Проверить nginx конфиг локально: `nginx -t`.

---

## 6) SSL — Let's Encrypt

- [ ] Настроить certbot в `docker-compose.prod.yml`:
  - образ `certbot/certbot`;
  - volume `./certbot/conf:/etc/letsencrypt`;
  - volume `./certbot/www:/var/www/certbot`.
- [ ] Создать скрипт `deploy/prod/init-ssl.sh` — первичное получение сертификата:
  - остановить nginx, запустить certbot в standalone mode, запустить nginx;
  - или: использовать webroot challenge через nginx.
- [ ] Настроить автопродление в `docker-compose.prod.yml`:
  - `command: renew --webroot -w /var/www/certbot` (запускать через cron или timer).
- [ ] Протестировать с `--staging` флагом (не тратить лимит Let's Encrypt).
- [ ] После успешного staging — повторить с реальным сертификатом.
- [ ] Убедиться, что nginx перезагружает конфиг после обновления сертификата.

---

## 7) Подготовка VPS (one-time, вручную)

- [ ] Выбрать и арендовать VPS (минимум 1 GB RAM, рекомендуется 2 GB).
- [ ] Установить Docker Engine (официальный способ: `install.docker.com`).
- [ ] Добавить пользователя `deploy`: `useradd -m -s /bin/bash deploy && usermod -aG docker deploy`.
- [ ] Настроить firewall ufw:
  - `ufw allow 22/tcp` (SSH);
  - `ufw allow 80/tcp`;
  - `ufw allow 443/tcp`;
  - `ufw enable`.
- [ ] Сгенерировать SSH-ключпара для GitHub Actions: `ssh-keygen -t ed25519 -C "github-actions"`.
- [ ] Добавить публичный ключ в `~deploy/.ssh/authorized_keys` на VPS.
- [ ] Добавить приватный ключ в GitHub Secrets: `VPS_SSH_KEY`.
- [ ] Добавить в GitHub Secrets: `VPS_HOST` (IP), `VPS_USER` (`deploy`).
- [ ] Клонировать репозиторий: `git clone <repo-url> /opt/my-chat`.
- [ ] Создать `/opt/my-chat/.env` по шаблону `.env.example`, заполнить реальными значениями.
- [ ] Убедиться, что `/opt/my-chat/.env` не попадает в git (проверить `.gitignore`).
- [ ] Добавить `deploy/prod/certbot/` в `.gitignore`.

---

## 8) GitHub Actions — CI workflow

- [ ] Создать `.github/workflows/ci.yml`:
  - trigger: `push` + `pull_request` на `main`;
  - job `lint`: `golangci/golangci-lint:v2.12.2` Docker action, `golangci-lint run ./...`;
  - job `test`: `go test -race -short ./...`;
  - job `test-integration`: поднять postgres через `docker compose -f deploy/test/docker-compose.test.yml`, прогнать `go test -tags=integration ./...`;
  - job `build`: `go build ./cmd/...`.
- [ ] Убедиться, что все jobs проходят на ветке `main`.
- [ ] Добавить badge CI в README (статус GitHub Actions).

---

## 9) GitHub Actions — CD workflow

- [ ] Создать `.github/workflows/cd.yml`:
  - trigger: `push` на `main`, только если CI прошел (`needs: ci` или `workflow_run`);
  - job `deploy`:
    - использовать `appleboy/ssh-action` или нативный SSH через `webfactory/ssh-agent`;
    - команды на VPS: `cd /opt/my-chat && git pull origin main`;
    - `docker compose -f deploy/prod/docker-compose.prod.yml build --no-cache`;
    - `docker compose -f deploy/prod/docker-compose.prod.yml up -d --remove-orphans`;
    - `docker image prune -f` (очистить старые образы).
- [ ] Проверить, что после push в `main` деплой проходит автоматически.
- [ ] Добавить Slack/Telegram уведомление при успешном/неуспешном деплое (опционально).

---

## 10) Smoke-тесты prod окружения

- [ ] `curl https://beepru.ru/health` → `{"status":"ok"}`.
- [ ] `curl -k https://beepru.ru/health` не нужен (сертификат валидный, не staging).
- [ ] `POST https://beepru.ru/auth/api/v1/auth/login` → получить токены.
- [ ] `GET https://beepru.ru/api/v1/me/unread-count` с Bearer token → 200.
- [ ] WebSocket подключение: `wss://beepru.ru/ws/connect` с валидным token → upgrade 101.
- [ ] Открыть мобильный клиент с телефона → нет предупреждений SSL → подключение к WS работает.
- [ ] `docker compose -f deploy/prod/docker-compose.prod.yml ps` на VPS → все сервисы `healthy`.
- [ ] Метрики: `https://beepru.ru/metrics` (или прямой доступ с VPS) → prometheus text format.

---

## 11) Безопасность и финальные проверки

- [ ] `git log --all -- '*.env'` → `.env` файла нет в истории git.
- [ ] `cat .gitignore` содержит `.env`, `deploy/prod/certbot/`, `deploy/prod/nginx/ssl/`.
- [ ] Порт 5432 (postgres) недоступен снаружи: `curl VPS_IP:5432` — connection refused.
- [ ] Порт 8080 (main-service) недоступен снаружи: `curl VPS_IP:8080` — connection refused.
- [ ] HSTS заголовок присутствует: `curl -I https://beepru.ru` → `Strict-Transport-Security`.
- [ ] HTTP автоматически редиректит на HTTPS: `curl -I http://beepru.ru` → 301.
- [ ] SSL grade на ssllabs.com: минимум B (рекомендуется A).

---

## 12) Критерии готовности (DoD)

- [ ] `https://beepru.ru` открывается без предупреждений на iOS/Android браузере.
- [ ] `wss://beepru.ru/ws/connect` устанавливает соединение с мобильного браузера.
- [ ] Push в `main` → GitHub Actions зеленый → деплой на VPS автоматически без ручных действий.
- [ ] Секреты не попадают в git (`.env` в `.gitignore`, проверено).
- [ ] Все 5+ сервисов работают на VPS и показывают `healthy`.
- [ ] Старые docker-образы автоматически очищаются после деплоя.
- [ ] `docs/sprint-5-plan.md` и этот чеклист актуализированы.

---

## 13) Демо

- [ ] Открыть `https://beepru.ru` на реальном телефоне → нет предупреждений SSL.
- [ ] Войти через мобильный клиент → отправить сообщение → получить в реальном времени.
- [ ] Показать GitHub Actions: зеленый CI + зеленый CD после последнего push.
- [ ] Показать `docker compose ps` на VPS — все сервисы running/healthy.
- [ ] Зафиксировать known limitations Sprint 5 (`docs/known-limitations-sprint-5.md`).

---

## 14) Улучшения мобильного клиента (backlog из Sprint 4)

- [ ] **Page Visibility API**: вызывать `markRead` только если диалог видим (`document.hidden === false`).
  - При открытии диалога: проверять `document.hidden` перед вызовом `markRead`.
  - При получении `message_new` в открытом диалоге: аналогично.
  - Добавить обработчик `document.addEventListener("visibilitychange", ...)`: при возврате фокуса дочитывать непрочитанные сообщения текущего диалога.
  - Актуально для браузера и PWA: сообщения не считаются прочитанными, пока вкладка свёрнута или скрыта.

---

**Sprint 5 — PLANNED**
