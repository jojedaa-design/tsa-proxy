package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type IPAllowlistRepository struct {
	pool *pgxpool.Pool
}

func NewIPAllowlistRepository(pool *pgxpool.Pool) *IPAllowlistRepository {
	return &IPAllowlistRepository{pool: pool}
}

func (r *IPAllowlistRepository) FindByTenant(ctx context.Context, tenantID uuid.UUID) ([]*model.IPAllowlistEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, host(cidr) || '/' || masklen(cidr) AS cidr,
		       label, is_active, created_at, created_by
		FROM tenant_ip_allowlist
		WHERE tenant_id=$1 AND is_active=TRUE
		ORDER BY created_at
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*model.IPAllowlistEntry, 0)
	for rows.Next() {
		e := &model.IPAllowlistEntry{}
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.CIDR, &e.Label, &e.IsActive, &e.CreatedAt, &e.CreatedBy,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *IPAllowlistRepository) FindAll(ctx context.Context, tenantID uuid.UUID) ([]*model.IPAllowlistEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, host(cidr) || '/' || masklen(cidr) AS cidr,
		       label, is_active, created_at, created_by
		FROM tenant_ip_allowlist
		WHERE tenant_id=$1
		ORDER BY created_at
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*model.IPAllowlistEntry, 0)
	for rows.Next() {
		e := &model.IPAllowlistEntry{}
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.CIDR, &e.Label, &e.IsActive, &e.CreatedAt, &e.CreatedBy,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *IPAllowlistRepository) Create(ctx context.Context, e *model.IPAllowlistEntry) (*model.IPAllowlistEntry, error) {
	created := &model.IPAllowlistEntry{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tenant_ip_allowlist (tenant_id, cidr, label, created_by)
		VALUES ($1, $2::cidr, $3, $4)
		RETURNING id, tenant_id, host(cidr) || '/' || masklen(cidr), label, is_active, created_at, created_by
	`, e.TenantID, e.CIDR, e.Label, e.CreatedBy).Scan(
		&created.ID, &created.TenantID, &created.CIDR, &created.Label,
		&created.IsActive, &created.CreatedAt, &created.CreatedBy,
	)
	return created, err
}

func (r *IPAllowlistRepository) Update(ctx context.Context, id uuid.UUID, label *string) (*model.IPAllowlistEntry, error) {
	e := &model.IPAllowlistEntry{}
	err := r.pool.QueryRow(ctx, `
		UPDATE tenant_ip_allowlist SET label=$1
		WHERE id=$2
		RETURNING id, tenant_id, host(cidr) || '/' || masklen(cidr), label, is_active, created_at, created_by
	`, label, id).Scan(
		&e.ID, &e.TenantID, &e.CIDR, &e.Label,
		&e.IsActive, &e.CreatedAt, &e.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return e, err
}

func (r *IPAllowlistRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tenant_ip_allowlist SET is_active=FALSE WHERE id=$1
	`, id)
	return err
}
