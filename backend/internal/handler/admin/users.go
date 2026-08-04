package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	authsvc "github.com/bigdavi/tsa-proxy/internal/service/auth"
)

// UsersHandler gestiona los usuarios administrativos de la plataforma
// (creación de cuentas admin/viewer y su scope de tenants).
type UsersHandler struct {
	repo       *postgres.AdminUserRepository
	tenantRepo *postgres.TenantRepository
	authSvc    *authsvc.Service
}

func NewUsersHandler(repo *postgres.AdminUserRepository, tenantRepo *postgres.TenantRepository, authSvc *authsvc.Service) *UsersHandler {
	return &UsersHandler{repo: repo, tenantRepo: tenantRepo, authSvc: authSvc}
}

// userResponse es la forma pública de un AdminUser (sin password_hash ni totp_secret).
type userResponse struct {
	ID          uuid.UUID  `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	IsActive    bool       `json:"is_active"`
	TOTPEnabled bool       `json:"totp_enabled"`
	Role        string     `json:"role"`
	TenantScope []string   `json:"tenant_scope"` // vacío = todos los tenants
	CreatedAt   string     `json:"created_at"`
	LastLoginAt *string    `json:"last_login_at,omitempty"`
}

func (h *UsersHandler) toResponse(r *http.Request, u *model.AdminUser) userResponse {
	role := ""
	if len(u.Roles) > 0 {
		role = u.Roles[0]
	}
	scope, _ := h.repo.GetTenantScope(r.Context(), u.ID)
	scopeStrs := make([]string, len(scope))
	for i, tid := range scope {
		scopeStrs[i] = tid.String()
	}
	var lastLogin *string
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
		lastLogin = &s
	}
	return userResponse{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		IsActive:    u.IsActive,
		TOTPEnabled: u.TOTPEnabled,
		Role:        role,
		TenantScope: scopeStrs,
		CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		LastLoginAt: lastLogin,
	}
}

// List — GET /api/admin/v1/users
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	users, total, err := h.repo.List(r.Context(), postgres.AdminUserFilter{
		Search: q.Get("search"),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	data := make([]userResponse, len(users))
	for i, u := range users {
		data[i] = h.toResponse(r, u)
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": model.NewPagination(page, limit, total),
	})
}

// Get — GET /api/admin/v1/users/{id}
func (h *UsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	u, err := h.repo.FindByID(r.Context(), id)
	if err != nil || u == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, h.toResponse(r, u))
}

type userWriteRequest struct {
	Username  string   `json:"username"`
	Email     string   `json:"email"`
	Password  string   `json:"password"`
	IsActive  *bool    `json:"is_active"`
	Role      string   `json:"role"`
	TenantIDs []string `json:"tenant_ids"`
}

// Create — POST /api/admin/v1/users
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req userWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	fields := map[string]string{}
	if req.Username == "" {
		fields["username"] = "required"
	}
	if req.Email == "" {
		fields["email"] = "required"
	}
	if len(req.Password) < 8 {
		fields["password"] = "minimum 8 characters"
	}
	if len(fields) > 0 {
		apierr.WriteValidationError(w, r, fields)
		return
	}

	role, tenantIDs, apiErr := h.validateRoleAndScope(r, req.Role, req.TenantIDs)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	hash, err := authsvc.HashPassword(req.Password)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	created, err := h.repo.Create(r.Context(), &model.AdminUser{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrConflict)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.repo.SetRole(r.Context(), created.ID, role.ID, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if role.Name == "viewer" {
		if err := h.repo.SetTenantScope(r.Context(), created.ID, tenantIDs); err != nil {
			apierr.WriteError(w, r, apierr.ErrInternal)
			return
		}
	}

	u, _ := h.repo.FindByID(r.Context(), created.ID)
	apierr.WriteJSON(w, http.StatusCreated, h.toResponse(r, u))
}

// Update — PUT /api/admin/v1/users/{id}
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req userWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if req.Email == "" {
		apierr.WriteValidationError(w, r, map[string]string{"email": "required"})
		return
	}

	role, tenantIDs, apiErr := h.validateRoleAndScope(r, req.Role, req.TenantIDs)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	// Protección del último superadmin: si el target es superadmin y esta
	// actualización lo desactiva o le quita el rol, no debe quedar en cero.
	if err := h.guardLastSuperadmin(r, id, role.Name, isActive); err != nil {
		apierr.WriteError(w, r, err)
		return
	}

	updated, err := h.repo.Update(r.Context(), id, req.Email, isActive)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if updated == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	actorID, _ := middleware.GetAdminUserID(r.Context())
	if err := h.repo.SetRole(r.Context(), id, role.ID, actorID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if err := h.repo.SetTenantScope(r.Context(), id, tenantIDs); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	u, _ := h.repo.FindByID(r.Context(), id)
	apierr.WriteJSON(w, http.StatusOK, h.toResponse(r, u))
}

// Delete — DELETE /api/admin/v1/users/{id}
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	u, err := h.repo.FindByID(r.Context(), id)
	if err != nil || u == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}
	if apiErr := h.guardLastSuperadmin(r, id, "", false); apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	if err := h.repo.SoftDelete(r.Context(), id); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ResetPassword — POST /api/admin/v1/users/{id}/reset-password
func (h *UsersHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Password) < 8 {
		apierr.WriteValidationError(w, r, map[string]string{"password": "minimum 8 characters"})
		return
	}

	hash, err := authsvc.HashPassword(req.Password)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	if err := h.repo.SetPassword(r.Context(), id, hash); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

// ResetTOTP — POST /api/admin/v1/users/{id}/reset-2fa
// Desactiva 2FA de un usuario sin requerir su código actual (p.ej. perdió el
// dispositivo). El usuario deberá reconfigurar 2FA obligatoriamente en su
// próximo login.
func (h *UsersHandler) ResetTOTP(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	u, err := h.repo.FindByID(r.Context(), id)
	if err != nil || u == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	if err := h.authSvc.AdminResetTOTP(r.Context(), id); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "2fa_reset"})
}

// ListRoles — GET /api/admin/v1/roles
func (h *UsersHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.repo.ListRoles(r.Context())
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": roles})
}

// validateRoleAndScope valida el rol solicitado y, si es "viewer", el conjunto
// de tenant_ids (deben existir); para admin/superadmin, tenant_ids debe venir vacío.
//
// El rol "viewer" está limitado a exactamente un cliente: solo ve la sección
// "Reportes" con la información de ese cliente, nunca "todos los clientes"
// ni varios a la vez.
func (h *UsersHandler) validateRoleAndScope(r *http.Request, roleName string, tenantIDStrs []string) (*model.Role, []uuid.UUID, *apierr.APIError) {
	role, err := h.repo.GetRoleByName(r.Context(), roleName)
	if err != nil || role == nil {
		return nil, nil, &apierr.APIError{StatusCode: http.StatusBadRequest, Code: "validation_error", Message: "invalid role"}
	}

	if role.Name != "viewer" && len(tenantIDStrs) > 0 {
		return nil, nil, &apierr.APIError{StatusCode: http.StatusBadRequest, Code: "validation_error", Message: "tenant_ids only applies to the viewer role"}
	}
	if role.Name == "viewer" && len(tenantIDStrs) != 1 {
		return nil, nil, &apierr.APIError{StatusCode: http.StatusBadRequest, Code: "validation_error", Message: "viewer role requires exactly one tenant_id"}
	}

	tenantIDs := make([]uuid.UUID, 0, len(tenantIDStrs))
	for _, s := range tenantIDStrs {
		tid, err := uuid.Parse(s)
		if err != nil {
			return nil, nil, &apierr.APIError{StatusCode: http.StatusBadRequest, Code: "validation_error", Message: "invalid tenant_id: " + s}
		}
		tenant, err := h.tenantRepo.GetByID(r.Context(), tid)
		if err != nil || tenant == nil {
			return nil, nil, &apierr.APIError{StatusCode: http.StatusBadRequest, Code: "validation_error", Message: "tenant not found: " + s}
		}
		tenantIDs = append(tenantIDs, tid)
	}

	return role, tenantIDs, nil
}

// guardLastSuperadmin evita dejar la plataforma sin ningún superadmin activo.
// targetID es el usuario afectado; newRole/newActive describen el estado propuesto
// (si newRole == "" se interpreta como "el usuario va a ser eliminado").
func (h *UsersHandler) guardLastSuperadmin(r *http.Request, targetID uuid.UUID, newRole string, newActive bool) *apierr.APIError {
	target, err := h.repo.FindByID(r.Context(), targetID)
	if err != nil || target == nil {
		return nil
	}
	wasSuperadmin := false
	for _, role := range target.Roles {
		if role == "superadmin" {
			wasSuperadmin = true
		}
	}
	if !wasSuperadmin {
		return nil
	}

	willStaySuperadmin := newRole == "superadmin" && newActive
	if willStaySuperadmin {
		return nil
	}

	count, err := h.repo.CountByRole(r.Context(), "superadmin")
	if err != nil {
		return apierr.ErrInternal
	}
	if count <= 1 {
		return &apierr.APIError{StatusCode: http.StatusConflict, Code: "conflict", Message: "cannot remove the last superadmin"}
	}
	return nil
}
