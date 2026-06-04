#!/bin/bash
# =============================================================
# Backup de PostgreSQL con retención configurable
# Ejecutar via cron: 0 2 * * * /opt/tsa-proxy/scripts/backup-db.sh
# =============================================================
set -euo pipefail

BACKUP_DIR="${BACKUP_DEST_DIR:-/var/backups/tsaproxy}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
CONTAINER="${POSTGRES_CONTAINER:-tsa_postgres}"
DB="${POSTGRES_DB:-tsaproxy}"
USER="${POSTGRES_USER:-tsaproxy}"
PGPASSWORD="${POSTGRES_PASSWORD:-}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
FILENAME="tsaproxy_${TIMESTAMP}.sql.gz"
FILEPATH="${BACKUP_DIR}/${FILENAME}"

mkdir -p "$BACKUP_DIR"

echo "[$(date -Iseconds)] Iniciando backup → ${FILEPATH}"

# Dump comprimido directamente desde el contenedor
docker exec -e PGPASSWORD="$PGPASSWORD" "$CONTAINER" \
  pg_dump -U "$USER" -d "$DB" --no-password --clean --if-exists \
  | gzip -9 > "$FILEPATH"

SIZE=$(du -sh "$FILEPATH" | cut -f1)
echo "[$(date -Iseconds)] Backup completado: ${SIZE}"

# Eliminar backups más antiguos que RETENTION_DAYS días
DELETED=$(find "$BACKUP_DIR" -name "tsaproxy_*.sql.gz" -mtime +${RETENTION_DAYS} -print -delete | wc -l)
if [ "$DELETED" -gt 0 ]; then
  echo "[$(date -Iseconds)] Eliminados ${DELETED} backups expirados (>${RETENTION_DAYS} días)"
fi

echo "[$(date -Iseconds)] Backup finalizado OK"
