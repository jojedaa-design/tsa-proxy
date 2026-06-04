#!/bin/bash
# =============================================================
# Renovación automática de certificados Let's Encrypt
# Cron recomendado: 0 3 1,15 * * /opt/tsa-proxy/scripts/renew-certs.sh
# =============================================================
set -euo pipefail

COMPOSE_DIR="${COMPOSE_DIR:-/opt/tsa-proxy}"
CERT_LIVE="/etc/letsencrypt/live"
CERT_VOL="/var/lib/docker/volumes/tsa-proxy_nginx-certs/_data/live"
LOG_TAG="tsa-renew-certs"

log() { echo "[$(date -Iseconds)] $*" | tee -a /var/log/tsaproxy-renew.log; }

log "Iniciando renovación de certificados..."

# Detener nginx para liberar el puerto 80 (standalone challenge)
cd "$COMPOSE_DIR"
docker compose stop nginx
log "Nginx detenido"

# Renovar certificados
if certbot renew --non-interactive --quiet 2>&1 | tee -a /var/log/tsaproxy-renew.log; then
    log "Certbot ejecutado OK"
else
    log "ERROR: certbot renew falló"
    docker compose start nginx
    exit 1
fi

# Detectar el directorio con los certs activos
# Puede ser tsa.bigdavi.com o tsa.bigdavi.com-0001, etc.
CERT_DIR=$(ls -dt "${CERT_LIVE}/tsa.bigdavi.com"* 2>/dev/null | head -1)
if [ -z "$CERT_DIR" ]; then
    log "ERROR: No se encontraron certificados en ${CERT_LIVE}"
    docker compose start nginx
    exit 1
fi
log "Usando certs de: $CERT_DIR"

# Copiar certificados al volumen Docker
for DOMAIN in tsa.bigdavi.com ast.bigdavi.com; do
    TARGET="${CERT_VOL}/${DOMAIN}"
    mkdir -p "$TARGET"
    cp "${CERT_DIR}/fullchain.pem" "${TARGET}/fullchain.pem"
    cp "${CERT_DIR}/privkey.pem"   "${TARGET}/privkey.pem"
    chmod 644 "${TARGET}/fullchain.pem"
    chmod 600 "${TARGET}/privkey.pem"
    log "Certs copiados → ${TARGET}"
done

# Reiniciar nginx con los nuevos certificados
docker compose start nginx
log "Nginx reiniciado"

# Verificar que nginx responde
sleep 3
if curl -sf --max-time 5 https://tsa.bigdavi.com/health > /dev/null 2>&1; then
    log "✓ tsa.bigdavi.com responde OK con nuevos certs"
else
    log "ADVERTENCIA: tsa.bigdavi.com no responde tras renovación"
fi

# Mostrar fecha de expiración del nuevo cert
EXPIRY=$(openssl x509 -noout -enddate -in "${CERT_DIR}/fullchain.pem" 2>/dev/null | cut -d= -f2 || echo "desconocida")
log "Cert válido hasta: ${EXPIRY}"

log "Renovación completada."
