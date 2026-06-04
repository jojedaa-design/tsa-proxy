package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

type QuotasHandler struct {
	quotaRepo *postgres.QuotaRepository
	audit     *postgres.AuditRepository
	cache     *rediscache.Cache
}

func NewQuotasHandler(quotaRepo *postgres.QuotaRepository, audit *postgres.AuditRepository, cache *rediscache.Cache) *QuotasHandler {
	return &QuotasHandler{quotaRepo: quotaRepo, audit: audit, cache: cache}
}

// Get — GET /api/admin/v1/tenants/:id/quota
func (h *QuotasHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	quota, err := h.quotaRepo.GetByTenant(r.Context(), tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if quota == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, quota)
}

// Update — PUT /api/admin/v1/tenants/:id/quota
func (h *QuotasHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		PlanID         *uuid.UUID `json:"plan_id"`
		MonthlyLimit   int        `json:"monthly_limit"`
		BurstPerMinute int        `json:"burst_per_minute"`
		HardLimit      bool       `json:"hard_limit"`
		AutoSuspend    bool       `json:"auto_suspend"`
		ResetDay       int        `json:"reset_day"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	fields := map[string]string{}
	if req.MonthlyLimit <= 0 { fields["monthly_limit"] = "must be > 0" }
	if req.BurstPerMinute <= 0 { fields["burst_per_minute"] = "must be > 0" }
	if req.ResetDay < 1 || req.ResetDay > 28 { fields["reset_day"] = "must be between 1 and 28" }
	if len(fields) > 0 {
		apierr.WriteValidationError(w, r, fields)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())

	before, _ := h.quotaRepo.GetByTenant(r.Context(), tenantID)

	upserted, err := h.quotaRepo.Upsert(r.Context(), &model.TenantQuota{
		TenantID:       tenantID,
		PlanID:         req.PlanID,
		MonthlyLimit:   req.MonthlyLimit,
		BurstPerMinute: req.BurstPerMinute,
		HardLimit:      req.HardLimit,
		AutoSuspend:    req.AutoSuspend,
		ResetDay:       req.ResetDay,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Invalidar cache de quota
	if h.cache != nil {
		go func() { _ = h.cache.InvalidateTenantQuota(context.Background(), tenantID) }()
	}

	go func() {
		changes := map[string]interface{}{}
		if before != nil {
			changes["before"] = map[string]interface{}{
				"monthly_limit":    before.MonthlyLimit,
				"burst_per_minute": before.BurstPerMinute,
			}
		}
		changes["after"] = map[string]interface{}{
			"monthly_limit":    req.MonthlyLimit,
			"burst_per_minute": req.BurstPerMinute,
		}
		eid := tenantID
		_ = h.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "quota.update",
			EntityType: "tenant",
			EntityID:   &eid,
			Changes:    changes,
		})
	}()

	apierr.WriteJSON(w, http.StatusOK, upserted)
}
