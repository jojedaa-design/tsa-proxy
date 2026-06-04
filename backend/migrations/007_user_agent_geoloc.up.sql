-- =============================================================
-- Migration 007 — Agregar User-Agent y Geolocalización
-- =============================================================

-- Agregar columnas a usage_events para capturar User-Agent y geolocalización
ALTER TABLE usage_events
    ADD COLUMN IF NOT EXISTS user_agent TEXT,
    ADD COLUMN IF NOT EXISTS geo_country VARCHAR(2),
    ADD COLUMN IF NOT EXISTS geo_city VARCHAR(255),
    ADD COLUMN IF NOT EXISTS geo_asn VARCHAR(20);

-- Índices para reportes por país y User-Agent
CREATE INDEX IF NOT EXISTS idx_ue_geo_country ON usage_events (geo_country, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_ue_user_agent ON usage_events (user_agent, occurred_at DESC);

-- Vista para estadísticas rápidas por país
CREATE OR REPLACE VIEW v_usage_by_country AS
  SELECT
    geo_country,
    COUNT(*) as total_requests,
    COUNT(*) FILTER (WHERE status = 'success') as successful,
    COUNT(*) FILTER (WHERE status IN ('error','rejected')) as failed,
    COUNT(DISTINCT tenant_id) as unique_tenants,
    MAX(occurred_at) as last_request,
    AVG(CAST(latency_ms AS FLOAT)) as avg_latency_ms
  FROM usage_events
  WHERE geo_country IS NOT NULL AND status IS NOT NULL
  GROUP BY geo_country
  ORDER BY total_requests DESC;

-- Vista para estadísticas por User-Agent
CREATE OR REPLACE VIEW v_usage_by_user_agent AS
  SELECT
    user_agent,
    COUNT(*) as total_requests,
    COUNT(*) FILTER (WHERE status = 'success') as successful,
    COUNT(*) FILTER (WHERE status IN ('error','rejected')) as failed,
    COUNT(DISTINCT tenant_id) as unique_tenants,
    COUNT(DISTINCT source_ip) as unique_ips,
    MAX(occurred_at) as last_request,
    AVG(CAST(latency_ms AS FLOAT)) as avg_latency_ms
  FROM usage_events
  WHERE user_agent IS NOT NULL AND status IS NOT NULL
  GROUP BY user_agent
  ORDER BY total_requests DESC;
