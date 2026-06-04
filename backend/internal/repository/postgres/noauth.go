package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type NoAuthRepository struct {
	pool *pgxpool.Pool
}

func NewNoAuthRepository(pool *pgxpool.Pool) *NoAuthRepository {
	return &NoAuthRepository{pool: pool}
}

// FindTenantByIP busca un tenant cuyo noauth_access esté activo y
// cuya ip_allowlist contenga la IP del cliente.
// Si no existe entrada en noauth_access o no hay IPs configuradas → nil.
func (r *NoAuthRepository) FindTenantByIP(ctx context.Context, clientIP string) (*model.Tenant, *model.NoAuthAccess, error) {
	const q = `
		SELECT
			na.id, na.tenant_id, na.name, na.status, na.created_at, na.updated_at,
			t.id, t.name, t.slug, t.status, t.contact_email, t.created_at, t.updated_at
		FROM noauth_access na
		JOIN tenants t ON t.id = na.tenant_id
		JOIN tenant_ip_allowlist ia ON ia.tenant_id = na.tenant_id
		WHERE na.status = 'active'
		  AND t.status = 'active'
		  AND ia.is_active = TRUE
		  AND $1::inet <<= ia.cidr::inet
		LIMIT 1`

	row := r.pool.QueryRow(ctx, q, clientIP)

	var na model.NoAuthAccess
	var t model.Tenant
	var contactEmail *string

	err := row.Scan(
		&na.ID, &na.TenantID, &na.Name, &na.Status, &na.CreatedAt, &na.UpdatedAt,
		&t.ID, &t.Name, &t.Slug, &t.Status, &contactEmail, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("FindTenantByIP: %w", err)
	}
	t.ContactEmail = contactEmail
	return &t, &na, nil
}

// GetByTenant devuelve el noauth_access de un tenant (puede no existir).
func (r *NoAuthRepository) GetByTenant(ctx context.Context, tenantID uuid.UUID) (*model.NoAuthAccess, error) {
	const q = `
		SELECT id, tenant_id, name, status, created_at, updated_at
		FROM noauth_access
		WHERE tenant_id = $1`

	var na model.NoAuthAccess
	err := r.pool.QueryRow(ctx, q, tenantID).Scan(
		&na.ID, &na.TenantID, &na.Name, &na.Status, &na.CreatedAt, &na.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetByTenant: %w", err)
	}
	return &na, nil
}

// Create habilita acceso no-auth para un tenant.
func (r *NoAuthRepository) Create(ctx context.Context, tenantID uuid.UUID, name *string) (*model.NoAuthAccess, error) {
	const q = `
		INSERT INTO noauth_access (tenant_id, name, status)
		VALUES ($1, $2, 'active')
		RETURNING id, tenant_id, name, status, created_at, updated_at`

	var na model.NoAuthAccess
	err := r.pool.QueryRow(ctx, q, tenantID, name).Scan(
		&na.ID, &na.TenantID, &na.Name, &na.Status, &na.CreatedAt, &na.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("Create noauth_access: %w", err)
	}
	return &na, nil
}

// Suspend desactiva el acceso no-auth de un tenant.
func (r *NoAuthRepository) Suspend(ctx context.Context, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE noauth_access SET status='suspended', updated_at=NOW() WHERE tenant_id=$1`,
		tenantID)
	return err
}

// Activate reactiva el acceso no-auth de un tenant.
func (r *NoAuthRepository) Activate(ctx context.Context, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE noauth_access SET status='active', updated_at=NOW() WHERE tenant_id=$1`,
		tenantID)
	return err
}

// Delete elimina el registro de noauth_access de un tenant.
func (r *NoAuthRepository) Delete(ctx context.Context, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM noauth_access WHERE tenant_id=$1`,
		tenantID)
	return err
}
