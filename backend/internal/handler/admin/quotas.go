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

// Update — PUT /api/admin/v1/tenants/:id/quota (solo burst_per_minute)
func (h *QuotasHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		BurstPerMinute int `json:"burst_per_minute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if req.BurstPerMinute <= 0 {
		apierr.WriteValidationError(w, r, map[string]string{
			"burst_per_minute": "must be > 0",
		})
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	before, _ := h.quotaRepo.GetByTenant(r.Context(), tenantID)

	upserted, err := h.quotaRepo.Upsert(r.Context(), &model.TenantQuota{
		TenantID:       tenantID,
		BurstPerMinute: req.BurstPerMinute,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	if h.cache != nil {
		go func() { _ = h.cache.InvalidateTenantQuota(context.Background(), tenantID) }()
	}

	go func() {
		changes := map[string]interface{}{
			"burst_per_minute": map[string]int{
				"before": before.BurstPerMinute,
				"after":  req.BurstPerMinute,
			},
		}
		eid := tenantID
		_ = h.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "quota.update",
			EntityType: "tenant",
			EntityID:   &eid,
			TenantID:   &eid,
			Changes:    changes,
		})
	}()

	apierr.WriteJSON(w, http.StatusOK, upserted)
}

// AddBundle — POST /api/admin/v1/tenants/:id/bundles (recargar bolsa)
func (h *QuotasHandler) AddBundle(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Amount int     `json:"amount"`
		Note   *string `json:"note,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if req.Amount <= 0 {
		apierr.WriteValidationError(w, r, map[string]string{
			"amount": "must be > 0",
		})
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())

	bundle, err := h.quotaRepo.AddBundle(r.Context(), &model.QuotaBundle{
		TenantID:  tenantID,
		Amount:    req.Amount,
		Note:      req.Note,
		CreatedBy: &actorID,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	if h.cache != nil {
		go func() { _ = h.cache.InvalidateTenantQuota(context.Background(), tenantID) }()
	}

	go func() {
		eid := tenantID
		_ = h.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "quota.bundle.add",
			EntityType: "tenant",
			EntityID:   &eid,
			TenantID:   &eid,
			Changes: map[string]interface{}{
				"bundle_id": bundle.ID,
				"amount":    req.Amount,
				"note":      req.Note,
			},
		})
	}()

	apierr.WriteJSON(w, http.StatusCreated, bundle)
}

// ListBundles — GET /api/admin/v1/tenants/:id/bundles (histórico)
func (h *QuotasHandler) ListBundles(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	bundles, err := h.quotaRepo.ListBundles(r.Context(), tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	if bundles == nil {
		bundles = []*model.QuotaBundle{}
	}

	apierr.WriteJSON(w, http.StatusOK, bundles)
}
