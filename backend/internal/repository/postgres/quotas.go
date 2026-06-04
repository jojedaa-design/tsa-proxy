package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type QuotaRepository struct {
	pool *pgxpool.Pool
}

func NewQuotaRepository(pool *pgxpool.Pool) *QuotaRepository {
	return &QuotaRepository{pool: pool}
}

func (r *QuotaRepository) GetByTenant(ctx context.Context, tenantID uuid.UUID) (*model.TenantQuota, error) {
	q := &model.TenantQuota{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, plan_id, monthly_limit, burst_per_minute,
		       hard_limit, auto_suspend, reset_day, created_at, updated_at
		FROM tenant_quotas WHERE tenant_id=$1
	`, tenantID).Scan(
		&q.ID, &q.TenantID, &q.PlanID, &q.MonthlyLimit, &q.BurstPerMinute,
		&q.HardLimit, &q.AutoSuspend, &q.ResetDay, &q.CreatedAt, &q.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return q, err
}

func (r *QuotaRepository) Upsert(ctx context.Context, q *model.TenantQuota) (*model.TenantQuota, error) {
	upserted := &model.TenantQuota{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tenant_quotas
		  (tenant_id, plan_id, monthly_limit, burst_per_minute, hard_limit, auto_suspend, reset_day)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (tenant_id) DO UPDATE SET
		  plan_id=$2, monthly_limit=$3, burst_per_minute=$4,
		  hard_limit=$5, auto_suspend=$6, reset_day=$7
		RETURNING id, tenant_id, plan_id, monthly_limit, burst_per_minute,
		          hard_limit, auto_suspend, reset_day, created_at, updated_at
	`, q.TenantID, q.PlanID, q.MonthlyLimit, q.BurstPerMinute,
		q.HardLimit, q.AutoSuspend, q.ResetDay).Scan(
		&upserted.ID, &upserted.TenantID, &upserted.PlanID,
		&upserted.MonthlyLimit, &upserted.BurstPerMinute,
		&upserted.HardLimit, &upserted.AutoSuspend, &upserted.ResetDay,
		&upserted.CreatedAt, &upserted.UpdatedAt,
	)
	return upserted, err
}

func (r *QuotaRepository) GetMonthlyUsage(ctx context.Context, tenantID uuid.UUID, year, month int) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(total_requests, 0)
		FROM monthly_usage_aggregates
		WHERE tenant_id=$1 AND year=$2 AND month=$3
	`, tenantID, year, month).Scan(&total)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return total, err
}

func (r *QuotaRepository) UpsertMonthlyAggregate(ctx context.Context, tenantID uuid.UUID, year, month int, status model.UsageStatus, latencyMs int) error {
	var successInt, failInt, rejectedInt int
	switch status {
	case model.UsageStatusSuccess:
		successInt = 1
	case model.UsageStatusRejected:
		rejectedInt = 1
	default: // error
		failInt = 1
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO monthly_usage_aggregates
		  (tenant_id, year, month, total_requests, successful_requests, failed_requests, rejected_requests, total_latency_ms)
		VALUES ($1,$2,$3,1,$4,$5,$6,$7)
		ON CONFLICT (tenant_id, year, month) DO UPDATE SET
		  total_requests      = monthly_usage_aggregates.total_requests + 1,
		  successful_requests = monthly_usage_aggregates.successful_requests + $4,
		  failed_requests     = monthly_usage_aggregates.failed_requests + $5,
		  rejected_requests   = monthly_usage_aggregates.rejected_requests + $6,
		  total_latency_ms    = monthly_usage_aggregates.total_latency_ms + $7,
		  last_updated_at     = NOW()
	`, tenantID, year, month, successInt, failInt, rejectedInt, latencyMs)
	return err
}

func (r *QuotaRepository) ListPlans(ctx context.Context) ([]*model.Plan, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, monthly_limit, burst_per_minute, is_active, created_at, updated_at
		FROM plans WHERE is_active=TRUE ORDER BY monthly_limit
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := make([]*model.Plan, 0)
	for rows.Next() {
		p := &model.Plan{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.MonthlyLimit,
			&p.BurstPerMinute, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}
