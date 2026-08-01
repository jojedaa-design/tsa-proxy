-- Migración 013: modelo de bolsas de prepago (reemplaza cuota mensual)
-- Crea una tabla inmutable de "bolsas contratadas" con histórico automático.
-- Cada recarga es una fila nueva; el consumo es un SUM sobre all-time.

CREATE TABLE quota_bundles (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    amount        integer NOT NULL CHECK (amount > 0),
    note          varchar(255),
    created_by    uuid REFERENCES admin_users(id),
    contracted_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_quota_bundles_tenant_fifo
    ON quota_bundles (tenant_id, contracted_at DESC);

-- tenant_quotas pasa a ser solo política, sin cifras de consumo.
ALTER TABLE tenant_quotas
    DROP COLUMN monthly_limit,
    DROP COLUMN reset_day,
    DROP COLUMN hard_limit,
    DROP COLUMN auto_suspend;

-- Conserva burst_per_minute (límite de ritmo, no relacionado con la bolsa).
