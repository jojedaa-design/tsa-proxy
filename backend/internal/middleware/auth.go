package middleware

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

// AuthMiddleware valida API keys de clientes en el proxy público.
type AuthMiddleware struct {
	credRepo   *postgres.CredentialRepository
	cache      *rediscache.Cache
	failedRepo *postgres.FailedRequestRepository
}

func NewAuthMiddleware(
	credRepo *postgres.CredentialRepository,
	cache *rediscache.Cache,
	failedRepo *postgres.FailedRequestRepository,
) *AuthMiddleware {
	return &AuthMiddleware{
		credRepo:   credRepo,
		cache:      cache,
		failedRepo: failedRepo,
	}
}

// Authenticate extrae y valida la API key.
//
// Modos soportados (en orden de prioridad):
//  1. Query param:  ?apikey=<key>        — para software de firma que no maneja auth headers
//  2. HTTP Basic:   contraseña = api_key  — estándar, usuario puede ser cualquier valor
//  3. HTTP Basic:   usuario   = api_key  — fallback para clientes Java que invierten los campos
//  4. Bearer token: Authorization: Bearer <key>
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := GetRequestID(r.Context())
		clientIP := r.Header.Get("X-Real-IP")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		var apiKey string

		// 1. Query param ?apikey=... (útil para software de firma con bugs en auth headers)
		if k := r.URL.Query().Get("apikey"); len(k) >= 10 {
			apiKey = k
		}

		// 2 & 3. HTTP Basic Auth
		if apiKey == "" {
			if u, p, ok := r.BasicAuth(); ok {
				if len(p) >= 10 {
					apiKey = p
				} else if len(u) >= 10 {
					apiKey = u
				}
			}
		}

		// 4. Bearer token
		if apiKey == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if len(apiKey) < 10 {
			m.recordFailed(r.Context(), nil, nil, clientIP, "invalid_key", rid)
			w.Header().Set("WWW-Authenticate", `Basic realm="TSA Proxy"`)
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}

		// Hash SHA-256 de la key para lookup
		hash := sha256.Sum256([]byte(apiKey))
		hashBytes := hash[:]

		// Intentar cache Redis primero
		cached, err := m.cache.GetCredential(r.Context(), hashBytes)
		if err != nil {
			// Cache miss o error — consultar Postgres
			cred, tenant, err := m.credRepo.FindByHash(r.Context(), hashBytes)
			if err != nil || cred == nil {
				m.recordFailed(r.Context(), nil, nil, clientIP, "invalid_key", rid)
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}

			// Verificar expiración
			if cred.ExpiresAt != nil && cred.ExpiresAt.Before(time.Now()) {
				keyID := cred.KeyID
				m.recordFailed(r.Context(), nil, &keyID, clientIP, "credential_expired", rid)
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}

			cached = &model.CachedCredential{
				CredentialID:   cred.ID,
				TenantID:       tenant.ID,
				TenantStatus:   tenant.Status,
				BurstPerMinute: 60,
			}

			// Buscar quota para el burst limit
			quota, qErr := m.credRepo.GetTenantBurstLimit(r.Context(), tenant.ID)
			if qErr == nil && quota > 0 {
				cached.BurstPerMinute = quota
			}

			// Guardar en cache 60 segundos
			_ = m.cache.SetCredential(r.Context(), hashBytes, cached, 60*time.Second)

			log.Debug().
				Str("request_id", rid).
				Str("tenant_id", tenant.ID.String()).
				Msg("credential validated from db")
		}

		// Verificar estado del tenant
		if cached.TenantStatus == model.TenantStatusSuspended {
			m.recordFailed(r.Context(), &cached.TenantID, nil, clientIP, "tenant_suspended", rid)
			apierr.WriteError(w, r, apierr.ErrTenantSuspended)
			return
		}
		if cached.TenantStatus == model.TenantStatusDeleted {
			m.recordFailed(r.Context(), nil, nil, clientIP, "invalid_key", rid)
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}

		// Inyectar en contexto
		ctx := r.Context()
		ctx = context.WithValue(ctx, model.CtxKeyTenantID, cached.TenantID)
		ctx = context.WithValue(ctx, model.CtxKeyCredentialID, cached.CredentialID)
		ctx = context.WithValue(ctx, model.CtxKeyBurstLimit, cached.BurstPerMinute)
		ctx = context.WithValue(ctx, model.CtxKeyClientIP, clientIP)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) recordFailed(
	ctx context.Context,
	tenantID *uuid.UUID,
	keyID *string,
	ip, reason, requestID string,
) {
	go func() {
		_ = m.failedRepo.Insert(context.Background(), &model.FailedRequest{
			TenantID:        tenantID,
			CredentialKeyID: keyID,
			SourceIP:        ip,
			FailureReason:   reason,
			RequestID:       &requestID,
		})
	}()
}
