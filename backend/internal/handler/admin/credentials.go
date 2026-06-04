package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	credsvc "github.com/bigdavi/tsa-proxy/internal/service/credential"
)

type CredentialsHandler struct {
	svc *credsvc.Service
}

func NewCredentialsHandler(svc *credsvc.Service) *CredentialsHandler {
	return &CredentialsHandler{svc: svc}
}

// ListByTenant — GET /api/admin/v1/tenants/:id/credentials
func (h *CredentialsHandler) ListByTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	creds, err := h.svc.ListByTenant(r.Context(), tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": creds})
}

// Create — POST /api/admin/v1/tenants/:id/credentials
func (h *CredentialsHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Name      *string    `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	result, err := h.svc.Create(r.Context(), tenantID, req.Name)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// api_key y stamp_url solo se devuelven en este momento
	apierr.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         result.ID,
		"tenant_id":  result.TenantID,
		"key_id":     result.KeyID,
		"api_key":    result.APIKey, // ← única vez visible
		"key_prefix": result.KeyPrefix,
		"url_token":  result.URLToken,
		"stamp_url":  result.StampURL, // ← URL para software de firma
		"name":       result.Name,
		"status":     result.Status,
		"created_at": result.CreatedAt,
		"warning":    "Save this API key now. It cannot be retrieved again.",
	})
}

// Rotate — POST /api/admin/v1/credentials/:id/rotate
func (h *CredentialsHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	credID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	result, err := h.svc.Rotate(r.Context(), credID, actorID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":         result.ID,
		"tenant_id":  result.TenantID,
		"key_id":     result.KeyID,
		"api_key":    result.APIKey, // ← única vez visible
		"key_prefix": result.KeyPrefix,
		"url_token":  result.URLToken,
		"stamp_url":  result.StampURL, // ← URL para software de firma
		"name":       result.Name,
		"status":     result.Status,
		"created_at": result.CreatedAt,
		"warning":    "Save this API key now. It cannot be retrieved again.",
	})
}

// Revoke — POST /api/admin/v1/credentials/:id/revoke
func (h *CredentialsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	credID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.svc.Revoke(r.Context(), credID, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
