-- Change FK delete rules from NO ACTION to CASCADE for tenant deletion
-- This allows physical deletion of tenants with all their related data

-- audit_events
ALTER TABLE audit_events DROP CONSTRAINT audit_events_tenant_id_fkey;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_tenant_id_fkey
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

-- failed_requests
ALTER TABLE failed_requests DROP CONSTRAINT failed_requests_tenant_id_fkey;
ALTER TABLE failed_requests ADD CONSTRAINT failed_requests_tenant_id_fkey
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

-- monthly_usage_aggregates
ALTER TABLE monthly_usage_aggregates DROP CONSTRAINT monthly_usage_aggregates_tenant_id_fkey;
ALTER TABLE monthly_usage_aggregates ADD CONSTRAINT monthly_usage_aggregates_tenant_id_fkey
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

-- proxy_requests (all partitions)
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN (
        SELECT tablename FROM pg_tables
        WHERE tablename LIKE 'proxy_requests%' AND schemaname = 'public'
        ORDER BY tablename
    ) LOOP
        EXECUTE format('ALTER TABLE %I DROP CONSTRAINT IF EXISTS %I CASCADE',
            r.tablename, r.tablename || '_tenant_id_fkey');
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE',
            r.tablename, r.tablename || '_tenant_id_fkey');
    END LOOP;
END $$;

-- usage_events (all partitions)
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN (
        SELECT tablename FROM pg_tables
        WHERE tablename LIKE 'usage_events%' AND schemaname = 'public'
        ORDER BY tablename
    ) LOOP
        EXECUTE format('ALTER TABLE %I DROP CONSTRAINT IF EXISTS %I CASCADE',
            r.tablename, r.tablename || '_tenant_id_fkey');
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE',
            r.tablename, r.tablename || '_tenant_id_fkey');
    END LOOP;
END $$;
