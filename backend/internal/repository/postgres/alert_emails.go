package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AlertEmail struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Email     string    `json:"email"`
	Label     *string   `json:"label,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type AlertEmailRepository struct {
	pool *pgxpool.Pool
}

func NewAlertEmailRepository(pool *pgxpool.Pool) *AlertEmailRepository {
	return &AlertEmailRepository{pool: pool}
}

func (r *AlertEmailRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*AlertEmail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, email, label, created_at
		FROM tenant_alert_emails WHERE tenant_id=$1 ORDER BY created_at ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*AlertEmail, 0)
	for rows.Next() {
		ae := &AlertEmail{}
		if err := rows.Scan(&ae.ID, &ae.TenantID, &ae.Email, &ae.Label, &ae.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ae)
	}
	return result, rows.Err()
}

func (r *AlertEmailRepository) ListEmails(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT email FROM tenant_alert_emails WHERE tenant_id=$1 ORDER BY created_at ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := make([]string, 0)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func (r *AlertEmailRepository) Add(ctx context.Context, tenantID uuid.UUID, email, label string) (*AlertEmail, error) {
	ae := &AlertEmail{}
	var lbl *string
	if label != "" {
		lbl = &label
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tenant_alert_emails (tenant_id, email, label)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, email) DO UPDATE SET label = EXCLUDED.label
		RETURNING id, tenant_id, email, label, created_at
	`, tenantID, email, lbl).Scan(&ae.ID, &ae.TenantID, &ae.Email, &ae.Label, &ae.CreatedAt)
	return ae, err
}

func (r *AlertEmailRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tenant_alert_emails WHERE id=$1`, id)
	return err
}
