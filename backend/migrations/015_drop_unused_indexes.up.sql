-- Migración 015: elimina índices de usage_events que el planner nunca usa.
--
-- Verificado con EXPLAIN sobre las consultas reales de TopUserAgents y
-- TopCountries: ambas filtran por occurred_at (usan idx_ue_occurred, por
-- partición) y agrupan con HashAggregate — nunca tocan idx_ue_geo_country
-- ni idx_ue_user_agent. Son índices B-tree completos (no parciales) que se
-- mantienen en cada INSERT sin aportar nada a las consultas actuales.
--
-- idx_ue_exceeds_quota se deja intacto a propósito: es un índice parcial
-- (WHERE exceeds_quota = true) y su costo de escritura ya es marginal.
DROP INDEX IF EXISTS idx_ue_geo_country;
DROP INDEX IF EXISTS idx_ue_user_agent;
