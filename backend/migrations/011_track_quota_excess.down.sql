-- Revert: remove exceeds_quota column and index
DROP INDEX IF EXISTS idx_ue_exceeds_quota;
ALTER TABLE usage_events DROP COLUMN IF EXISTS exceeds_quota;
