package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type BasicAuthRepository struct {
	pool *pgxpool.Pool
}

func NewBasicAuthRepository(pool *pgxpool.Pool) *BasicAuthRepository {
	return &BasicAuthRepository{pool: pool}
}

func (r *BasicAuthRepository) Create(ctx context.Context, cred *model.BasicAuthCredential) (*model.BasicAuthCredential, error) {
	const q = `
		INSERT INTO basic_auth_credentials
			(tenant_id, username, key_hash, key_prefix, name, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
		RETURNING id, tenant_id, username, key_hash, key_prefix, name, status, created_at, updated_at`
	row := r.pool.QueryRow(ctx, q,
		cred.TenantID, cred.Username, cred.KeyHash, cred.KeyPrefix, cred.Name)
	return scanBasicAuth(row)
}

func (r *BasicAuthRepository) FindByUsername(ctx context.Context, username string) (*model.BasicAuthCredential, *model.Tenant, error) {
	const q = `
		SELECT b.id, b.tenant_id, b.username, b.key_hash, b.key_prefix, b.name,
		       b.status, b.created_at, b.updated_at, b.revoked_at, b.revoked_by,
		       t.id, t.name, t.slug, t.status, t.contact_email, t.created_at, t.updated_at
		FROM basic_auth_credentials b
		JOIN tenants t ON t.id = b.tenant_id
		WHERE b.username = $1`
	row := r.pool.QueryRow(ctx, q, username)

	var b model.BasicAuthCredential
	var t model.Tenant
	var contactEmail *string
	err := row.Scan(
		&b.ID, &b.TenantID, &b.Username, &b.KeyHash, &b.KeyPrefix, &b.Name,
		&b.Status, &b.CreatedAt, &b.UpdatedAt, &b.RevokedAt, &b.RevokedBy,
		&t.ID, &t.Name, &t.Slug, &t.Status, &contactEmail, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("FindByUsername: %w", err)
	}
	t.ContactEmail = contactEmail
	return &b, &t, nil
}

func (r *BasicAuthRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*model.BasicAuthCredential, error) {
	const q = `
		SELECT id, tenant_id, username, key_hash, key_prefix, name,
		       status, created_at, updated_at, revoked_at, revoked_by
		FROM basic_auth_credentials
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*model.BasicAuthCredential
	for rows.Next() {
		var b model.BasicAuthCredential
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.Username, &b.KeyHash, &b.KeyPrefix, &b.Name,
			&b.Status, &b.CreatedAt, &b.UpdatedAt, &b.RevokedAt, &b.RevokedBy,
		); err != nil {
			return nil, err
		}
		results = append(results, &b)
	}
	return results, nil
}

func (r *BasicAuthRepository) Revoke(ctx context.Context, id uuid.UUID, revokedBy uuid.UUID) error {
	const q = `
		UPDATE basic_auth_credentials
		SET status = 'revoked', revoked_at = NOW(), revoked_by = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'active'`
	tag, err := r.pool.Exec(ctx, q, id, revokedBy)
	if err != nil {
		return fmt.Errorf("Revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("credential not found or already revoked")
	}
	return nil
}

func (r *BasicAuthRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.BasicAuthCredential, error) {
	const q = `
		SELECT id, tenant_id, username, key_hash, key_prefix, name,
		       status, created_at, updated_at, revoked_at, revoked_by
		FROM basic_auth_credentials WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id)
	b, err := scanBasicAuthFull(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func (r *BasicAuthRepository) HashPassword(password string) []byte {
	h := sha256.Sum256([]byte(password))
	return h[:]
}

func scanBasicAuth(row pgx.Row) (*model.BasicAuthCredential, error) {
	var b model.BasicAuthCredential
	err := row.Scan(
		&b.ID, &b.TenantID, &b.Username, &b.KeyHash, &b.KeyPrefix, &b.Name,
		&b.Status, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func scanBasicAuthFull(row pgx.Row) (*model.BasicAuthCredential, error) {
	var b model.BasicAuthCredential
	err := row.Scan(
		&b.ID, &b.TenantID, &b.Username, &b.KeyHash, &b.KeyPrefix, &b.Name,
		&b.Status, &b.CreatedAt, &b.UpdatedAt, &b.RevokedAt, &b.RevokedBy,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
