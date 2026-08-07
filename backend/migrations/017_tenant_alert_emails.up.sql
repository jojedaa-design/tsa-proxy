-- Correos adicionales para alertas de consumo por tenant
CREATE TABLE tenant_alert_emails (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email      TEXT        NOT NULL,
  label      TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, email)
);
CREATE INDEX idx_tae_tenant ON tenant_alert_emails(tenant_id);
