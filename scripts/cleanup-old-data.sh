#!/bin/bash
# =============================================================
# Limpieza manual de datos con más de 1 año
# El backend lo hace automáticamente cada día a las 03:00 UTC.
# Este script es para ejecución manual si se necesita.
# =============================================================
set -euo pipefail

CONTAINER="${POSTGRES_CONTAINER:-tsa_postgres}"
DB="${POSTGRES_DB:-tsaproxy}"
USER="${POSTGRES_USER:-tsaproxy}"

echo "[$(date -Iseconds)] Iniciando limpieza de datos > 1 año..."

docker exec -i "$CONTAINER" psql -U "$USER" -d "$DB" << 'SQL'
DO $$
DECLARE
  cutoff TIMESTAMPTZ := NOW() - INTERVAL '1 year';
  rec    RECORD;
  rows   BIGINT;
  suffix TEXT;
  tbl    TEXT;
BEGIN
  RAISE NOTICE 'Fecha de corte: %', cutoff;

  -- 1. Eliminar particiones mensales expiradas de usage_events y proxy_requests
  FOR rec IN
    SELECT schemaname, tablename
    FROM pg_tables
    WHERE (tablename LIKE 'usage_events_%' OR tablename LIKE 'proxy_requests_%')
      AND tablename < 'usage_events_' || TO_CHAR(cutoff, 'YYYY_MM')
      AND tablename < 'proxy_requests_' || TO_CHAR(cutoff, 'YYYY_MM')
    ORDER BY tablename
  LOOP
    EXECUTE 'DROP TABLE IF EXISTS ' || rec.schemaname || '.' || rec.tablename;
    RAISE NOTICE 'Eliminada partición: %', rec.tablename;
  END LOOP;

  -- 2. Limpiar failed_requests (tabla no particionada)
  DELETE FROM failed_requests WHERE occurred_at < cutoff;
  GET DIAGNOSTICS rows = ROW_COUNT;
  RAISE NOTICE 'failed_requests eliminados: %', rows;

  -- 3. Limpiar audit_events > 1 año
  DELETE FROM audit_events WHERE occurred_at < cutoff;
  GET DIAGNOSTICS rows = ROW_COUNT;
  RAISE NOTICE 'audit_events eliminados: %', rows;

  -- 4. Limpiar monthly_usage_aggregates muy antiguos (> 2 años para mantener histórico largo)
  DELETE FROM monthly_usage_aggregates
  WHERE year < EXTRACT(YEAR FROM NOW() - INTERVAL '2 years')::INT;
  GET DIAGNOSTICS rows = ROW_COUNT;
  RAISE NOTICE 'monthly_usage_aggregates eliminados: %', rows;

END $$;
SQL

echo "[$(date -Iseconds)] Limpieza completada."
