-- Credenciales HTTP Basic Auth por tenant
CREATE TABLE basic_auth_credentials (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    username    VARCHAR(100) NOT NULL UNIQUE,
    key_hash    BYTEA NOT NULL,        -- SHA-256 de la password generada
    key_prefix  VARCHAR(20) NOT NULL,  -- primeros chars para display en UI
    name        TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'revoked')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ,
    revoked_by  UUID REFERENCES admin_users(id)
);

CREATE INDEX idx_bac_tenant   ON basic_auth_credentials(tenant_id);
CREATE INDEX idx_bac_username ON basic_auth_credentials(username);
CREATE INDEX idx_bac_status   ON basic_auth_credentials(status);
