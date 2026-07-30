# Sprint 5 — детальный план (Production Deployment)

Источник: архитектурный план `docs/chat-architecture-plan.md` §12 + решения, принятые по итогам Sprint 4.

## 1) Цель спринта

Развернуть все сервисы my-chat на боевом VPS таким образом, чтобы:

- мобильный клиент открывался через HTTPS без предупреждений браузера;
- WebSocket работал по `wss://`;
- деплой новой версии занимал одну команду (автоматически из GitHub Actions при push в `main`);
- секреты (пароли БД, JWT-ключ) не попадали в git.

## 2) Исходные данные и принятые решения

| Вопрос | Решение |
|--------|---------|
| Домен | Зарегистрировать любой (`.ru`, `.online` и т.д.), добавить A-запись → IP VPS |
| SSL | Let's Encrypt + Certbot в Docker (автопродление через certbot renew) |
| Reverse proxy | Nginx в Docker, единственный сервис с публичными портами 80/443 |
| Container Registry | Не используем — образы собираются прямо на VPS (`docker compose build`) |
| Инфраструктура | Один VPS, минимум 1 GB RAM (рекомендуется 2 GB) |
| CD | GitHub Actions → SSH на VPS → `git pull` + `docker compose build` + `up -d` |
| Секреты на VPS | `.env` файл в `/opt/my-chat/.env` (не коммитится в git) |
| Секреты в CI | GitHub Secrets: `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` |

## 3) Архитектура на VPS

```
Internet
    │
    ▼
  Nginx (443/80)            ← единственный публичный сервис
    │
    ├──► main-service:8080   (API + WebSocket)
    ├──► auth-proxy:33081    (Auth endpoints)
    └──► [prometheus:9090]   (опционально, с basic auth)

Внутренняя docker-сеть:
    ├── postgres:5432
    ├── notification-worker
    └── message-expirer
```

Все сервисы, кроме nginx, работают **только внутри docker-сети**. Прямого доступа снаружи нет.

## 4) Маршрутизация nginx

```
https://yourdomain.com/api/v1/...    → main-service:8080
https://yourdomain.com/ws/connect    → main-service:8080  (WebSocket upgrade)
https://yourdomain.com/auth/...      → auth-proxy:33081
https://yourdomain.com/health        → main-service:8080
```

Либо вариант с поддоменами:
```
https://api.yourdomain.com/...       → main-service:8080
https://auth.yourdomain.com/...      → auth-proxy:33081
```

Выбор фиксируется при реализации чеклиста (зависит от структуры mobile-клиента).

## 5) Структура файлов, которые появятся в репозитории

```
deploy/prod/
    docker-compose.prod.yml       # полный prod-стек
    nginx/
        nginx.conf                # основной конфиг
        conf.d/
            main-service.conf     # location правила
    .env.example                  # шаблон для .env на VPS (без секретов)

configs/
    config.main-service.prod.yaml       # prod конфиг (DSN через env var)
    config.auth-proxy.prod.yaml
    config.notification-worker.prod.yaml
    config.message-expirer.prod.yaml

.github/
    workflows/
        ci.yml                    # lint + test + build (Sprint 4, пункт 11)
        cd.yml                    # deploy on push to main

Dockerfiles (уже есть или обновятся):
    prod.Dockerfile                     # main-service (уже есть)
    auth-proxy.Dockerfile               # (уже есть)
    notification-worker.Dockerfile      # (уже есть)
    message-expirer.Dockerfile          # (уже есть)
```

Файл `.env` на VPS **не коммитится**, только `.env.example` как шаблон.

## 6) Содержимое .env на VPS

```env
# PostgreSQL
POSTGRES_PASSWORD=...

# JWT
JWT_SECRET=...

# Domain
DOMAIN=yourdomain.com

# Notification worker
PUSH_PROVIDER=noop   # пока нет реальных APNs/FCM credentials

# Prometheus (basic auth для nginx)
METRICS_USER=admin
METRICS_PASSWORD=...
```

## 7) Схема CD-деплоя (GitHub Actions)

```
Push → main
    │
    ▼
CI workflow (ci.yml):
    ├── job: lint
    ├── job: test
    └── job: build
        │  (все зеленые?)
        ▼
CD workflow (cd.yml):
    └── job: deploy
        ├── SSH connect к VPS
        ├── cd /opt/my-chat
        ├── git pull origin main
        ├── docker compose -f deploy/prod/docker-compose.prod.yml build --no-cache
        └── docker compose -f deploy/prod/docker-compose.prod.yml up -d --remove-orphans
```

CD запускается только если CI прошел (зависимость через `needs: ci` или отдельный workflow).

## 8) Подготовка VPS (one-time, вручную)

Эти шаги выполняются один раз на VPS до первого деплоя:

1. Установить Docker Engine + Docker Compose plugin
2. Добавить пользователя `deploy` с доступом к Docker (без root-сессии в prod)
3. Добавить SSH публичный ключ GitHub Actions в `~/.ssh/authorized_keys`
4. Склонировать репозиторий: `git clone ... /opt/my-chat`
5. Создать `/opt/my-chat/.env` по шаблону `.env.example`, заполнить секреты
6. Настроить firewall (ufw): открыть 22 (SSH), 80, 443; закрыть всё остальное
7. Первый запуск: `docker compose -f deploy/prod/docker-compose.prod.yml up -d`
8. Получить SSL-сертификат: certbot через docker (первый раз в standalone mode)

## 9) Definition of Done Sprint 5

Спринт считается завершенным, если:

- `https://yourdomain.com` открывается без предупреждений SSL;
- `wss://yourdomain.com/ws/connect` устанавливает соединение с мобильного браузера;
- `POST https://yourdomain.com/auth/api/v1/auth/login` возвращает токены;
- Push в `main` → GitHub Actions → автодеплой на VPS без ручных действий;
- `.env` файл не попадает в git (проверено через `git status`);
- `docker compose ps` на VPS показывает все сервисы `healthy`;
- Метрики доступны по `https://yourdomain.com/metrics` (или отдельный поддомен) с basic auth.

## 10) Риски и меры

| Риск | Мера |
|------|------|
| Let's Encrypt rate limit (5 сертификатов/неделю на домен) | Сначала тестировать с `--staging` флагом |
| Сборка образов на VPS занимает много RAM/CPU | Добавить `--memory` лимит в docker build, или собирать по одному образу |
| Секрет попал в git | `.gitignore` для `.env*` (кроме `.env.example`), pre-commit hook |
| SSH ключ GitHub Actions скомпрометирован | Использовать отдельного пользователя `deploy` с ограниченными правами, не root |
| Nginx не поднимается из-за отсутствия SSL-сертификата при первом запуске | Первый run — certbot в standalone mode до запуска nginx, затем switch на webroot/docker |
