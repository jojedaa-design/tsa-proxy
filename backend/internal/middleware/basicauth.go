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
	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

const basicAuthCacheTTL = 5 * time.Minute

type BasicAuthMiddleware struct {
	repo  *postgres.BasicAuthRepository
	cache *rediscache.Cache
}

func NewBasicAuthMiddleware(repo *postgres.BasicAuthRepository, cache *rediscache.Cache) *BasicAuthMiddleware {
	return &BasicAuthMiddleware{repo: repo, cache: cache}
}

// Authenticate verifica las credenciales HTTP Basic Auth y carga el tenant en el contexto.
func (m *BasicAuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extraer credenciales Basic Auth
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Basic ") {
			log.Warn().Bool("header_empty", authHeader == "").Msg("basicauth: missing or non-Basic Authorization header")
			w.Header().Set("WWW-Authenticate", `Basic realm="TSA Proxy"`)
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			log.Warn().Err(err).Msg("basicauth: base64 decode failed")
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			log.Warn().Msg("basicauth: malformed credentials (no colon)")
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}

		username := parts[0]
		password := parts[1]

		// SHA-256 de la contraseña ingresada
		inputHash := sha256.Sum256([]byte(password))

		ctx := r.Context()
		clientIP := r.Header.Get("X-Real-IP")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		// Cache-first lookup
		var tenantID interface{}

		cached, cacheErr := m.cache.GetBasicAuth(ctx, username)
		if cacheErr == nil && cached != nil {
			// Verificar estado del cache
			if cached.CredStatus != "active" {
				log.Warn().Str("username", username).Str("status", cached.CredStatus).Msg("basicauth: credential not active (cache)")
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			if cached.TenantStatus == string(model.TenantStatusSuspended) || cached.TenantStatus == string(model.TenantStatusDeleted) {
				log.Warn().Str("username", username).Str("tenant_status", cached.TenantStatus).Msg("basicauth: tenant suspended/deleted (cache)")
				apierr.WriteError(w, r, apierr.ErrTenantSuspended)
				return
			}
			// Comparar hash
			expectedHashHex := cached.KeyHash
			inputHashHex := fmt.Sprintf("%x", inputHash)
			if expectedHashHex != inputHashHex {
				log.Warn().Str("username", username).Int("password_len", len(password)).Msg("basicauth: password mismatch (cache)")
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			// Parsear UUIDs
			tid, err1 := uuid.Parse(cached.TenantID)
			if err1 != nil {
				log.Warn().Str("username", username).Err(err1).Msg("basicauth: invalid tenant id (cache)")
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			tenantID = tid
		} else {
			// Cache miss: buscar en Postgres
			cred, tenant, err := m.repo.FindByUsername(ctx, username)
			if err != nil || cred == nil {
				log.Warn().Str("username", username).Err(err).Bool("not_found", cred == nil).Msg("basicauth: credential lookup failed")
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			if cred.Status != model.CredentialStatusActive {
				log.Warn().Str("username", username).Str("status", string(cred.Status)).Msg("basicauth: credential not active (db)")
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			if tenant.Status == model.TenantStatusSuspended || tenant.Status == model.TenantStatusDeleted {
				log.Warn().Str("username", username).Str("tenant_status", string(tenant.Status)).Msg("basicauth: tenant suspended/deleted (db)")
				apierr.WriteError(w, r, apierr.ErrTenantSuspended)
				return
			}
			// Comparar hash
			if !bytes.Equal(cred.KeyHash, inputHash[:]) {
				log.Warn().Str("username", username).Int("password_len", len(password)).Msg("basicauth: password mismatch (db)")
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
			tenantID = tenant.ID
			// Poblar cache
			go func() {
				entry := &rediscache.CachedBasicAuth{
					CredID:       cred.ID.String(),
					TenantID:     tenant.ID.String(),
					TenantStatus: string(tenant.Status),
					KeyHash:      fmt.Sprintf("%x", cred.KeyHash),
					CredStatus:   string(cred.Status),
				}
				_ = m.cache.SetBasicAuth(context.Background(), username, entry, basicAuthCacheTTL)
			}()
		}

		// Inyectar en contexto
		// Para Basic Auth, no hay credencial de API, por lo que usamos uuid.Nil
		ctx = context.WithValue(ctx, model.CtxKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, model.CtxKeyCredentialID, uuid.Nil)
		ctx = context.WithValue(ctx, model.CtxKeyClientIP, clientIP)
		ctx = context.WithValue(ctx, model.CtxKeyBurstLimit, 60)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
