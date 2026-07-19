package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type TenantRepository struct {
	pool *pgxpool.Pool
}

func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{pool: pool}
}

// List devuelve tenants con filtros opcionales.
type TenantFilter struct {
	Status string
	Search string
	Page   int
	Limit  int
}

func (r *TenantRepository) List(ctx context.Context, f TenantFilter) ([]*model.Tenant, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 200 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	args := []interface{}{}
	where := "WHERE t.deleted_at IS NULL"
	argIdx := 1

	if f.Status != "" {
		where += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (t.name ILIKE $%d OR t.slug ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	// Count query
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM tenants t %s`, where)
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tenants: %w", err)
	}

	// Data query
	args = append(args, f.Limit, offset)
	dataSQL := fmt.Sprintf(`
		SELECT t.id, t.name, t.slug, t.description, t.status, t.contact_email,
		       t.created_by, t.created_at, t.updated_at
		FROM tenants t
		%s
		ORDER BY t.created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	tenants := make([]*model.Tenant, 0)
	for rows.Next() {
		t := &model.Tenant{}
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug, &t.Description, &t.Status,
			&t.ContactEmail, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, t)
	}

	return tenants, total, rows.Err()
}

func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Tenant, error) {
	t := &model.Tenant{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, slug, description, status, contact_email,
		       created_by, created_at, updated_at
		FROM tenants
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Description, &t.Status,
		&t.ContactEmail, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*model.Tenant, error) {
	t := &model.Tenant{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, slug, description, status, contact_email,
		       created_by, created_at, updated_at
		FROM tenants
		WHERE slug = $1 AND deleted_at IS NULL
	`, slug).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Description, &t.Status,
		&t.ContactEmail, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (r *TenantRepository) Create(ctx context.Context, t *model.Tenant) (*model.Tenant, error) {
	created := &model.Tenant{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tenants (name, slug, description, status, contact_email, created_by)
		VALUES ($1, $2, $3, 'active', $4, $5)
		RETURNING id, name, slug, description, status, contact_email, created_by, created_at, updated_at
	`, t.Name, t.Slug, t.Description, t.ContactEmail, t.CreatedBy).Scan(
		&created.ID, &created.Name, &created.Slug, &created.Description,
		&created.Status, &created.ContactEmail, &created.CreatedBy,
		&created.CreatedAt, &created.UpdatedAt,
	)
	return created, err
}

func (r *TenantRepository) Update(ctx context.Context, t *model.Tenant) (*model.Tenant, error) {
	updated := &model.Tenant{}
	err := r.pool.QueryRow(ctx, `
		UPDATE tenants
		SET name=$1, description=$2, contact_email=$3
		WHERE id=$4 AND deleted_at IS NULL
		RETURNING id, name, slug, description, status, contact_email, created_by, created_at, updated_at
	`, t.Name, t.Description, t.ContactEmail, t.ID).Scan(
		&updated.ID, &updated.Name, &updated.Slug, &updated.Description,
		&updated.Status, &updated.ContactEmail, &updated.CreatedBy,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return updated, err
}

func (r *TenantRepository) SetStatus(ctx context.Context, id uuid.UUID, status model.TenantStatus) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE tenants SET status=$1 WHERE id=$2 AND deleted_at IS NULL
	`, status, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *TenantRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tenants SET status='deleted', deleted_at=NOW() WHERE id=$1
	`, id)
	return err
}

func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, id)
	return err
}
