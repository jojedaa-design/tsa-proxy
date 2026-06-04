package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

const tspCacheTTL = 5 * time.Minute

// TSPAuthMiddleware autentica los endpoints /ts y /tsp de dos formas:
//
//  1. Con credenciales (Authorization: Basic ...):
//     Valida usuario/contraseña contra basic_auth_credentials.
//     Compatible con Adobe Acrobat, JSignPdf y otros clientes tradicionales.
//
//  2. Sin credenciales (sin Authorization header):
//     Busca el tenant por IP en noauth_access + tenant_ip_allowlist.
//     Compatible con DAVISIGN y otros clientes que no envían preemptive auth.
//     Si la IP no está configurada → 401 Unauthorized con WWW-Authenticate
//     (para que el cliente reintente con credenciales).
//
// En ambos casos, después de autenticar, se inyecta el tenant_id en el context
// para que el pipeline de IP allowlist + rate limit + quota se apliquen.
type TSPAuthMiddleware struct {
	basicRepo  *postgres.BasicAuthRepository
	noauthRepo *postgres.NoAuthRepository
	cache      *rediscache.Cache
}

func NewTSPAuthMiddleware(
	basicRepo *postgres.BasicAuthRepository,
	noauthRepo *postgres.NoAuthRepository,
	cache *rediscache.Cache,
) *TSPAuthMiddleware {
	return &TSPAuthMiddleware{
		basicRepo:  basicRepo,
		noauthRepo: noauthRepo,
		cache:      cache,
	}
}

// Authenticate es el middleware principal del endpoint /tsp.
func (m *TSPAuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		clientIP := r.Header.Get("X-Real-IP")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		authHeader := r.Header.Get("Authorization")

		var tenantID uuid.UUID
		var credentialID uuid.UUID

		// Para GET sin Authorization: retorna 401 + WWW-Authenticate
		// (permite que clientes como DAVISIGN detecten que necesitan credenciales)
		if r.Method == "GET" && (authHeader == "" || !strings.HasPrefix(authHeader, "Basic ")) {
			w.Header().Set("WWW-Authenticate", `Basic realm="TSA Proxy"`)
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}

		if authHeader != "" && strings.HasPrefix(authHeader, "Basic ") {
			// ── Flujo 1: Basic Auth ──────────────────────────────
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
			if err != nil {
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			username := parts[0]
			password := parts[1]
			inputHash := sha256.Sum256([]byte(password))

			// Cache-first
			cached, cacheErr := m.cache.GetBasicAuth(ctx, username)
			if cacheErr == nil && cached != nil {
				if cached.CredStatus != "active" {
					apierr.WriteError(w, r, apierr.ErrUnauthorized)
					return
				}
				if cached.TenantStatus == string(model.TenantStatusSuspended) ||
					cached.TenantStatus == string(model.TenantStatusDeleted) {
					apierr.WriteError(w, r, apierr.ErrTenantSuspended)
					return
				}
				if fmt.Sprintf("%x", inputHash) != cached.KeyHash {
					apierr.WriteError(w, r, apierr.ErrUnauthorized)
					return
				}
				tid, err1 := uuid.Parse(cached.TenantID)
				if err1 != nil {
					apierr.WriteError(w, r, apierr.ErrUnauthorized)
					return
				}
				tenantID = tid
				// credentialID queda en uuid.Nil: basic_auth_credentials.id no referencia
				// api_credentials y no puede usarse como FK en usage_events.
			} else {
				cred, tenant, err := m.basicRepo.FindByUsername(ctx, username)
				if err != nil || cred == nil {
					apierr.WriteError(w, r, apierr.ErrUnauthorized)
					return
				}
				if cred.Status != model.CredentialStatusActive {
					apierr.WriteError(w, r, apierr.ErrUnauthorized)
					return
				}
				if tenant.Status == model.TenantStatusSuspended || tenant.Status == model.TenantStatusDeleted {
					apierr.WriteError(w, r, apierr.ErrTenantSuspended)
					return
				}
				if !bytes.Equal(cred.KeyHash, inputHash[:]) {
					apierr.WriteError(w, r, apierr.ErrUnauthorized)
					return
				}
				tenantID = tenant.ID
				// credentialID queda en uuid.Nil: basic_auth_credentials.id no referencia
				// api_credentials y no puede usarse como FK en usage_events.
				go func() {
					entry := &rediscache.CachedBasicAuth{
						CredID:       cred.ID.String(),
						TenantID:     tenant.ID.String(),
						TenantStatus: string(tenant.Status),
						KeyHash:      fmt.Sprintf("%x", cred.KeyHash),
						CredStatus:   string(cred.Status),
					}
					_ = m.cache.SetBasicAuth(context.Background(), username, entry, tspCacheTTL)
				}()
			}
		} else {
			// ── Flujo 2: Autenticación por IP (sin credenciales) ──
			// Busca el tenant cuya noauth_access esté activa
			// y cuya ip_allowlist contenga la IP del cliente.
			tenant, _, err := m.noauthRepo.FindTenantByIP(ctx, clientIP)
			if err != nil || tenant == nil {
				// IP no autorizada o tenant no tiene noauth_access habilitado.
				// Devolvemos 401 con WWW-Authenticate en lugar de 403, para que
				// clientes como Adobe Acrobat/JSignPdf puedan reintentar con
				// credenciales Basic Auth en la siguiente solicitud.
				w.Header().Set("WWW-Authenticate", `Basic realm="TSA Proxy"`)
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			if tenant.Status == model.TenantStatusSuspended || tenant.Status == model.TenantStatusDeleted {
				apierr.WriteError(w, r, apierr.ErrTenantSuspended)
				return
			}
			tenantID = tenant.ID
			credentialID = uuid.Nil // sin credencial específica en este flujo
		}

		// Inyectar en contexto
		ctx = context.WithValue(ctx, model.CtxKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, model.CtxKeyCredentialID, credentialID)
		ctx = context.WithValue(ctx, model.CtxKeyClientIP, clientIP)
		ctx = context.WithValue(ctx, model.CtxKeyBurstLimit, 60)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
