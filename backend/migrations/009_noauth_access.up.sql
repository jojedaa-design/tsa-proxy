-- =============================================================================
-- 009_noauth_access.up.sql
-- Acceso sin contraseña (por IP) para clientes como DAVISIGN
--
-- Un tenant puede tener habilitado el acceso "no-auth":
--   - La autenticación se realiza por IP (ip_allowlist del tenant)
--   - Por defecto está BLOQUEADO — se desbloquea agregando IPs al allowlist
--   - Soporta cuotas y rate-limiting igual que otros tipos de acceso
-- =============================================================================

CREATE TABLE noauth_access (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT,
    status      VARCHAR(20)  NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'suspended')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id)
);

CREATE INDEX idx_noauth_access_tenant ON noauth_access(tenant_id);
CREATE INDEX idx_noauth_access_status ON noauth_access(status);

SELECT trigger_updated_at('noauth_access');
