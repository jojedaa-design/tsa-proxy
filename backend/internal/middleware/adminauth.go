package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/config"
	"github.com/bigdavi/tsa-proxy/internal/model"
)

// AdminClaims define los claims del JWT para administradores.
type AdminClaims struct {
	jwt.RegisteredClaims
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// AdminAuthMiddleware valida JWT de usuarios administradores.
type AdminAuthMiddleware struct {
	cfg *config.Config
}

func NewAdminAuthMiddleware(cfg *config.Config) *AdminAuthMiddleware {
	return &AdminAuthMiddleware{cfg: cfg}
}

// Require verifica JWT en el header Authorization.
func (m *AdminAuthMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		claims := &AdminClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, apierr.ErrAdminUnauthorized
			}
			return []byte(m.cfg.JWT.Secret), nil
		})

		if err != nil || !token.Valid {
			apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			apierr.WriteError(w, r, apierr.ErrAdminUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, model.CtxKeyAdminUser, userID)
		ctx = context.WithValue(ctx, model.CtxKeyAdminRoles, claims.Roles)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole verifica que el usuario tenga al menos uno de los roles requeridos.
func (m *AdminAuthMiddleware) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRoles, ok := r.Context().Value(model.CtxKeyAdminRoles).([]string)
			if !ok {
				apierr.WriteError(w, r, apierr.ErrForbidden)
				return
			}
			for _, required := range roles {
				for _, ur := range userRoles {
					if ur == required || ur == "superadmin" {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			apierr.WriteError(w, r, apierr.ErrForbidden)
		})
	}
}

// GetAdminUserID extrae el UUID del admin autenticado desde el contexto.
func GetAdminUserID(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(model.CtxKeyAdminUser).(uuid.UUID)
	return v, ok
}

// GetAdminRoles extrae los roles del admin desde el contexto.
func GetAdminRoles(ctx context.Context) []string {
	v, _ := ctx.Value(model.CtxKeyAdminRoles).([]string)
	return v
}
