-- =============================================================
-- Migration 003 — Roles y plan base
-- =============================================================

INSERT INTO roles (name, description) VALUES
  ('superadmin', 'Acceso total al sistema'),
  ('admin',      'Gestión de tenants y credenciales'),
  ('viewer',     'Solo lectura de reportes y auditoría')
ON CONFLICT (name) DO NOTHING;

INSERT INTO plans (name, description, monthly_limit, burst_per_minute) VALUES
  ('free',       'Plan gratuito',           1000,  10),
  ('starter',    'Plan inicial',           10000,  30),
  ('business',   'Plan de negocios',       50000,  60),
  ('enterprise', 'Plan empresarial',      500000, 200)
ON CONFLICT (name) DO NOTHING;

-- Upstream TSA por defecto (configurar URL y env vars reales en .env)
INSERT INTO tsa_upstreams (name, url, env_key_user, env_key_pass, timeout_ms, max_retries, is_active, is_default)
VALUES (
  'Primary TSA Upstream',
  'https://upstream-tsa-provider.example.com/tsa',
  'TSA_UPSTREAM_USERNAME',
  'TSA_UPSTREAM_PASSWORD',
  10000,
  1,
  TRUE,
  TRUE
)
ON CONFLICT DO NOTHING;
