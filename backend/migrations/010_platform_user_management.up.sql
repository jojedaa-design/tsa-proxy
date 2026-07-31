-- =============================================================================
-- 010_platform_user_management.up.sql
-- Gestión de usuarios de la plataforma: scoping de tenants para viewers,
-- vínculo de auditoría a tenant, y activación segura del sistema de roles.
--
--   - admin_user_tenant_scope: a qué tenants puede ver un usuario "viewer".
--     Sin filas para un usuario = acceso irrestricto (todos los tenants).
--     Con filas = restringido exactamente a esos tenants.
--   - audit_events.tenant_id: permite acotar auditoría por tenant para viewers.
--   - Grant de superadmin a los admin_users existentes: admin_user_roles está
--     vacía hoy, así que antes de empezar a exigir RequireRole en las rutas
--     hay que asegurar que ningún admin actual quede bloqueado fuera.
-- =============================================================================

CREATE TABLE admin_user_tenant_scope (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id UUID        NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    tenant_id     UUID        NOT NULL REFERENCES tenants(id)     ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (admin_user_id, tenant_id)
);

CREATE INDEX idx_aut_scope_user   ON admin_user_tenant_scope (admin_user_id);
CREATE INDEX idx_aut_scope_tenant ON admin_user_tenant_scope (tenant_id);

ALTER TABLE audit_events ADD COLUMN tenant_id UUID REFERENCES tenants(id);
CREATE INDEX idx_ae_tenant ON audit_events (tenant_id, occurred_at DESC);

INSERT INTO admin_user_roles (admin_user_id, role_id)
SELECT id, (SELECT id FROM roles WHERE name = 'superadmin')
FROM admin_users
ON CONFLICT DO NOTHING;
