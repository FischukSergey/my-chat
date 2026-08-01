#!/usr/bin/env bash
# =============================================================================
# init-ssl.sh — первичное получение SSL-сертификата от Let's Encrypt
#
# Стратегия (standalone):
#   1. Поднять postgres и Go-сервисы (без nginx — порт 80 свободен)
#   2. Certbot в standalone-режиме сам слушает порт 80 и проходит challenge
#   3. Запустить nginx с реальным сертификатом
#   4. Запустить certbot-loop для автопродления
#
# Использование:
#   cd /opt/my-chat/deploy/prod
#   bash init-ssl.sh [--staging]      # --staging: тест без расхода лимита LE
#   bash init-ssl.sh                  # боевой сертификат
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.prod.yml"

DOMAIN="beepru.ru"
EMAIL="admin@beepru.ru"     # замените на реальный email для уведомлений LE

CERTBOT_FLAGS="--non-interactive --agree-tos --email ${EMAIL}"
STAGING=""

# ── Разобрать аргументы ──────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --staging)
      STAGING="--staging"
      echo "⚠️  Режим staging: сертификат будет тестовым (браузер покажет предупреждение)"
      ;;
    *)
      echo "Неизвестный аргумент: $arg" >&2
      exit 1
      ;;
  esac
done

COMPOSE="docker compose -f ${COMPOSE_FILE} --env-file ${REPO_ROOT}/.env"

echo "==> Рабочая директория: ${SCRIPT_DIR}"
cd "${SCRIPT_DIR}"

# ── Шаг 1: Убедиться что nginx остановлен (нужен свободный порт 80) ──────────
echo "==> Шаг 1: останавливаем nginx (освобождаем порт 80 для certbot)..."
${COMPOSE} stop nginx 2>/dev/null || true

# ── Шаг 2: Поднять postgres и Go-сервисы ─────────────────────────────────────
echo "==> Шаг 2: запускаем postgres и Go-сервисы..."
${COMPOSE} up -d postgres
echo "    Ждём готовности postgres..."
${COMPOSE} exec postgres sh -c "until pg_isready -U \${POSTGRES_USER} -d \${POSTGRES_DB}; do sleep 1; done"

${COMPOSE} up -d main-service auth-proxy notification-worker message-expirer
echo "    Ждём готовности main-service (healthcheck)..."
for i in $(seq 1 30); do
  STATUS=$(${COMPOSE} ps --format json main-service 2>/dev/null | grep -o '"Health":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
  if [ "${STATUS}" = "healthy" ]; then
    echo "    main-service: healthy"
    break
  fi
  echo "    main-service: ${STATUS:-waiting} (${i}/30)..."
  sleep 3
done

# ── Шаг 3: Получить сертификат через certbot standalone ──────────────────────
echo "==> Шаг 3: получаем сертификат от Let's Encrypt${STAGING:+ (staging)} через standalone..."

mkdir -p "${SCRIPT_DIR}/certbot/conf"
mkdir -p "${SCRIPT_DIR}/certbot/www"

docker run --rm \
  -v "${SCRIPT_DIR}/certbot/conf:/etc/letsencrypt" \
  -v "${SCRIPT_DIR}/certbot/www:/var/www/certbot" \
  -p 80:80 \
  certbot/certbot:latest certonly \
    --standalone \
    --domain "${DOMAIN}" \
    ${CERTBOT_FLAGS} \
    ${STAGING}

echo "    Сертификат получен."

# ── Шаг 4: Запустить nginx с реальным сертификатом ───────────────────────────
echo "==> Шаг 4: запускаем nginx с реальным сертификатом..."
${COMPOSE} up -d nginx
sleep 3
echo "    nginx запущен."

# ── Шаг 5: Запустить certbot-loop и nginx-reloader ───────────────────────────
echo "==> Шаг 5: запускаем certbot-loop и nginx-reloader..."
${COMPOSE} up -d certbot nginx-reloader
echo "    Автопродление настроено."

echo ""
echo "✅  Готово! Проверьте:"
echo "    curl -k https://${DOMAIN}/health"
echo ""
if [ -n "${STAGING}" ]; then
  echo "⚠️  Staging сертификат. Для получения боевого сертификата:"
  echo "    bash ${BASH_SOURCE[0]}"
fi
