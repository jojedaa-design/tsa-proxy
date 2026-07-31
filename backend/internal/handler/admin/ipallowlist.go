package admin

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

type IPAllowlistHandler struct {
	repo  *postgres.IPAllowlistRepository
	cache *rediscache.Cache
	audit *postgres.AuditRepository
}

func NewIPAllowlistHandler(
	repo *postgres.IPAllowlistRepository,
	cache *rediscache.Cache,
	audit *postgres.AuditRepository,
) *IPAllowlistHandler {
	return &IPAllowlistHandler{repo: repo, cache: cache, audit: audit}
}

// ListByTenant — GET /api/admin/v1/tenants/:id/ip-allowlist
func (h *IPAllowlistHandler) ListByTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	entries, err := h.repo.FindAll(r.Context(), tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": entries})
}

// Create — POST /api/admin/v1/tenants/:id/ip-allowlist
func (h *IPAllowlistHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		CIDR  string  `json:"cidr"`
		Label *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if req.CIDR == "" {
		apierr.WriteValidationError(w, r, map[string]string{"cidr": "required"})
		return
	}

	// Normalizar: si es una IP sin máscara, añadir /32 o /128
	cidr := req.CIDR
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		// Intentar como IP individual
		ip := net.ParseIP(cidr)
		if ip == nil {
			apierr.WriteValidationError(w, r, map[string]string{"cidr": "invalid IP or CIDR format"})
			return
		}
		if ip.To4() != nil {
			cidr = cidr + "/32"
		} else {
			cidr = cidr + "/128"
		}
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	created, err := h.repo.Create(r.Context(), &model.IPAllowlistEntry{
		TenantID:  tenantID,
		CIDR:      cidr,
		Label:     req.Label,
		CreatedBy: &actorID,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Invalidar cache
	_ = h.cache.InvalidateIPAllowlist(r.Context(), tenantID)

	go func() {
		eid := created.ID
		_ = h.audit.Insert(r.Context(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "ip_allowlist.create",
			EntityType: "ip_allowlist",
			EntityID:   &eid,
			TenantID:   &tenantID,
			Changes:    map[string]interface{}{"cidr": cidr, "tenant_id": tenantID},
		})
	}()

	apierr.WriteJSON(w, http.StatusCreated, created)
}

// Update — PUT /api/admin/v1/ip-allowlist/:id
func (h *IPAllowlistHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Label *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	updated, err := h.repo.Update(r.Context(), id, req.Label)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if updated == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	_ = h.cache.InvalidateIPAllowlist(r.Context(), updated.TenantID)
	apierr.WriteJSON(w, http.StatusOK, updated)
}

// Delete — DELETE /api/admin/v1/ip-allowlist/:id
func (h *IPAllowlistHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
