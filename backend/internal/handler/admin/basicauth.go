package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	basicauthsvc "github.com/bigdavi/tsa-proxy/internal/service/basicauth"
)

type BasicAuthHandler struct {
	svc *basicauthsvc.Service
}

func NewBasicAuthHandler(svc *basicauthsvc.Service) *BasicAuthHandler {
	return &BasicAuthHandler{svc: svc}
}

// ListByTenant — GET /api/admin/v1/tenants/:id/basic-auth
func (h *BasicAuthHandler) ListByTenant(w http.ResponseWriter, r *http.Request) {
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
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data":         creds,
		"tsa_endpoint": basicauthsvc.TSAEndpoint(),
	})
}

// Create — POST /api/admin/v1/tenants/:id/basic-auth
func (h *BasicAuthHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Username string  `json:"username"`
		Name     *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if req.Username == "" {
		apierr.WriteValidationError(w, r, map[string]string{"username": "required"})
		return
	}

	result, err := h.svc.Create(r.Context(), tenantID, req.Username, req.Name)
	if err != nil {
		apierr.WriteError(w, r, &apierr.APIError{
			StatusCode: http.StatusConflict,
			Code:       "username_taken",
			Message:    err.Error(),
		})
		return
	}

	apierr.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           result.ID,
		"tenant_id":    result.TenantID,
		"username":     result.Username,
		"password":     result.Password, // única vez visible
		"key_prefix":   result.KeyPrefix,
		"name":         result.Name,
		"status":       result.Status,
		"tsa_endpoint": basicauthsvc.TSAEndpoint(),
		"created_at":   result.CreatedAt,
		"warning":      "Save this password now. It cannot be retrieved again.",
	})
}

// Revoke — POST /api/admin/v1/basic-auth/:id/revoke
func (h *BasicAuthHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.svc.Revoke(r.Context(), id, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
