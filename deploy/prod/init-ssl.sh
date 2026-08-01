#!/usr/bin/env bash
# =============================================================================
# init-ssl.sh — первичное получение SSL-сертификата от Let's Encrypt
#
# Решает проблему «курица и яйцо»:
#   nginx не стартует без сертификата,
#   certbot не может пройти webroot challenge без nginx.
#
# Стратегия:
#   1. Создать временный самоподписанный сертификат → nginx стартует
#   2. Запустить весь стек (кроме certbot-loop)
#   3. Получить реальный сертификат через webroot challenge
#   4. Перезагрузить nginx с реальным сертификатом
#   5. Запустить certbot-loop для автопродления
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

CERT_PATH="${SCRIPT_DIR}/certbot/conf/live/${DOMAIN}"
COMPOSE="docker compose -f ${COMPOSE_FILE}"

echo "==> Рабочая директория: ${SCRIPT_DIR}"
cd "${SCRIPT_DIR}"

# ── Шаг 1: Временный самоподписанный сертификат ──────────────────────────────
if [ ! -f "${CERT_PATH}/fullchain.pem" ]; then
  echo "==> Шаг 1: создаём временный самоподписанный сертификат..."

  mkdir -p "${CERT_PATH}"

  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${CERT_PATH}/privkey.pem" \
    -out    "${CERT_PATH}/fullchain.pem" \
    -days   1 \
    -subj   "/CN=${DOMAIN}" \
    2>/dev/null

  # chain.pem нужен для ssl_trusted_certificate
  cp "${CERT_PATH}/fullchain.pem" "${CERT_PATH}/chain.pem"

  echo "    Временный сертификат создан."
else
  echo "==> Шаг 1: сертификат уже существует, пропускаем создание временного."
fi

# ── Шаг 2: Поднять весь стек (certbot-loop не стартует — зависит от nginx) ──
echo "==> Шаг 2: запускаем стек (postgres, сервисы, nginx)..."
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

${COMPOSE} up -d nginx
sleep 2
echo "    nginx запущен."

# ── Шаг 3: Получить реальный сертификат через webroot ────────────────────────
echo "==> Шаг 3: получаем сертификат от Let's Encrypt${STAGING:+ (staging)}..."

mkdir -p "${SCRIPT_DIR}/certbot/www"

docker run --rm \
  -v "${SCRIPT_DIR}/certbot/conf:/etc/letsencrypt" \
  -v "${SCRIPT_DIR}/certbot/www:/var/www/certbot" \
  --network my-chat-prod_my-chat-net \
  certbot/certbot:latest certonly \
    --webroot \
    --webroot-path /var/www/certbot \
    --domain "${DOMAIN}" \
    ${CERTBOT_FLAGS} \
    ${STAGING} \
    --force-renewal

echo "    Сертификат получен."

# ── Шаг 4: Перезагрузить nginx с реальным сертификатом ───────────────────────
echo "==> Шаг 4: перезагружаем nginx с реальным сертификатом..."
${COMPOSE} exec nginx nginx -s reload
echo "    nginx перезагружен."

# ── Шаг 5: Запустить certbot-loop для автопродления ──────────────────────────
echo "==> Шаг 5: запускаем certbot-loop для автопродления..."
${COMPOSE} up -d certbot
echo "    certbot-loop запущен."

echo ""
echo "✅  Готово! Проверьте:"
echo "    curl https://${DOMAIN}/health"
echo ""
if [ -n "${STAGING}" ]; then
  echo "⚠️  Staging сертификат. Для получения боевого сертификата:"
  echo "    bash ${BASH_SOURCE[0]}"
fi
