package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type UsageRepository struct {
	pool *pgxpool.Pool
}

func NewUsageRepository(pool *pgxpool.Pool) *UsageRepository {
	return &UsageRepository{pool: pool}
}

// InsertEvent registra un evento de uso.
func (r *UsageRepository) InsertEvent(ctx context.Context, e *model.UsageEvent) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO usage_events
		  (id, tenant_id, credential_id, request_id, source_ip, status,
		   rejection_reason, upstream_status, latency_ms, upstream_latency_ms,
		   request_size_bytes, response_size_bytes, tsa_upstream_id,
		   user_agent, geo_country, geo_city, geo_asn, exceeds_quota, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`,
		e.ID, e.TenantID, e.CredentialID, e.RequestID, e.SourceIP, e.Status,
		e.RejectionReason, e.UpstreamStatus, e.LatencyMs, e.UpstreamLatencyMs,
		e.RequestSizeBytes, e.ResponseSizeBytes, e.TSAUpstreamID,
		e.UserAgent, e.GeoCountry, e.GeoCity, e.GeoASN, e.ExceedsQuota, e.OccurredAt,
	)
	return err
}

// UsageFilter para consultas de reportes
type UsageFilter struct {
	TenantID    *uuid.UUID
	// AllowedTenantIDs, si no es nil, restringe el resultado a este conjunto
	// de tenants (usado para usuarios "viewer" con scope limitado). nil = sin restricción.
	AllowedTenantIDs []uuid.UUID
	From             time.Time
	To               time.Time
	Granularity      string // day | month
	Page             int
	Limit            int
}

// GetAggregates devuelve los agregados de consumo filtrados.
func (r *UsageRepository) GetAggregates(ctx context.Context, f UsageFilter) ([]*model.MonthlyAggregate, error) {
	// Convertir fechas a año-mes
	fromYear, fromMonth, _ := f.From.Date()
	toYear, toMonth, _ := f.To.Date()
	fromYearMonth := fromYear*100 + int(fromMonth)
	toYearMonth := toYear*100 + int(toMonth)

	args := []interface{}{fromYearMonth, toYearMonth}
	where := "WHERE m.year*100+m.month >= $1 AND m.year*100+m.month <= $2"
	argIdx := 3

	if f.TenantID != nil {
		where += fmt.Sprintf(" AND m.tenant_id=$%d", argIdx)
		args = append(args, *f.TenantID)
		argIdx++
	}
	if f.AllowedTenantIDs != nil {
		where += fmt.Sprintf(" AND m.tenant_id = ANY($%d)", argIdx)
		args = append(args, f.AllowedTenantIDs)
		argIdx++
	}

	// Para granularidad mensual usamos monthly_usage_aggregates con información de cuota
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT m.tenant_id, t.name,
		       m.year, m.month,
		       m.total_requests, m.successful_requests,
		       m.failed_requests, m.rejected_requests,
		       m.total_latency_ms,
		       CASE WHEN m.successful_requests > 0
		            THEN m.total_latency_ms::float / m.successful_requests
		            ELSE 0 END AS avg_latency_ms,
		       q.monthly_limit,
		       (SELECT COUNT(*) FROM usage_events ue
		        WHERE ue.tenant_id = m.tenant_id
		          AND EXTRACT(YEAR FROM ue.occurred_at) = m.year
		          AND EXTRACT(MONTH FROM ue.occurred_at) = m.month
		          AND ue.status = 'success'
		          AND ue.exceeds_quota = true) AS exceeded_requests,
		       m.last_updated_at
		FROM monthly_usage_aggregates m
		JOIN tenants t ON t.id = m.tenant_id
		LEFT JOIN tenant_quotas q ON q.tenant_id = m.tenant_id
		%s
		ORDER BY m.year DESC, m.month DESC, m.total_requests DESC
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("get aggregates: %w", err)
	}
	defer rows.Close()

	result := make([]*model.MonthlyAggregate, 0)
	for rows.Next() {
		a := &model.MonthlyAggregate{}
		if err := rows.Scan(
			&a.TenantID, &a.TenantName,
			&a.Year, &a.Month,
			&a.TotalRequests, &a.SuccessfulRequests,
			&a.FailedRequests, &a.RejectedRequests,
			&a.TotalLatencyMs, &a.AvgLatencyMs,
			&a.MonthlyLimit, &a.ExceededRequests,
			&a.LastUpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// IPUsage representa el uso desde una IP
type IPUsage struct {
	IP           string `json:"ip"`
	TenantID     uuid.UUID `json:"tenant_id"`
	TenantName   string `json:"tenant_name"`
	Requests     int `json:"requests"`
	SuccessCount int `json:"success_count"`
	FailCount    int `json:"fail_count"`
	LastUsedAt   *time.Time `json:"last_used_at"`
}

// GetIPsByTenant devuelve las IPs que más han consumido en un período
func (r *UsageRepository) GetIPsByTenant(ctx context.Context, tenantID uuid.UUID, from, to time.Time) ([]*IPUsage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			host(ue.source_ip) as ip,
			ue.tenant_id,
			t.name,
			COUNT(*) as requests,
			COUNT(*) FILTER (WHERE ue.status='success') as success_count,
			COUNT(*) FILTER (WHERE ue.status IN ('error','rejected')) as fail_count,
			MAX(ue.occurred_at) as last_used_at
		FROM usage_events ue
		JOIN tenants t ON t.id=ue.tenant_id
		WHERE ue.tenant_id=$1 AND ue.occurred_at >= $2 AND ue.occurred_at <= $3
		GROUP BY host(ue.source_ip), ue.tenant_id, t.name
		ORDER BY requests DESC
		LIMIT 50
	`, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get ips by tenant: %w", err)
	}
	defer rows.Close()

	result := make([]*IPUsage, 0)
	for rows.Next() {
		ip := &IPUsage{}
		if err := rows.Scan(&ip.IP, &ip.TenantID, &ip.TenantName, &ip.Requests, &ip.SuccessCount, &ip.FailCount, &ip.LastUsedAt); err != nil {
			return nil, err
		}
		result = append(result, ip)
	}
	return result, rows.Err()
}

// GetTopIPs devuelve las IPs más activas globalmente en un período.
// allowedTenantIDs, si no es nil, restringe el resultado a esos tenants.
func (r *UsageRepository) GetTopIPs(ctx context.Context, from, to time.Time, limit int, allowedTenantIDs []uuid.UUID) ([]*IPUsage, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []interface{}{from, to}
	where := "WHERE ue.occurred_at >= $1 AND ue.occurred_at <= $2"
	if allowedTenantIDs != nil {
		where += " AND ue.tenant_id = ANY($3)"
		args = append(args, allowedTenantIDs)
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			host(ue.source_ip) as ip,
			t.id,
			t.name,
			COUNT(*) as requests,
			COUNT(*) FILTER (WHERE ue.status='success') as success_count,
			COUNT(*) FILTER (WHERE ue.status IN ('error','rejected')) as fail_count,
			MAX(ue.occurred_at) as last_used_at
		FROM usage_events ue
		JOIN tenants t ON t.id=ue.tenant_id
		%s
		GROUP BY host(ue.source_ip), t.id, t.name
		ORDER BY requests DESC
		LIMIT $%d
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("get top ips: %w", err)
	}
	defer rows.Close()

	result := make([]*IPUsage, 0)
	for rows.Next() {
		ip := &IPUsage{}
		if err := rows.Scan(&ip.IP, &ip.TenantID, &ip.TenantName, &ip.Requests, &ip.SuccessCount, &ip.FailCount, &ip.LastUsedAt); err != nil {
			return nil, err
		}
		result = append(result, ip)
	}
	return result, rows.Err()
}

// UserAgentStat representa estadísticas de un User-Agent
type UserAgentStat struct {
	UserAgent       string `json:"user_agent"`
	Requests        int    `json:"requests"`
	SuccessCount    int    `json:"success_count"`
	FailCount       int    `json:"fail_count"`
	UniqueIPs       int    `json:"unique_ips"`
	UniqueTenants   int    `json:"unique_tenants"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	LastUsedAt      *time.Time `json:"last_used_at"`
}

// GetTopUserAgents devuelve los User-Agents más comunes.
// allowedTenantIDs, si no es nil, restringe el resultado a esos tenants.
func (r *UsageRepository) GetTopUserAgents(ctx context.Context, from, to time.Time, limit int, allowedTenantIDs []uuid.UUID) ([]*UserAgentStat, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []interface{}{from, to}
	where := "WHERE occurred_at >= $1 AND occurred_at <= $2"
	if allowedTenantIDs != nil {
		where += " AND tenant_id = ANY($3)"
		args = append(args, allowedTenantIDs)
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(user_agent, 'unknown') as user_agent,
			COUNT(*) as requests,
			COUNT(*) FILTER (WHERE status='success') as success_count,
			COUNT(*) FILTER (WHERE status IN ('error','rejected')) as fail_count,
			COUNT(DISTINCT host(source_ip)) as unique_ips,
			COUNT(DISTINCT tenant_id) as unique_tenants,
			AVG(CAST(latency_ms AS FLOAT)) as avg_latency_ms,
			MAX(occurred_at) as last_used_at
		FROM usage_events
		%s
		GROUP BY COALESCE(user_agent, 'unknown')
		ORDER BY requests DESC
		LIMIT $%d
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("get user agents: %w", err)
	}
	defer rows.Close()

	result := make([]*UserAgentStat, 0)
	for rows.Next() {
		stat := &UserAgentStat{}
		if err := rows.Scan(&stat.UserAgent, &stat.Requests, &stat.SuccessCount, &stat.FailCount,
			&stat.UniqueIPs, &stat.UniqueTenants, &stat.AvgLatencyMs, &stat.LastUsedAt); err != nil {
			return nil, err
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

// CountryStat representa estadísticas de acceso por país
type CountryStat struct {
	Country       string `json:"country"`
	CountryName   string `json:"country_name"`
	Requests      int    `json:"requests"`
	SuccessCount  int    `json:"success_count"`
	FailCount     int    `json:"fail_count"`
	UniqueIPs     int    `json:"unique_ips"`
	UniqueTenants int    `json:"unique_tenants"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	LastUsedAt    *time.Time `json:"last_used_at"`
}

// GetTopCountries devuelve los países con más acceso.
// allowedTenantIDs, si no es nil, restringe el resultado a esos tenants.
func (r *UsageRepository) GetTopCountries(ctx context.Context, from, to time.Time, limit int, allowedTenantIDs []uuid.UUID) ([]*CountryStat, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []interface{}{from, to}
	where := "WHERE occurred_at >= $1 AND occurred_at <= $2"
	if allowedTenantIDs != nil {
		where += " AND tenant_id = ANY($3)"
		args = append(args, allowedTenantIDs)
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(geo_country, 'unknown') as country,
			COUNT(*) as requests,
			COUNT(*) FILTER (WHERE status='success') as success_count,
			COUNT(*) FILTER (WHERE status IN ('error','rejected')) as fail_count,
			COUNT(DISTINCT host(source_ip)) as unique_ips,
			COUNT(DISTINCT tenant_id) as unique_tenants,
			AVG(CAST(latency_ms AS FLOAT)) as avg_latency_ms,
			MAX(occurred_at) as last_used_at
		FROM usage_events
		%s
		GROUP BY COALESCE(geo_country, 'unknown')
		ORDER BY requests DESC
		LIMIT $%d
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("get countries: %w", err)
	}
	defer rows.Close()

	result := make([]*CountryStat, 0)
	for rows.Next() {
		stat := &CountryStat{}
		if err := rows.Scan(&stat.Country, &stat.Requests, &stat.SuccessCount, &stat.FailCount,
			&stat.UniqueIPs, &stat.UniqueTenants, &stat.AvgLatencyMs, &stat.LastUsedAt); err != nil {
			return nil, err
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

// UpstreamFailureStat agrega errores del proxy por motivo (de usage_events).
type UpstreamFailureStat struct {
	Reason   string    `json:"reason"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// GetUpstreamFailureStats devuelve los errores y rechazos registrados en usage_events,
// agrupados por rejection_reason (o "upstream_error" si el campo es NULL).
func (r *UsageRepository) GetUpstreamFailureStats(ctx context.Context, tenantID *uuid.UUID, allowedTenantIDs []uuid.UUID, from, to time.Time) ([]*UpstreamFailureStat, error) {
	args := []interface{}{from, to}
	where := "WHERE status IN ('error', 'rejected') AND occurred_at >= $1 AND occurred_at <= $2"
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
		SELECT
			COALESCE(rejection_reason, 'upstream_error') AS reason,
			COUNT(*) AS count,
			MAX(occurred_at) AS last_seen
		FROM usage_events
		%s
		GROUP BY COALESCE(rejection_reason, 'upstream_error')
		ORDER BY count DESC
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("get upstream failure stats: %w", err)
	}
	defer rows.Close()

	result := make([]*UpstreamFailureStat, 0)
	for rows.Next() {
		stat := &UpstreamFailureStat{}
		if err := rows.Scan(&stat.Reason, &stat.Count, &stat.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

// GetDashboardSummary retorna el resumen para el dashboard.
type DashboardSummary struct {
	ActiveTenants   int     `json:"active_tenants"`
	TodayRequests   int     `json:"today_requests"`
	MonthRequests   int     `json:"month_requests"`
	ErrorsLast24h   int     `json:"errors_last_24h"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	TopTenants      []TopTenant `json:"top_tenants"`
}

type TopTenant struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	Requests   int       `json:"requests"`
}

// GetDashboardSummary retorna el resumen para el dashboard.
// allowedTenantIDs, si no es nil, restringe todo el resumen a esos tenants.
func (r *UsageRepository) GetDashboardSummary(ctx context.Context, allowedTenantIDs []uuid.UUID) (*DashboardSummary, error) {
	s := &DashboardSummary{}
	scoped := allowedTenantIDs != nil

	// Tenants activos
	if scoped {
		_ = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM tenants WHERE status='active' AND deleted_at IS NULL AND id = ANY($1)
		`, allowedTenantIDs).Scan(&s.ActiveTenants)
	} else {
		_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tenants WHERE status='active' AND deleted_at IS NULL`).
			Scan(&s.ActiveTenants)
	}

	now := time.Now()
	y, m, _ := now.Date()

	// Requests hoy
	today := time.Date(y, m, now.Day(), 0, 0, 0, 0, now.Location())
	if scoped {
		_ = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM usage_events
			WHERE occurred_at >= $1 AND status='success' AND tenant_id = ANY($2)
		`, today, allowedTenantIDs).Scan(&s.TodayRequests)
	} else {
		_ = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM usage_events
			WHERE occurred_at >= $1 AND status='success'
		`, today).Scan(&s.TodayRequests)
	}

	// Requests este mes (de los agregados, más rápido)
	if scoped {
		_ = r.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(total_requests),0) FROM monthly_usage_aggregates
			WHERE year=$1 AND month=$2 AND tenant_id = ANY($3)
		`, y, int(m), allowedTenantIDs).Scan(&s.MonthRequests)
	} else {
		_ = r.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(total_requests),0) FROM monthly_usage_aggregates
			WHERE year=$1 AND month=$2
		`, y, int(m)).Scan(&s.MonthRequests)
	}

	// Errores últimas 24h
	since24h := now.Add(-24 * time.Hour)
	if scoped {
		_ = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM usage_events
			WHERE occurred_at >= $1 AND status IN ('error','rejected') AND tenant_id = ANY($2)
		`, since24h, allowedTenantIDs).Scan(&s.ErrorsLast24h)
	} else {
		_ = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM usage_events
			WHERE occurred_at >= $1 AND status IN ('error','rejected')
		`, since24h).Scan(&s.ErrorsLast24h)
	}

	// Latencia promedio (últimas 24h)
	if scoped {
		_ = r.pool.QueryRow(ctx, `
			SELECT COALESCE(AVG(latency_ms), 0) FROM usage_events
			WHERE occurred_at >= $1 AND status='success' AND latency_ms IS NOT NULL AND tenant_id = ANY($2)
		`, since24h, allowedTenantIDs).Scan(&s.AvgLatencyMs)
	} else {
		_ = r.pool.QueryRow(ctx, `
			SELECT COALESCE(AVG(latency_ms), 0) FROM usage_events
			WHERE occurred_at >= $1 AND status='success' AND latency_ms IS NOT NULL
		`, since24h).Scan(&s.AvgLatencyMs)
	}

	// Top 5 tenants este mes
	var rows pgx.Rows
	var err error
	if scoped {
		rows, err = r.pool.Query(ctx, `
			SELECT m.tenant_id, t.name, m.total_requests
			FROM monthly_usage_aggregates m
			JOIN tenants t ON t.id=m.tenant_id
			WHERE m.year=$1 AND m.month=$2 AND m.tenant_id = ANY($3)
			ORDER BY m.total_requests DESC LIMIT 5
		`, y, int(m), allowedTenantIDs)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT m.tenant_id, t.name, m.total_requests
			FROM monthly_usage_aggregates m
			JOIN tenants t ON t.id=m.tenant_id
			WHERE m.year=$1 AND m.month=$2
			ORDER BY m.total_requests DESC LIMIT 5
		`, y, int(m))
	}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			tt := TopTenant{}
			_ = rows.Scan(&tt.TenantID, &tt.TenantName, &tt.Requests)
			s.TopTenants = append(s.TopTenants, tt)
		}
	}

	return s, nil
}
