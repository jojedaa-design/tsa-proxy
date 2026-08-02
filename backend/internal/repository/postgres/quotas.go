package postgres

import (
	"context"
	"time"

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
		SELECT id, tenant_id, plan_id, burst_per_minute, created_at, updated_at
		FROM tenant_quotas WHERE tenant_id=$1
	`, tenantID).Scan(
		&q.ID, &q.TenantID, &q.PlanID, &q.BurstPerMinute, &q.CreatedAt, &q.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return q, err
}

func (r *QuotaRepository) Upsert(ctx context.Context, q *model.TenantQuota) (*model.TenantQuota, error) {
	upserted := &model.TenantQuota{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tenant_quotas (tenant_id, plan_id, burst_per_minute)
		VALUES ($1,$2,$3)
		ON CONFLICT (tenant_id) DO UPDATE SET
		  plan_id=$2, burst_per_minute=$3
		RETURNING id, tenant_id, plan_id, burst_per_minute, created_at, updated_at
	`, q.TenantID, q.PlanID, q.BurstPerMinute).Scan(
		&upserted.ID, &upserted.TenantID, &upserted.PlanID,
		&upserted.BurstPerMinute, &upserted.CreatedAt, &upserted.UpdatedAt,
	)
	return upserted, err
}

func (r *QuotaRepository) GetTotalConsumption(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_requests), 0) FROM monthly_usage_aggregates
		WHERE tenant_id=$1
	`, tenantID).Scan(&total)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return total, err
}

func (r *QuotaRepository) GetTotalContracted(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM quota_bundles
		WHERE tenant_id=$1
	`, tenantID).Scan(&total)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return total, err
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

func (r *QuotaRepository) AddBundle(ctx context.Context, bundle *model.QuotaBundle) (*model.QuotaBundle, error) {
	created := &model.QuotaBundle{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO quota_bundles (tenant_id, amount, note, created_by)
		VALUES ($1,$2,$3,$4)
		RETURNING id, tenant_id, amount, note, created_by, contracted_at
	`, bundle.TenantID, bundle.Amount, bundle.Note, bundle.CreatedBy).Scan(
		&created.ID, &created.TenantID, &created.Amount, &created.Note,
		&created.CreatedBy, &created.ContractedAt,
	)
	return created, err
}

func (r *QuotaRepository) ListBundles(ctx context.Context, tenantID uuid.UUID) ([]*model.QuotaBundle, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, amount, note, created_by, contracted_at
		FROM quota_bundles WHERE tenant_id=$1 ORDER BY contracted_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bundles := make([]*model.QuotaBundle, 0)
	for rows.Next() {
		b := &model.QuotaBundle{}
		if err := rows.Scan(&b.ID, &b.TenantID, &b.Amount, &b.Note, &b.CreatedBy, &b.ContractedAt); err != nil {
			return nil, err
		}
		bundles = append(bundles, b)
	}
	return bundles, rows.Err()
}

type BundleWithConsumption struct {
	ID           uuid.UUID `json:"id"`
	Amount       int       `json:"amount"`
	Consumed     int       `json:"consumed"`
	Note         *string   `json:"note,omitempty"`
	ContractedAt time.Time `json:"contracted_at"`
}

func (r *QuotaRepository) GetBundlesWithConsumption(ctx context.Context, tenantID uuid.UUID) ([]*BundleWithConsumption, error) {
	totalConsumed, _ := r.GetTotalConsumption(ctx, tenantID)

	rows, err := r.pool.Query(ctx, `
		SELECT id, amount, note, contracted_at
		FROM quota_bundles WHERE tenant_id = $1
		ORDER BY contracted_at ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bundles := make([]*BundleWithConsumption, 0)
	remaining := totalConsumed

	for rows.Next() {
		b := &BundleWithConsumption{}
		if err := rows.Scan(&b.ID, &b.Amount, &b.Note, &b.ContractedAt); err != nil {
			return nil, err
		}

		// FIFO: cada bolsa consume lo que le corresponde
		if remaining >= b.Amount {
			b.Consumed = b.Amount
			remaining -= b.Amount
		} else {
			b.Consumed = remaining
			remaining = 0
		}

		bundles = append(bundles, b)
	}
	return bundles, rows.Err()
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
