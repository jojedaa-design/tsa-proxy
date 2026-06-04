-- =============================================================
-- Migration 002 — Particiones iniciales (mes actual + 2 siguientes)
-- Este script se ejecuta una vez en deploy y luego el job
-- partitions.go crea nuevas particiones mensualmente.
-- =============================================================

DO $$
DECLARE
  base_month DATE;
  start_date DATE;
  end_date   DATE;
  tbl_suffix TEXT;
  i          INTEGER;
BEGIN
  -- Crear particiones para el mes actual y los 2 siguientes
  FOR i IN 0..2 LOOP
    base_month := DATE_TRUNC('month', NOW()) + (i || ' months')::INTERVAL;
    start_date := base_month;
    end_date   := base_month + INTERVAL '1 month';
    tbl_suffix := TO_CHAR(base_month, 'YYYY_MM');

    -- usage_events
    BEGIN
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS usage_events_%s
         PARTITION OF usage_events
         FOR VALUES FROM (%L) TO (%L)',
        tbl_suffix, start_date, end_date
      );
    EXCEPTION WHEN duplicate_table THEN NULL;
    END;

    -- proxy_requests
    BEGIN
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS proxy_requests_%s
         PARTITION OF proxy_requests
         FOR VALUES FROM (%L) TO (%L)',
        tbl_suffix, start_date, end_date
      );
    EXCEPTION WHEN duplicate_table THEN NULL;
    END;

  END LOOP;
END $$;
