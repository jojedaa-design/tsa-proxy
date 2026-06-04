-- =============================================================
-- Migration 001 — Schema inicial TimeStamp Proxy
-- =============================================================

-- Extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- =============================================================
-- Función trigger para updated_at automático
-- =============================================================
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================
-- ROLES
-- =============================================================
CREATE TABLE roles (
  id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  name        VARCHAR(50) NOT NULL UNIQUE,
  description TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================
-- ADMIN USERS
-- =============================================================
CREATE TABLE admin_users (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  username      VARCHAR(100) NOT NULL UNIQUE,
  email         VARCHAR(255) NOT NULL UNIQUE,
  password_hash TEXT         NOT NULL,
  is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
  last_login_at TIMESTAMPTZ,
  last_login_ip INET,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at    TIMESTAMPTZ
);

CREATE TRIGGER trg_admin_users_updated_at
  BEFORE UPDATE ON admin_users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_admin_users_email  ON admin_users (email)     WHERE deleted_at IS NULL;
CREATE INDEX idx_admin_users_active ON admin_users (is_active) WHERE deleted_at IS NULL;

-- =============================================================
-- ADMIN USER ROLES
-- =============================================================
CREATE TABLE admin_user_roles (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_user_id UUID        NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  role_id       UUID        NOT NULL REFERENCES roles(id),
  granted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  granted_by    UUID        REFERENCES admin_users(id),
  UNIQUE (admin_user_id, role_id)
);

CREATE INDEX idx_aur_user ON admin_user_roles (admin_user_id);
CREATE INDEX idx_aur_role ON admin_user_roles (role_id);

-- =============================================================
-- TENANTS
-- =============================================================
CREATE TABLE tenants (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  name          VARCHAR(255) NOT NULL,
  slug          VARCHAR(100) NOT NULL UNIQUE,
  description   TEXT,
  status        VARCHAR(20)  NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','suspended','deleted')),
  contact_email VARCHAR(255),
  metadata      JSONB        NOT NULL DEFAULT '{}',
  created_by    UUID         REFERENCES admin_users(id),
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at    TIMESTAMPTZ
);

CREATE TRIGGER trg_tenants_updated_at
  BEFORE UPDATE ON tenants
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_tenants_status ON tenants (status)     WHERE deleted_at IS NULL;
CREATE INDEX idx_tenants_slug   ON tenants (slug)       WHERE deleted_at IS NULL;
CREATE INDEX idx_tenants_name   ON tenants USING gin(name gin_trgm_ops);

-- =============================================================
-- API CREDENTIALS
-- =============================================================
CREATE TABLE api_credentials (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  key_id        VARCHAR(32)  NOT NULL UNIQUE,
  key_hash      BYTEA        NOT NULL,
  key_prefix    VARCHAR(16)  NOT NULL,
  name          VARCHAR(255),
  status        VARCHAR(20)  NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','revoked','expired')),
  expires_at    TIMESTAMPTZ,
  last_used_at  TIMESTAMPTZ,
  last_used_ip  INET,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  revoked_at    TIMESTAMPTZ,
  revoked_by    UUID         REFERENCES admin_users(id)
);

CREATE TRIGGER trg_api_credentials_updated_at
  BEFORE UPDATE ON api_credentials
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_cred_tenant   ON api_credentials (tenant_id);
CREATE INDEX idx_cred_key_hash ON api_credentials (key_hash)  WHERE status = 'active';
CREATE INDEX idx_cred_status   ON api_credentials (status);

-- =============================================================
-- TENANT IP ALLOWLIST
-- =============================================================
CREATE TABLE tenant_ip_allowlist (
  id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  cidr        CIDR        NOT NULL,
  label       VARCHAR(255),
  is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by  UUID        REFERENCES admin_users(id),
  UNIQUE (tenant_id, cidr)
);

CREATE INDEX idx_ipallow_tenant ON tenant_ip_allowlist (tenant_id) WHERE is_active = TRUE;

-- =============================================================
-- PLANS
-- =============================================================
CREATE TABLE plans (
  id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  name             VARCHAR(100) NOT NULL UNIQUE,
  description      TEXT,
  monthly_limit    INTEGER      NOT NULL,
  burst_per_minute INTEGER      NOT NULL,
  is_active        BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_plans_updated_at
  BEFORE UPDATE ON plans
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================
-- TENANT QUOTAS
-- =============================================================
CREATE TABLE tenant_quotas (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        UUID        NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
  plan_id          UUID        REFERENCES plans(id),
  monthly_limit    INTEGER     NOT NULL,
  burst_per_minute INTEGER     NOT NULL,
  hard_limit       BOOLEAN     NOT NULL DEFAULT TRUE,
  auto_suspend     BOOLEAN     NOT NULL DEFAULT FALSE,
  reset_day        SMALLINT    NOT NULL DEFAULT 1 CHECK (reset_day BETWEEN 1 AND 28),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_tenant_quotas_updated_at
  BEFORE UPDATE ON tenant_quotas
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================
-- TSA UPSTREAMS
-- =============================================================
CREATE TABLE tsa_upstreams (
  id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  name         VARCHAR(100) NOT NULL,
  url          TEXT         NOT NULL,
  env_key_user VARCHAR(100),
  env_key_pass VARCHAR(100),
  timeout_ms   INTEGER      NOT NULL DEFAULT 10000,
  max_retries  SMALLINT     NOT NULL DEFAULT 1,
  is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
  is_default   BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_tsa_upstreams_updated_at
  BEFORE UPDATE ON tsa_upstreams
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX idx_tsa_default ON tsa_upstreams (is_default)
  WHERE is_default = TRUE AND is_active = TRUE;

-- =============================================================
-- USAGE EVENTS (particionada por mes)
-- =============================================================
CREATE TABLE usage_events (
  id                  UUID        NOT NULL DEFAULT gen_random_uuid(),
  tenant_id           UUID        NOT NULL REFERENCES tenants(id),
  credential_id       UUID        REFERENCES api_credentials(id),
  request_id          VARCHAR(36) NOT NULL,
  source_ip           INET        NOT NULL,
  status              VARCHAR(20) NOT NULL
                        CHECK (status IN ('success','error','rejected')),
  rejection_reason    VARCHAR(50),
  upstream_status     SMALLINT,
  latency_ms          INTEGER,
  upstream_latency_ms INTEGER,
  request_size_bytes  INTEGER,
  response_size_bytes INTEGER,
  tsa_upstream_id     UUID        REFERENCES tsa_upstreams(id),
  occurred_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_ue_tenant_time ON usage_events (tenant_id,  occurred_at DESC);
CREATE INDEX idx_ue_occurred    ON usage_events (occurred_at DESC);
CREATE INDEX idx_ue_status      ON usage_events (status,     occurred_at DESC);

-- =============================================================
-- MONTHLY USAGE AGGREGATES
-- =============================================================
CREATE TABLE monthly_usage_aggregates (
  id                  UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id           UUID     NOT NULL REFERENCES tenants(id),
  year                SMALLINT NOT NULL,
  month               SMALLINT NOT NULL,
  total_requests      INTEGER  NOT NULL DEFAULT 0,
  successful_requests INTEGER  NOT NULL DEFAULT 0,
  failed_requests     INTEGER  NOT NULL DEFAULT 0,
  rejected_requests   INTEGER  NOT NULL DEFAULT 0,
  total_latency_ms    BIGINT   NOT NULL DEFAULT 0,
  last_updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, year, month)
);

CREATE INDEX idx_mua_tenant_period ON monthly_usage_aggregates (tenant_id, year DESC, month DESC);

-- =============================================================
-- AUDIT EVENTS
-- =============================================================
CREATE TABLE audit_events (
  id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id    UUID         REFERENCES admin_users(id),
  actor_type  VARCHAR(20)  NOT NULL DEFAULT 'admin'
                CHECK (actor_type IN ('admin','system')),
  action      VARCHAR(100) NOT NULL,
  entity_type VARCHAR(50)  NOT NULL,
  entity_id   UUID,
  changes     JSONB,
  ip_address  INET,
  user_agent  TEXT,
  occurred_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ae_actor    ON audit_events (actor_id,              occurred_at DESC);
CREATE INDEX idx_ae_entity   ON audit_events (entity_type, entity_id);
CREATE INDEX idx_ae_occurred ON audit_events (occurred_at DESC);
CREATE INDEX idx_ae_action   ON audit_events (action,               occurred_at DESC);

-- =============================================================
-- PROXY REQUESTS (detalle técnico upstream, particionada)
-- =============================================================
CREATE TABLE proxy_requests (
  id                   UUID        NOT NULL DEFAULT gen_random_uuid(),
  usage_event_id       UUID        NOT NULL,
  tenant_id            UUID        NOT NULL REFERENCES tenants(id),
  request_hash         CHAR(64),
  upstream_url         TEXT        NOT NULL,
  upstream_http_status SMALLINT,
  upstream_latency_ms  INTEGER,
  error_detail         VARCHAR(500),
  occurred_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_pr_tenant_time ON proxy_requests (tenant_id,  occurred_at DESC);
CREATE INDEX idx_pr_occurred    ON proxy_requests (occurred_at DESC);

-- =============================================================
-- FAILED REQUESTS (para security monitoring)
-- =============================================================
CREATE TABLE failed_requests (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID        REFERENCES tenants(id),
  credential_key_id VARCHAR(32),
  source_ip         INET        NOT NULL,
  failure_reason    VARCHAR(50) NOT NULL
    CHECK (failure_reason IN (
      'invalid_key','ip_not_allowed','quota_exceeded',
      'rate_limited','upstream_error','tenant_suspended',
      'credential_expired','malformed_request'
    )),
  request_id        VARCHAR(36),
  occurred_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_fr_tenant_time ON failed_requests (tenant_id,  occurred_at DESC) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_fr_ip_time     ON failed_requests (source_ip,  occurred_at DESC);
CREATE INDEX idx_fr_occurred    ON failed_requests (occurred_at DESC);
CREATE INDEX idx_fr_reason      ON failed_requests (failure_reason, occurred_at DESC);
