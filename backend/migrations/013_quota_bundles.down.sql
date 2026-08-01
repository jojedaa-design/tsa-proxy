-- Rollback
ALTER TABLE tenant_quotas
    ADD COLUMN monthly_limit integer NOT NULL DEFAULT 100,
    ADD COLUMN reset_day smallint NOT NULL DEFAULT 1 CHECK (reset_day >= 1 AND reset_day <= 28),
    ADD COLUMN hard_limit boolean NOT NULL DEFAULT true,
    ADD COLUMN auto_suspend boolean NOT NULL DEFAULT false;

DROP INDEX IF EXISTS idx_quota_bundles_tenant_fifo;
DROP TABLE quota_bundles;
