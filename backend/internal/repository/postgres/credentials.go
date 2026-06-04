package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type CredentialRepository struct {
	pool *pgxpool.Pool
}

func NewCredentialRepository(pool *pgxpool.Pool) *CredentialRepository {
	return &CredentialRepository{pool: pool}
}

// FindByHash busca una credencial activa por su hash SHA-256.
// Devuelve la credencial y el tenant asociado.
func (r *CredentialRepository) FindByHash(ctx context.Context, hash []byte) (*model.APICredential, *model.Tenant, error) {
	cred := &model.APICredential{}
	tenant := &model.Tenant{}
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, c.tenant_id, c.key_id, c.key_hash, c.key_prefix, c.url_token, c.name,
		       c.status, c.expires_at, c.last_used_at,
		       t.id, t.status
		FROM api_credentials c
		JOIN tenants t ON t.id = c.tenant_id
		WHERE c.key_hash = $1
		  AND c.status = 'active'
		  AND t.deleted_at IS NULL
	`, hash).Scan(
		&cred.ID, &cred.TenantID, &cred.KeyID, &cred.KeyHash, &cred.KeyPrefix, &cred.URLToken, &cred.Name,
		&cred.Status, &cred.ExpiresAt, &cred.LastUsedAt,
		&tenant.ID, &tenant.Status,
	)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find credential by hash: %w", err)
	}
	return cred, tenant, nil
}

// FindByURLToken busca una credencial activa por su url_token.
// Usado para autenticación vía URL (software de firma que no envía auth headers).
func (r *CredentialRepository) FindByURLToken(ctx context.Context, token string) (*model.APICredential, *model.Tenant, error) {
	cred := &model.APICredential{}
	tenant := &model.Tenant{}
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, c.tenant_id, c.key_id, c.key_hash, c.key_prefix, c.url_token, c.name,
		       c.status, c.expires_at, c.last_used_at,
		       t.id, t.status
		FROM api_credentials c
		JOIN tenants t ON t.id = c.tenant_id
		WHERE c.url_token = $1
		  AND c.status = 'active'
		  AND t.deleted_at IS NULL
	`, token).Scan(
		&cred.ID, &cred.TenantID, &cred.KeyID, &cred.KeyHash, &cred.KeyPrefix, &cred.URLToken, &cred.Name,
		&cred.Status, &cred.ExpiresAt, &cred.LastUsedAt,
		&tenant.ID, &tenant.Status,
	)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find credential by url_token: %w", err)
	}
	return cred, tenant, nil
}

// GetTenantBurstLimit devuelve el burst_per_minute del tenant.
func (r *CredentialRepository) GetTenantBurstLimit(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var limit int
	err := r.pool.QueryRow(ctx, `
		SELECT burst_per_minute FROM tenant_quotas WHERE tenant_id = $1
	`, tenantID).Scan(&limit)
	if err == pgx.ErrNoRows {
		return 60, nil // default
	}
	return limit, err
}

// ListByTenant devuelve todas las credenciales de un tenant.
func (r *CredentialRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*model.APICredential, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, key_id, key_prefix, url_token, name, status,
		       expires_at, last_used_at, last_used_ip,
		       created_at, updated_at, revoked_at, revoked_by
		FROM api_credentials
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	creds := make([]*model.APICredential, 0)
	for rows.Next() {
		c := &model.APICredential{}
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.KeyID, &c.KeyPrefix, &c.URLToken, &c.Name, &c.Status,
			&c.ExpiresAt, &c.LastUsedAt, &c.LastUsedIP,
			&c.CreatedAt, &c.UpdatedAt, &c.RevokedAt, &c.RevokedBy,
		); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// Create inserta una nueva credencial. key_hash es el SHA-256 del API key completo.
func (r *CredentialRepository) Create(ctx context.Context, c *model.APICredential) (*model.APICredential, error) {
	created := &model.APICredential{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO api_credentials
		  (tenant_id, key_id, key_hash, key_prefix, url_token, name, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
		RETURNING id, tenant_id, key_id, key_prefix, url_token, name, status,
		          expires_at, created_at, updated_at
	`, c.TenantID, c.KeyID, c.KeyHash, c.KeyPrefix, c.URLToken, c.Name, c.ExpiresAt).Scan(
		&created.ID, &created.TenantID, &created.KeyID, &created.KeyPrefix, &created.URLToken,
		&created.Name, &created.Status, &created.ExpiresAt,
		&created.CreatedAt, &created.UpdatedAt,
	)
	return created, err
}

// Revoke marca una credencial como revocada.
func (r *CredentialRepository) Revoke(ctx context.Context, id uuid.UUID, revokedBy uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE api_credentials
		SET status='revoked', revoked_at=NOW(), revoked_by=$1
		WHERE id=$2 AND status='active'
	`, revokedBy, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetByID recupera una credencial por ID.
func (r *CredentialRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.APICredential, error) {
	c := &model.APICredential{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, key_id, key_hash, key_prefix, name, status,
		       expires_at, last_used_at, last_used_ip,
		       created_at, updated_at, revoked_at, revoked_by
		FROM api_credentials WHERE id=$1
	`, id).Scan(
		&c.ID, &c.TenantID, &c.KeyID, &c.KeyHash, &c.KeyPrefix, &c.Name, &c.Status,
		&c.ExpiresAt, &c.LastUsedAt, &c.LastUsedIP,
		&c.CreatedAt, &c.UpdatedAt, &c.RevokedAt, &c.RevokedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// UpdateLastUsed actualiza last_used_at e IP en la credencial.
func (r *CredentialRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID, ip string) {
	_, _ = r.pool.Exec(ctx, `
		UPDATE api_credentials SET last_used_at=NOW(), last_used_ip=$1 WHERE id=$2
	`, ip, id)
}
