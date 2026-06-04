package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type UpstreamRepository struct {
	pool *pgxpool.Pool
}

func NewUpstreamRepository(pool *pgxpool.Pool) *UpstreamRepository {
	return &UpstreamRepository{pool: pool}
}

func (r *UpstreamRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.TSAUpstream, error) {
	u := &model.TSAUpstream{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, url, env_key_user, env_key_pass, username, password,
		       timeout_ms, max_retries, is_active, is_default, created_at, updated_at
		FROM tsa_upstreams
		WHERE id=$1
	`, id).Scan(
		&u.ID, &u.Name, &u.URL, &u.EnvKeyUser, &u.EnvKeyPass, &u.Username, &u.Password,
		&u.TimeoutMs, &u.MaxRetries, &u.IsActive, &u.IsDefault,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.HasPassword = u.Password != nil && *u.Password != ""
	return u, nil
}

func (r *UpstreamRepository) GetDefault(ctx context.Context) (*model.TSAUpstream, error) {
	u := &model.TSAUpstream{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, url, env_key_user, env_key_pass, username, password,
		       timeout_ms, max_retries, is_active, is_default, created_at, updated_at
		FROM tsa_upstreams
		WHERE is_default=TRUE AND is_active=TRUE
		LIMIT 1
	`).Scan(
		&u.ID, &u.Name, &u.URL, &u.EnvKeyUser, &u.EnvKeyPass, &u.Username, &u.Password,
		&u.TimeoutMs, &u.MaxRetries, &u.IsActive, &u.IsDefault,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.HasPassword = u.Password != nil && *u.Password != ""
	return u, nil
}

func (r *UpstreamRepository) List(ctx context.Context) ([]*model.TSAUpstream, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, url, env_key_user, env_key_pass, username, password,
		       timeout_ms, max_retries, is_active, is_default, created_at, updated_at
		FROM tsa_upstreams ORDER BY is_default DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	upstreams := make([]*model.TSAUpstream, 0)
	for rows.Next() {
		u := &model.TSAUpstream{}
		if err := rows.Scan(
			&u.ID, &u.Name, &u.URL, &u.EnvKeyUser, &u.EnvKeyPass, &u.Username, &u.Password,
			&u.TimeoutMs, &u.MaxRetries, &u.IsActive, &u.IsDefault,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		u.HasPassword = u.Password != nil && *u.Password != ""
		upstreams = append(upstreams, u)
	}
	return upstreams, rows.Err()
}

func (r *UpstreamRepository) Create(ctx context.Context, u *model.TSAUpstream) (*model.TSAUpstream, error) {
	created := &model.TSAUpstream{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tsa_upstreams (name, url, env_key_user, env_key_pass, username, password, timeout_ms, max_retries)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, name, url, env_key_user, env_key_pass, username, password,
		          timeout_ms, max_retries, is_active, is_default, created_at, updated_at
	`, u.Name, u.URL, u.EnvKeyUser, u.EnvKeyPass, u.Username, u.Password, u.TimeoutMs, u.MaxRetries).Scan(
		&created.ID, &created.Name, &created.URL, &created.EnvKeyUser, &created.EnvKeyPass,
		&created.Username, &created.Password,
		&created.TimeoutMs, &created.MaxRetries, &created.IsActive, &created.IsDefault,
		&created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	created.HasPassword = created.Password != nil && *created.Password != ""
	return created, nil
}

func (r *UpstreamRepository) Update(ctx context.Context, u *model.TSAUpstream) (*model.TSAUpstream, error) {
	updated := &model.TSAUpstream{}
	err := r.pool.QueryRow(ctx, `
		UPDATE tsa_upstreams
		SET name=$1, url=$2, env_key_user=$3, env_key_pass=$4, username=$5, password=$6,
		    timeout_ms=$7, max_retries=$8, is_active=$9
		WHERE id=$10
		RETURNING id, name, url, env_key_user, env_key_pass, username, password,
		          timeout_ms, max_retries, is_active, is_default, created_at, updated_at
	`, u.Name, u.URL, u.EnvKeyUser, u.EnvKeyPass, u.Username, u.Password,
		u.TimeoutMs, u.MaxRetries, u.IsActive, u.ID).Scan(
		&updated.ID, &updated.Name, &updated.URL, &updated.EnvKeyUser, &updated.EnvKeyPass,
		&updated.Username, &updated.Password,
		&updated.TimeoutMs, &updated.MaxRetries, &updated.IsActive, &updated.IsDefault,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	updated.HasPassword = updated.Password != nil && *updated.Password != ""
	return updated, nil
}

func (r *UpstreamRepository) SetDefault(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Quitar default de todos
	if _, err := tx.Exec(ctx, `UPDATE tsa_upstreams SET is_default=FALSE WHERE is_default=TRUE`); err != nil {
		return err
	}
	// Marcar el nuevo default
	if _, err := tx.Exec(ctx, `UPDATE tsa_upstreams SET is_default=TRUE, is_active=TRUE WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
