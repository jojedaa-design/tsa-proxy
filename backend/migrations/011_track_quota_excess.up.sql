-- Add exceeds_quota column to usage_events to track when usage exceeds monthly limit
ALTER TABLE usage_events ADD COLUMN exceeds_quota BOOLEAN NOT NULL DEFAULT FALSE;

-- Create index for filtering exceeded quota events
CREATE INDEX idx_ue_exceeds_quota ON usage_events (tenant_id, exceeds_quota) WHERE exceeds_quota = TRUE;
