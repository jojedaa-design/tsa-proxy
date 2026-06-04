package postgres

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type AdminUserRepository struct {
	pool *pgxpool.Pool
}

func NewAdminUserRepository(pool *pgxpool.Pool) *AdminUserRepository {
	return &AdminUserRepository{pool: pool}
}

func (r *AdminUserRepository) FindByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	u := &model.AdminUser{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, email, password_hash, is_active, totp_secret, totp_enabled,
		       last_login_at, last_login_ip::text, created_at, updated_at
		FROM admin_users
		WHERE username = $1 AND deleted_at IS NULL
	`, username).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.IsActive, &u.TOTPSecret, &u.TOTPEnabled,
		&u.LastLoginAt, &u.LastLoginIP,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find admin by username: %w", err)
	}
	// Cargar roles
	u.Roles, _ = r.GetRoles(ctx, u.ID)
	return u, nil
}

func (r *AdminUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.AdminUser, error) {
	u := &model.AdminUser{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, email, password_hash, is_active, totp_secret, totp_enabled,
		       last_login_at, last_login_ip::text, created_at, updated_at
		FROM admin_users
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.IsActive, &u.TOTPSecret, &u.TOTPEnabled,
		&u.LastLoginAt, &u.LastLoginIP,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find admin by id: %w", err)
	}
	u.Roles, _ = r.GetRoles(ctx, u.ID)
	return u, nil
}

func (r *AdminUserRepository) GetRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.name FROM roles r
		JOIN admin_user_roles aur ON aur.role_id = r.id
		WHERE aur.admin_user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			roles = append(roles, name)
		}
	}
	return roles, rows.Err()
}

func (r *AdminUserRepository) UpdateLoginInfo(ctx context.Context, id uuid.UUID, ip string) error {
	netIP := net.ParseIP(ip)
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET last_login_at=NOW(), last_login_ip=$1 WHERE id=$2
	`, netIP, id)
	return err
}

func (r *AdminUserRepository) Create(ctx context.Context, u *model.AdminUser) (*model.AdminUser, error) {
	created := &model.AdminUser{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO admin_users (username, email, password_hash, is_active)
		VALUES ($1, $2, $3, TRUE)
		RETURNING id, username, email, is_active, created_at, updated_at
	`, u.Username, u.Email, u.PasswordHash).Scan(
		&created.ID, &created.Username, &created.Email,
		&created.IsActive, &created.CreatedAt, &created.UpdatedAt,
	)
	return created, err
}

func (r *AdminUserRepository) AssignRole(ctx context.Context, userID, roleID, grantedBy uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO admin_user_roles (admin_user_id, role_id, granted_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (admin_user_id, role_id) DO NOTHING
	`, userID, roleID, grantedBy)
	return err
}

func (r *AdminUserRepository) UpdateTOTP(ctx context.Context, userID uuid.UUID, secret *string, enabled bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET totp_secret=$1, totp_enabled=$2, updated_at=NOW() WHERE id=$3
	`, secret, enabled, userID)
	return err
}

func (r *AdminUserRepository) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
	role := &model.Role{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, created_at FROM roles WHERE name=$1
	`, name).Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return role, err
}
