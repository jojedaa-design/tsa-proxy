package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type FailedRequestRepository struct {
	pool *pgxpool.Pool
}

func NewFailedRequestRepository(pool *pgxpool.Pool) *FailedRequestRepository {
	return &FailedRequestRepository{pool: pool}
}

func (r *FailedRequestRepository) Insert(ctx context.Context, f *model.FailedRequest) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO failed_requests
		  (tenant_id, credential_key_id, source_ip, failure_reason, request_id)
		VALUES ($1,$2,$3,$4,$5)
	`, f.TenantID, f.CredentialKeyID, f.SourceIP, f.FailureReason, f.RequestID)
	return err
}

func (r *FailedRequestRepository) InsertInterface(ctx context.Context, f *model.FailedRequest) error {
	return r.Insert(ctx, f)
}

// FailureBreakdownRow agrega la cantidad de rechazos previos al proxy por motivo.
type FailureBreakdownRow struct {
	Reason   string    `json:"reason"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// GetBreakdown devuelve el desglose de rechazos pre-proxy agrupados por failure_reason.
// allowedTenantIDs, si no es nil, restringe el resultado a esos tenants.
func (r *FailedRequestRepository) GetBreakdown(ctx context.Context, tenantID *uuid.UUID, allowedTenantIDs []uuid.UUID, from, to time.Time) ([]*FailureBreakdownRow, error) {
	args := []interface{}{from, to}
	where := "WHERE occurred_at >= $1 AND occurred_at <= $2"
	argIdx := 3
	if tenantID != nil {
		where += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, *tenantID)
		argIdx++
	}
	if allowedTenantIDs != nil {
		where += fmt.Sprintf(" AND tenant_id = ANY($%d)", argIdx)
		args = append(args, allowedTenantIDs)
		argIdx++
	}
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT failure_reason, COUNT(*) AS count, MAX(occurred_at) AS last_seen
		FROM failed_requests
		%s
		GROUP BY failure_reason
		ORDER BY count DESC
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("get failure breakdown: %w", err)
	}
	defer rows.Close()

	result := make([]*FailureBreakdownRow, 0)
	for rows.Next() {
		row := &FailureBreakdownRow{}
		if err := rows.Scan(&row.Reason, &row.Count, &row.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
