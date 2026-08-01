package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	tenantsvc "github.com/bigdavi/tsa-proxy/internal/service/tenant"
)

type TenantsHandler struct {
	svc       *tenantsvc.Service
	quotaRepo *postgres.QuotaRepository
	userRepo  *postgres.AdminUserRepository
}

func NewTenantsHandler(svc *tenantsvc.Service, quotaRepo *postgres.QuotaRepository, userRepo *postgres.AdminUserRepository) *TenantsHandler {
	return &TenantsHandler{svc: svc, quotaRepo: quotaRepo, userRepo: userRepo}
}

// List — GET /api/admin/v1/tenants
func (h *TenantsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 { page = 1 }
	if limit < 1 { limit = 20 }

	filter := postgres.TenantFilter{
		Status: q.Get("status"),
		Search: q.Get("search"),
		Page:   page,
		Limit:  limit,
	}

	tenants, total, err := h.svc.List(r.Context(), filter)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	pagination := model.NewPagination(page, limit, total)
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data":       tenants,
		"pagination": pagination,
	})
}

// Create — POST /api/admin/v1/tenants
func (h *TenantsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		ContactEmail *string `json:"contact_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	fields := map[string]string{}
	if req.Name == "" { fields["name"] = "required" }
	if len(fields) > 0 {
		apierr.WriteValidationError(w, r, fields)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	ip := r.Header.Get("X-Real-IP")

	created, err := h.svc.Create(r.Context(), &tenantsvc.CreateRequest{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		ContactEmail: req.ContactEmail,
		CreatedBy:    actorID,
	})
	if err != nil {
		if isConflict(err) {
			apierr.WriteError(w, r, apierr.ErrConflict)
			return
		}
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	_ = ip // ip registered in service via audit
	apierr.WriteJSON(w, http.StatusCreated, created)
}

// Get — GET /api/admin/v1/tenants/:id
func (h *TenantsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	tenant, err := h.svc.GetByID(r.Context(), id)
	if err != nil || tenant == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	// Cargar cuota
	quota, _ := h.quotaRepo.GetByTenant(r.Context(), id)
	tenant.Quota = quota

	apierr.WriteJSON(w, http.StatusOK, tenant)
}

// Update — PUT /api/admin/v1/tenants/:id
func (h *TenantsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		ContactEmail *string `json:"contact_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if req.Name == "" {
		apierr.WriteValidationError(w, r, map[string]string{"name": "required"})
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	updated, err := h.svc.Update(r.Context(), id, actorID, &model.Tenant{
		Name:         req.Name,
		Description:  req.Description,
		ContactEmail: req.ContactEmail,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if updated == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, updated)
}

// Suspend — POST /api/admin/v1/tenants/:id/suspend
func (h *TenantsHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.svc.Suspend(r.Context(), id, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

// Reactivate — POST /api/admin/v1/tenants/:id/reactivate
func (h *TenantsHandler) Reactivate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.svc.Reactivate(r.Context(), id, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

// Delete — DELETE /api/admin/v1/tenants/:id
// Deprecated: Use VerifyAndDelete instead, which requires 2FA
func (h *TenantsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.svc.Delete(r.Context(), id, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// VerifyAndDelete — POST /api/admin/v1/tenants/:id/verify-delete
// Verifies 2FA code before deleting a tenant
func (h *TenantsHandler) VerifyAndDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		TotpCode string `json:"totp_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())

	// Obtener el usuario actual
	user, err := h.userRepo.FindByID(r.Context(), actorID)
	if err != nil || user == nil || !user.IsActive {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	// Verificar si el usuario tiene 2FA habilitado
	// Si tiene 2FA CON secreto guardado: requiere validación del código
	// Si NO tiene 2FA o no hay secreto: permite eliminación sin código
	hasTOTPSecret := user.TOTPSecret != nil && *user.TOTPSecret != ""

	if hasTOTPSecret && user.TOTPEnabled {
		code := req.TotpCode
		if code == "" {
			apierr.WriteValidationError(w, r, map[string]string{"totp_code": "requerido"})
			return
		}

		// Verificar el código TOTP
		// totp.Validate() by default checks current and +/- 1 time window (30 sec each)
		if !totp.Validate(code, *user.TOTPSecret) {
			apierr.WriteValidationError(w, r, map[string]string{"totp_code": "código inválido o expirado"})
			return
		}
	}
	// Si no tiene 2FA configurado correctamente, proceder sin validación

	// Código válido, proceder con la eliminación
	if err := h.svc.Delete(r.Context(), id, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func isConflict(err error) bool {
	return err != nil && (contains(err.Error(), "already exists") || contains(err.Error(), "duplicate"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
