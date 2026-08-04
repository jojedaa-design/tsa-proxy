package admin

import (
	"encoding/json"
	"net/http"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	authsvc "github.com/bigdavi/tsa-proxy/internal/service/auth"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
)

// AuthHandler maneja los endpoints de autenticación admin.
type AuthHandler struct {
	svc        *authsvc.Service
	userRepo   *postgres.AdminUserRepository
	tenantRepo *postgres.TenantRepository
}

func NewAuthHandler(svc *authsvc.Service, userRepo *postgres.AdminUserRepository, tenantRepo *postgres.TenantRepository) *AuthHandler {
	return &AuthHandler{svc: svc, userRepo: userRepo, tenantRepo: tenantRepo}
}

// Login autentica a un administrador.
// POST /api/admin/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		apierr.WriteValidationError(w, r, map[string]string{
			"username": "required",
			"password": "required",
		})
		return
	}

	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.RemoteAddr
	}

	result, err := h.svc.Login(r.Context(), req.Username, req.Password, ip)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	// Si requiere 2FA, devolver mfa_token en lugar de JWT
	if result.TOTPRequired {
		apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"mfa_required": true,
			"mfa_token":    result.MFAToken,
		})
		return
	}

	// 2FA es obligatorio: si el usuario aún no lo activó, no se emite sesión —
	// debe completar el setup con setup_token antes de poder continuar.
	if result.TOTPSetupRequired {
		apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"totp_setup_required": true,
			"setup_token":         result.SetupToken,
		})
		return
	}

	h.setRefreshCookie(w, result.RefreshToken)
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": result.AccessToken,
		"expires_in":   result.ExpiresIn,
		"user": map[string]interface{}{
			"id":       result.User.ID,
			"username": result.User.Username,
			"email":    result.User.Email,
			"roles":    result.User.Roles,
		},
	})
}

// VerifyTOTP verifica el código TOTP y emite el JWT final.
// POST /api/admin/v1/auth/2fa/verify
func (h *AuthHandler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if req.MFAToken == "" || req.Code == "" {
		apierr.WriteValidationError(w, r, map[string]string{
			"mfa_token": "required",
			"code":      "required",
		})
		return
	}

	result, err := h.svc.VerifyTOTP(r.Context(), req.MFAToken, req.Code)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken)
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": result.AccessToken,
		"expires_in":   result.ExpiresIn,
		"user": map[string]interface{}{
			"id":       result.User.ID,
			"username": result.User.Username,
			"email":    result.User.Email,
			"roles":    result.User.Roles,
		},
	})
}

// SetupTOTP genera un secreto TOTP y devuelve la URL para el QR.
// GET /api/admin/v1/auth/2fa/setup
func (h *AuthHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetAdminUserID(r.Context())
	if !ok {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	user, err := h.userRepo.FindByID(r.Context(), userID)
	if err != nil || user == nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	setup, err := h.svc.SetupTOTP(r.Context(), userID, user.Username)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"secret":     setup.Secret,
		"qr_url":     setup.QRCodeURL,
		"totp_enabled": user.TOTPEnabled,
	})
}

// EnableTOTP activa 2FA verificando el primer código.
// POST /api/admin/v1/auth/2fa/enable
func (h *AuthHandler) EnableTOTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetAdminUserID(r.Context())
	if !ok {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		apierr.WriteValidationError(w, r, map[string]string{"code": "required"})
		return
	}

	if err := h.svc.EnableTOTP(r.Context(), userID, req.Code); err != nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "2fa_enabled"})
}

// SetupTOTPForLogin genera el secreto TOTP pendiente para un usuario que aún no
// tiene 2FA, usando el setup_token emitido por Login (sin JWT — el usuario
// todavía no tiene sesión).
// POST /api/admin/v1/auth/2fa/setup-required
func (h *AuthHandler) SetupTOTPForLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SetupToken string `json:"setup_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SetupToken == "" {
		apierr.WriteValidationError(w, r, map[string]string{"setup_token": "required"})
		return
	}

	setup, err := h.svc.SetupTOTPWithToken(r.Context(), req.SetupToken)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"secret": setup.Secret,
		"qr_url": setup.QRCodeURL,
	})
}

// CompleteTOTPSetup activa 2FA y completa el login en un solo paso — usado en
// el flujo de setup obligatorio (usuario sin sesión todavía).
// POST /api/admin/v1/auth/2fa/complete-setup
func (h *AuthHandler) CompleteTOTPSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SetupToken string `json:"setup_token"`
		Code       string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if req.SetupToken == "" || req.Code == "" {
		apierr.WriteValidationError(w, r, map[string]string{
			"setup_token": "required",
			"code":        "required",
		})
		return
	}

	result, err := h.svc.CompleteTOTPSetup(r.Context(), req.SetupToken, req.Code)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken)
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": result.AccessToken,
		"expires_in":   result.ExpiresIn,
		"user": map[string]interface{}{
			"id":       result.User.ID,
			"username": result.User.Username,
			"email":    result.User.Email,
			"roles":    result.User.Roles,
		},
	})
}

func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, token string) {
	// Sin MaxAge/Expires: cookie de sesión de navegador. El JWT del refresh
	// token sigue teniendo su propio TTL (JWT_REFRESH_TTL) como backstop de
	// seguridad server-side, pero la cookie en sí debe desaparecer al cerrar
	// el navegador para no dejar sesiones abiertas indefinidamente.
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/api/admin/v1/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// Logout invalida la sesión.
// POST /api/admin/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		_ = h.svc.Logout(r.Context(), cookie.Value)
	}

	// Limpiar cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/api/admin/v1/auth",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusNoContent)
}

// Refresh renueva el access token usando el refresh token de la cookie.
// POST /api/admin/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	accessToken, err := h.svc.Refresh(r.Context(), cookie.Value)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": accessToken,
	})
}

// Me devuelve la información del admin autenticado.
// GET /api/admin/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetAdminUserID(r.Context())
	if !ok {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	user, err := h.userRepo.FindByID(r.Context(), userID)
	if err != nil || user == nil {
		apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
		return
	}

	// tenant_scope: solo relevante para viewer; vacío/omitido = todos los tenants.
	var tenantScope []string
	var tenantScopeDetail []map[string]interface{}
	scope, _ := h.userRepo.GetTenantScope(r.Context(), user.ID)
	for _, tid := range scope {
		tenantScope = append(tenantScope, tid.String())
		if tenant, err := h.tenantRepo.GetByID(r.Context(), tid); err == nil && tenant != nil {
			tenantScopeDetail = append(tenantScopeDetail, map[string]interface{}{
				"id": tenant.ID, "name": tenant.Name,
			})
		}
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":                  user.ID,
		"username":            user.Username,
		"email":               user.Email,
		"roles":               user.Roles,
		"tenant_scope":        tenantScope,
		"tenant_scope_detail": tenantScopeDetail,
		"totp_enabled":  user.TOTPEnabled,
		"last_login_at": user.LastLoginAt,
	})
}
