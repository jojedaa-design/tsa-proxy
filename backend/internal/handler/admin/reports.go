package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
)

type ReportsHandler struct {
	usageRepo  *postgres.UsageRepository
	failedRepo *postgres.FailedRequestRepository
	userRepo   *postgres.AdminUserRepository
}

func NewReportsHandler(usageRepo *postgres.UsageRepository, failedRepo *postgres.FailedRequestRepository, userRepo *postgres.AdminUserRepository) *ReportsHandler {
	return &ReportsHandler{usageRepo: usageRepo, failedRepo: failedRepo, userRepo: userRepo}
}

// FailureDetail describe un motivo de fallo con su conteo y metadatos de presentación.
type FailureDetail struct {
	Reason   string    `json:"reason"`
	Label    string    `json:"label"`
	Category string    `json:"category"` // "auth" | "quota" | "rate" | "upstream" | "request"
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// failureMeta centraliza las etiquetas y categorías de cada motivo de fallo conocido.
var failureMeta = map[string]struct{ label, category string }{
	"invalid_key":        {"Clave API inválida", "auth"},
	"ip_not_allowed":     {"IP no autorizada", "auth"},
	"tenant_suspended":   {"Tenant suspendido", "auth"},
	"credential_expired": {"Credencial expirada", "auth"},
	"quota_exceeded":     {"Bolsa de Sello de Tiempo agotada", "quota"},
	"rate_limited":       {"Límite de tasa excedido", "rate"},
	"upstream_error":     {"Error del servidor TSA", "upstream"},
	"malformed_request":  {"Solicitud malformada", "request"},
}

// Usage — GET /api/admin/v1/reports/usage
func (h *ReportsHandler) Usage(w http.ResponseWriter, r *http.Request) {
	f, err := parseUsageFilter(r)
	if err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"filter": err.Error()})
		return
	}
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, f.TenantID)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}
	f.AllowedTenantIDs = allowed

	aggregates, err := h.usageRepo.GetAggregates(r.Context(), *f)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Calcular summary
	var totalReqs, totalSuccess int
	var totalLatency int64
	for _, a := range aggregates {
		totalReqs += a.TotalRequests
		totalSuccess += a.SuccessfulRequests
		totalLatency += a.TotalLatencyMs
	}

	var avgLatency float64
	if totalSuccess > 0 {
		avgLatency = float64(totalLatency) / float64(totalSuccess)
	}
	var successRate float64
	if totalReqs > 0 {
		successRate = float64(totalSuccess) / float64(totalReqs) * 100
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": aggregates,
		"summary": map[string]interface{}{
			"total_requests":  totalReqs,
			"success_rate":    fmt.Sprintf("%.2f", successRate),
			"avg_latency_ms":  fmt.Sprintf("%.1f", avgLatency),
		},
	})
}

// NextReportNumber — POST /api/admin/v1/reports/next-number
// Devuelve un número correlativo único para numerar un reporte de consumo
// exportado (PDF ejecutivo). Cada llamada consume el siguiente valor de la
// secuencia, por lo que solo debe invocarse una vez por documento generado.
func (h *ReportsHandler) NextReportNumber(w http.ResponseWriter, r *http.Request) {
	n, err := h.usageRepo.NextReportNumber(r.Context())
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"number": n})
}

// UsageCSV — GET /api/admin/v1/reports/usage.csv
func (h *ReportsHandler) UsageCSV(w http.ResponseWriter, r *http.Request) {
	f, err := parseUsageFilter(r)
	if err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"filter": err.Error()})
		return
	}
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, f.TenantID)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}
	f.AllowedTenantIDs = allowed

	aggregates, err := h.usageRepo.GetAggregates(r.Context(), *f)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	filename := fmt.Sprintf("usage_%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"tenant_id", "tenant_name", "year", "month",
		"total_requests", "successful_requests", "failed_requests",
		"rejected_requests", "avg_latency_ms",
	})

	for _, a := range aggregates {
		_ = cw.Write([]string{
			a.TenantID.String(),
			a.TenantName,
			strconv.Itoa(a.Year),
			strconv.Itoa(a.Month),
			strconv.Itoa(a.TotalRequests),
			strconv.Itoa(a.SuccessfulRequests),
			strconv.Itoa(a.FailedRequests),
			strconv.Itoa(a.RejectedRequests),
			fmt.Sprintf("%.1f", a.AvgLatencyMs),
		})
	}

	cw.Flush()
}

// Summary — GET /api/admin/v1/reports/usage/summary
func (h *ReportsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, nil)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}
	summary, err := h.usageRepo.GetDashboardSummary(r.Context(), allowed)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, summary)
}

// TopIPs — GET /api/admin/v1/reports/top-ips
// Devuelve las IPs más activas en un período
func (h *ReportsHandler) TopIPs(w http.ResponseWriter, r *http.Request) {
	f, err := parseUsageFilter(r)
	if err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"filter": err.Error()})
		return
	}
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, f.TenantID)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	ips, err := h.usageRepo.GetTopIPs(r.Context(), f.From, f.To, 50, allowed)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": ips})
}

// TopUserAgents — GET /api/admin/v1/reports/top-user-agents
// Devuelve los User-Agents más comunes
func (h *ReportsHandler) TopUserAgents(w http.ResponseWriter, r *http.Request) {
	f, err := parseUsageFilter(r)
	if err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"filter": err.Error()})
		return
	}
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, f.TenantID)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	agents, err := h.usageRepo.GetTopUserAgents(r.Context(), f.From, f.To, 50, allowed)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": agents})
}

// TopCountries — GET /api/admin/v1/reports/top-countries
// Devuelve los países con más acceso
func (h *ReportsHandler) TopCountries(w http.ResponseWriter, r *http.Request) {
	f, err := parseUsageFilter(r)
	if err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"filter": err.Error()})
		return
	}
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, f.TenantID)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	countries, err := h.usageRepo.GetTopCountries(r.Context(), f.From, f.To, 50, allowed)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": countries})
}

// FailureBreakdown — GET /api/admin/v1/reports/failures
//
// Devuelve dos grupos de fallos:
//   - proxy_errors: errores y rechazos que llegaron al proxy (upstream_error, quota_exceeded)
//   - auth_failures: conexiones bloqueadas antes del proxy (clave inválida, IP bloqueada, etc.)
func (h *ReportsHandler) FailureBreakdown(w http.ResponseWriter, r *http.Request) {
	f, err := parseUsageFilter(r)
	if err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"filter": err.Error()})
		return
	}
	allowed, apiErr := resolveTenantScope(r.Context(), h.userRepo, f.TenantID)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	// Errores registrados en usage_events (llegaron al proxy, fallaron en upstream o fueron rechazados por cuota)
	upstreamStats, err := h.usageRepo.GetUpstreamFailureStats(r.Context(), f.TenantID, allowed, f.From, f.To)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Rechazos previos al proxy (autenticación, rate limit, IP bloqueada…)
	authStats, err := h.failedRepo.GetBreakdown(r.Context(), f.TenantID, allowed, f.From, f.To)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Construir lista proxy_errors
	proxyErrors := make([]*FailureDetail, 0, len(upstreamStats))
	totalProxy := 0
	for _, s := range upstreamStats {
		fd := toFailureDetail(s.Reason, s.Count, s.LastSeen)
		proxyErrors = append(proxyErrors, fd)
		totalProxy += s.Count
	}
	sort.Slice(proxyErrors, func(i, j int) bool { return proxyErrors[i].Count > proxyErrors[j].Count })

	// Construir lista auth_failures
	authFailures := make([]*FailureDetail, 0, len(authStats))
	totalAuth := 0
	for _, s := range authStats {
		fd := toFailureDetail(s.Reason, s.Count, s.LastSeen)
		authFailures = append(authFailures, fd)
		totalAuth += s.Count
	}
	sort.Slice(authFailures, func(i, j int) bool { return authFailures[i].Count > authFailures[j].Count })

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"proxy_errors":       proxyErrors,
		"auth_failures":      authFailures,
		"total_proxy_errors": totalProxy,
		"total_auth_failures": totalAuth,
	})
}

// toFailureDetail construye un FailureDetail resolviendo la etiqueta y categoría desde failureMeta.
func toFailureDetail(reason string, count int, lastSeen time.Time) *FailureDetail {
	fd := &FailureDetail{
		Reason:   reason,
		Count:    count,
		LastSeen: lastSeen,
		Label:    reason,
		Category: "other",
	}
	if meta, ok := failureMeta[reason]; ok {
		fd.Label = meta.label
		fd.Category = meta.category
	}
	return fd
}

// ─── helpers ─────────────────────────────────────────────────

func parseUsageFilter(r *http.Request) (*postgres.UsageFilter, error) {
	q := r.URL.Query()

	now := time.Now()
	// Default: mes actual
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := now

	if s := q.Get("from"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, fmt.Errorf("invalid from date, use YYYY-MM-DD")
		}
		from = t
	}
	if s := q.Get("to"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, fmt.Errorf("invalid to date, use YYYY-MM-DD")
		}
		to = t.Add(24*time.Hour - time.Second)
	}

	f := &postgres.UsageFilter{
		From:        from,
		To:          to,
		Granularity: q.Get("granularity"),
	}

	if tid := q.Get("tenant_id"); tid != "" {
		id, err := uuid.Parse(tid)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant_id")
		}
		f.TenantID = &id
	}

	return f, nil
}
