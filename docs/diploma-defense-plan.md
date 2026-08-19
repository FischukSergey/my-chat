# План подготовки к защите диплома

Цель: преподаватель клонирует репозиторий, поднимает стек **одним Docker Compose** и проверяет чат в браузере. Без Node, Go, VPS и Capacitor.

Сейчас так нельзя: local compose — только бэкенд; PWA отдаётся лишь в prod (nginx); `configs/config.*.docker.local.yaml` в `.gitignore`; seed `alice`/`bob` вручную; `/debug` логинится по `user_id` (сломан с Sprint 6).

Не смешивать с DoD Sprint 9 (шифрование тел в БД).

---

## 1) Local «как prod», но HTTP

- Закоммитить docker-local конфиги (или `*.example` + одна команда `cp`). Учебные секреты допустимы.
- Сервис PWA в `deploy/local/docker-compose.local.yml`: образ `mobile/Dockerfile` + nginx без TLS, порт например **3000**.
  - `/` → статика; `/api/v1/auth/` → auth-proxy; `/api/`, `/ws/` → main-service (как prod).
- Автоseed пользователей при первом старте (`alice` / `bob` / `password123`). Диалог не сидировать — преподаватель создаёт сам.
- TTL в local docker: **30–60 с** (чтобы исчезновение сообщений было видно на защите).
- `task local:up` / raw `docker compose` — оба варианта в инструкции.

Проверка: чистый clone (или `local:down:clean`) → compose up → `http://localhost:3000`.

## 2) Инструкция для преподавателя

Одна страница в корне или `docs/teacher-local.md`:

- Требования: Docker Desktop (Task — опционально).
- Команды запуска / остановки.
- Учётки и URL.
- Сценарий проверки (ниже).
- Чего не ждать локально: Web Push, «На экран Домой» на iPhone, Face ID. Это — на `https://beepru.ru`.

Коротко обновить корневой `README.md` (сейчас «быстрый старт» про host-конфиги, не про Docker+PWA).

## 3) Сценарий проверки (15–20 мин)

Два окна Chrome: обычное + инкогнито.

1. Clone → compose up → открыть PWA.
2. Alice: login → PIN 4 цифры.
3. Bob: login → другой PIN.
4. Alice: «Новый чат» → username `bob`.
5. Переписка в обе стороны (WS).
6. Bob читает → таймер → сообщение пропадает (TTL).
7. Свернуть вкладку Alice > grace (60 с) → снова PIN.
8. *Опционально:* Register третьего пользователя.
9. *На проекторе:* телефон / `beepru.ru` — установленная PWA и push.

Не включать в обязательный прогон: `/debug`, curl, Prometheus, симулятор iOS.

## 4) Прогон до защиты

- Сценарий на «чистой» машине или после `local:down:clean`.
- Запасной план: живой prod, если у комиссии нет Docker.
- Не тащить в инструкцию prod SSH/секреты.

## Вне scope этого плана

- Кастомный pull-to-refresh.
- Починка `/debug` (не нужна, если PWA в Docker есть).
- Sprint 9 и новые фичи.
