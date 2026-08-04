-- Rollback
CREATE INDEX IF NOT EXISTS idx_ue_geo_country ON usage_events (geo_country, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_ue_user_agent ON usage_events (user_agent, occurred_at DESC);
