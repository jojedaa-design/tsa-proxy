package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
)

type NoAuthHandler struct {
	repo *postgres.NoAuthRepository
}

func NewNoAuthHandler(repo *postgres.NoAuthRepository) *NoAuthHandler {
	return &NoAuthHandler{repo: repo}
}

// GetByTenant devuelve el estado de noauth_access de un tenant.
// GET /api/admin/v1/tenants/{id}/noauth
func (h *NoAuthHandler) GetByTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	na, err := h.repo.GetByTenant(r.Context(), tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if na == nil {
		// No existe → acceso no habilitado
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"access":  nil,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": true,
		"access":  na,
	})
}

// Enable habilita el acceso no-auth para un tenant.
// POST /api/admin/v1/tenants/{id}/noauth/enable
func (h *NoAuthHandler) Enable(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var body struct {
		Name *string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	ctx := r.Context()

	// Si ya existe, solo activarlo
	existing, err := h.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	if existing != nil {
		if err := h.repo.Activate(ctx, tenantID); err != nil {
			apierr.WriteError(w, r, apierr.ErrInternal)
			return
		}
		existing.Status = "active"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": true,
			"access":  existing,
			"message": "Acceso TSP sin credenciales reactivado. Agrega IPs al allowlist para habilitar el acceso.",
		})
		return
	}

	na, err := h.repo.Create(ctx, tenantID, body.Name)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": true,
		"access":  na,
		"message": "Acceso TSP sin credenciales habilitado. Por defecto BLOQUEADO — agrega IPs al allowlist para permitir el acceso.",
	})
}

// Disable suspende el acceso no-auth para un tenant.
// POST /api/admin/v1/tenants/{id}/noauth/disable
func (h *NoAuthHandler) Disable(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if err := h.repo.Suspend(r.Context(), tenantID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Acceso TSP sin credenciales suspendido.",
	})
}

// Delete elimina el acceso no-auth para un tenant.
// DELETE /api/admin/v1/tenants/{id}/noauth
func (h *NoAuthHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if err := h.repo.Delete(r.Context(), tenantID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
