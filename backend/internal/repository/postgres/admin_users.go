package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

// ErrUsernameTaken / ErrEmailTaken distinguen qué campo violó la restricción
// UNIQUE al crear un usuario, para poder devolver un mensaje de validación
// específico en vez de un "conflict" genérico.
var (
	ErrUsernameTaken = errors.New("username already exists")
	ErrEmailTaken    = errors.New("email already exists")
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
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "admin_users_username_key":
				return nil, ErrUsernameTaken
			case "admin_users_email_key":
				return nil, ErrEmailTaken
			}
		}
		return nil, err
	}
	return created, nil
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
		SELECT id, name, description FROM roles WHERE name=$1
	`, name).Scan(&role.ID, &role.Name, &role.Description)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return role, err
}

// ListRoles devuelve todos los roles disponibles (para el selector del formulario).
func (r *AdminUserRepository) ListRoles(ctx context.Context) ([]*model.Role, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description FROM roles ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]*model.Role, 0)
	for rows.Next() {
		role := &model.Role{}
		if err := rows.Scan(&role.ID, &role.Name, &role.Description); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// AdminUserFilter agrupa los filtros opcionales para List.
type AdminUserFilter struct {
	Search string
	Page   int
	Limit  int
}

// List devuelve usuarios de la plataforma con paginación y búsqueda.
func (r *AdminUserRepository) List(ctx context.Context, f AdminUserFilter) ([]*model.AdminUser, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 200 {
		f.Limit = 20
	}
	offset := (f.Page - 1) * f.Limit

	args := []interface{}{}
	where := "WHERE deleted_at IS NULL"
	argIdx := 1

	if f.Search != "" {
		where += fmt.Sprintf(" AND (username ILIKE $%d OR email ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM admin_users %s`, where)
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin users: %w", err)
	}

	args = append(args, f.Limit, offset)
	dataSQL := fmt.Sprintf(`
		SELECT id, username, email, password_hash, is_active, totp_secret, totp_enabled,
		       last_login_at, last_login_ip::text, created_at, updated_at
		FROM admin_users
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list admin users: %w", err)
	}
	defer rows.Close()

	users := make([]*model.AdminUser, 0)
	for rows.Next() {
		u := &model.AdminUser{}
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash,
			&u.IsActive, &u.TOTPSecret, &u.TOTPEnabled,
			&u.LastLoginAt, &u.LastLoginIP,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	for _, u := range users {
		u.Roles, _ = r.GetRoles(ctx, u.ID)
	}

	return users, total, nil
}

// Update actualiza email y estado activo de un usuario.
func (r *AdminUserRepository) Update(ctx context.Context, id uuid.UUID, email string, isActive bool) (*model.AdminUser, error) {
	u := &model.AdminUser{}
	err := r.pool.QueryRow(ctx, `
		UPDATE admin_users SET email=$1, is_active=$2, updated_at=NOW()
		WHERE id=$3 AND deleted_at IS NULL
		RETURNING id, username, email, is_active, totp_enabled, created_at, updated_at
	`, email, isActive, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.IsActive, &u.TOTPEnabled, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "admin_users_email_key" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("update admin user: %w", err)
	}
	u.Roles, _ = r.GetRoles(ctx, u.ID)
	return u, nil
}

// SetPassword reemplaza el hash de contraseña de un usuario.
func (r *AdminUserRepository) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET password_hash=$1, updated_at=NOW() WHERE id=$2
	`, hash, id)
	return err
}

// SoftDelete marca un usuario como eliminado sin borrar el registro.
func (r *AdminUserRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1
	`, id)
	return err
}

// SetRole reemplaza el rol de un usuario por uno solo (borra los previos).
func (r *AdminUserRepository) SetRole(ctx context.Context, userID, roleID, grantedBy uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM admin_user_roles WHERE admin_user_id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_user_roles (admin_user_id, role_id, granted_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (admin_user_id, role_id) DO NOTHING
	`, userID, roleID, grantedBy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetTenantScope devuelve los tenants a los que un usuario viewer está restringido.
// Una lista vacía significa acceso irrestricto (todos los tenants).
func (r *AdminUserRepository) GetTenantScope(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tenant_id FROM admin_user_tenant_scope WHERE admin_user_id=$1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scope := make([]uuid.UUID, 0)
	for rows.Next() {
		var tid uuid.UUID
		if err := rows.Scan(&tid); err != nil {
			return nil, err
		}
		scope = append(scope, tid)
	}
	return scope, rows.Err()
}

// SetTenantScope reemplaza el scope de tenants de un usuario viewer.
// Una lista vacía deja al usuario sin restricción (todos los tenants).
func (r *AdminUserRepository) SetTenantScope(ctx context.Context, userID uuid.UUID, tenantIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM admin_user_tenant_scope WHERE admin_user_id=$1`, userID); err != nil {
		return err
	}
	for _, tid := range tenantIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_user_tenant_scope (admin_user_id, tenant_id)
			VALUES ($1, $2)
			ON CONFLICT (admin_user_id, tenant_id) DO NOTHING
		`, userID, tid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CountByRole cuenta usuarios activos y no eliminados con un rol dado.
// Se usa para evitar dejar la plataforma sin ningún superadmin.
func (r *AdminUserRepository) CountByRole(ctx context.Context, roleName string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT aur.admin_user_id)
		FROM admin_user_roles aur
		JOIN roles r ON r.id = aur.role_id
		JOIN admin_users au ON au.id = aur.admin_user_id
		WHERE r.name = $1 AND au.deleted_at IS NULL AND au.is_active = TRUE
	`, roleName).Scan(&count)
	return count, err
}
