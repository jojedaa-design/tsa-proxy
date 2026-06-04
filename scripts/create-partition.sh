#!/bin/bash
# =============================================================
# Crear particiones mensuales para el mes siguiente
# Cron recomendado: 0 12 25 * * /opt/tsa-proxy/scripts/create-partition.sh
# =============================================================
set -euo pipefail

CONTAINER="${POSTGRES_CONTAINER:-tsa_postgres}"
DB="${POSTGRES_DB:-tsaproxy}"
USER="${POSTGRES_USER:-tsaproxy}"

# Calcular mes siguiente
NEXT_MONTH=$(date -d "+1 month" +%Y-%m-01 2>/dev/null || date -v+1m +%Y-%m-01)
START_DATE="${NEXT_MONTH}"
END_DATE=$(date -d "${START_DATE} +1 month" +%Y-%m-01 2>/dev/null || date -v+1m -j -f "%Y-%m-%d" "$START_DATE" +%Y-%m-01)
SUFFIX=$(echo "$NEXT_MONTH" | sed 's/-01$//' | sed 's/-/_/g')

echo "[$(date -Iseconds)] Creando particiones para ${SUFFIX} (${START_DATE} → ${END_DATE})"

docker exec -i "$CONTAINER" psql -U "$USER" -d "$DB" << SQL
DO \$\$
BEGIN
  -- usage_events
  BEGIN
    EXECUTE 'CREATE TABLE IF NOT EXISTS usage_events_${SUFFIX}
             PARTITION OF usage_events
             FOR VALUES FROM (''${START_DATE}'') TO (''${END_DATE}'')';
    RAISE NOTICE 'Creada: usage_events_${SUFFIX}';
  EXCEPTION WHEN duplicate_table THEN
    RAISE NOTICE 'Ya existe: usage_events_${SUFFIX}';
  END;

  -- proxy_requests
  BEGIN
    EXECUTE 'CREATE TABLE IF NOT EXISTS proxy_requests_${SUFFIX}
             PARTITION OF proxy_requests
             FOR VALUES FROM (''${START_DATE}'') TO (''${END_DATE}'')';
    RAISE NOTICE 'Creada: proxy_requests_${SUFFIX}';
  EXCEPTION WHEN duplicate_table THEN
    RAISE NOTICE 'Ya existe: proxy_requests_${SUFFIX}';
  END;
END \$\$;
SQL

echo "[$(date -Iseconds)] Particiones listas para ${SUFFIX}"
