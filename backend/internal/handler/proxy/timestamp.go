package proxy

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/config"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
	proxysvc "github.com/bigdavi/tsa-proxy/internal/service/proxy"
)

const tokenCacheTTL = 5 * time.Minute

// TimestampHandler maneja POST /api/v1/timestamp y POST /ts/{urlToken}
type TimestampHandler struct {
	svc      *proxysvc.Service
	cfg      *config.Config
	credRepo *postgres.CredentialRepository
	cache    *rediscache.Cache
}

func NewTimestampHandler(svc *proxysvc.Service, cfg *config.Config, credRepo *postgres.CredentialRepository, cache *rediscache.Cache) *TimestampHandler {
	return &TimestampHandler{svc: svc, cfg: cfg, credRepo: credRepo, cache: cache}
}

// Probe responde a GET /ts — DAVISIGN, Adobe y JSignPdf hacen un GET
// para verificar que la TSA está activa antes de enviar el POST real.
func (h *TimestampHandler) Probe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"TSA Proxy","protocol":"RFC 3161"}`))
}

// Handle procesa un request RFC 3161.
func (h *TimestampHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rid := middleware.GetRequestID(ctx)

	ct := r.Header.Get("Content-Type")
	if ct != "application/timestamp-query" {
		apierr.WriteError(w, r, &apierr.APIError{
			StatusCode: http.StatusUnsupportedMediaType,
			Code:       "invalid_content_type",
			Message:    "Content-Type must be application/timestamp-query",
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.Proxy.MaxRequestBody+1))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if int64(len(body)) > h.cfg.Proxy.MaxRequestBody {
		apierr.WriteError(w, r, &apierr.APIError{
			StatusCode: http.StatusRequestEntityTooLarge,
			Code:       "request_too_large",
		})
		return
	}
	if len(body) == 0 {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	tenantID, ok := ctx.Value(model.CtxKeyTenantID).(uuid.UUID)
	if !ok {
		apierr.WriteError(w, r, apierr.ErrUnauthorized)
		return
	}
	credentialID, _ := ctx.Value(model.CtxKeyCredentialID).(uuid.UUID)
	clientIP, _ := ctx.Value(model.CtxKeyClientIP).(string)
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}

	req := &proxysvc.Request{
		Body:         body,
		TenantID:     tenantID,
		CredentialID: credentialID,
		SourceIP:     clientIP,
		RequestID:    rid,
		RequestSize:  len(body),
		UserAgent:    r.Header.Get("User-Agent"),
	}

	result, apiErr := h.svc.Process(ctx, req)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	w.Header().Set("Content-Type", "application/timestamp-reply")
	w.Header().Set("X-Request-ID", rid)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
}

// HandleByToken procesa POST /ts/{urlToken}.
func (h *TimestampHandler) HandleByToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rid := middleware.GetRequestID(ctx)

	token := chi.URLParam(r, "urlToken")
	if len(token) < 16 {
		apierr.WriteError(w, r, apierr.ErrUnauthorized)
		return
	}

	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	var tenantID uuid.UUID
	var credID uuid.UUID

	// Cache-first: intentar resolver desde Redis
	cached, cacheErr := h.cache.GetCredentialByToken(ctx, token)
	if cacheErr == nil && cached != nil {
		// Cache hit
		tid, err1 := uuid.Parse(cached.TenantID)
		cid, err2 := uuid.Parse(cached.CredID)
		if err1 != nil || err2 != nil {
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}
		if cached.TenantStatus == string(model.TenantStatusSuspended) || cached.TenantStatus == string(model.TenantStatusDeleted) {
			apierr.WriteError(w, r, apierr.ErrTenantSuspended)
			return
		}
		if cached.ExpiresAt != nil {
			expAt, err := time.Parse(time.RFC3339, *cached.ExpiresAt)
			if err == nil && expAt.Before(time.Now()) {
				apierr.WriteError(w, r, apierr.ErrUnauthorized)
				return
			}
		}
		tenantID = tid
		credID = cid
	} else {
		// Cache miss: ir a Postgres
		cred, tenant, err := h.credRepo.FindByURLToken(ctx, token)
		if err != nil || cred == nil {
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}
		if tenant.Status == model.TenantStatusSuspended || tenant.Status == model.TenantStatusDeleted {
			apierr.WriteError(w, r, apierr.ErrTenantSuspended)
			return
		}
		if cred.ExpiresAt != nil && cred.ExpiresAt.Before(time.Now()) {
			apierr.WriteError(w, r, apierr.ErrUnauthorized)
			return
		}
		tenantID = tenant.ID
		credID = cred.ID

		// Poblar cache
		entry := &rediscache.CachedTokenCredential{
			TenantID:     tenant.ID.String(),
			TenantStatus: string(tenant.Status),
			CredID:       cred.ID.String(),
		}
		if cred.ExpiresAt != nil {
			s := cred.ExpiresAt.Format(time.RFC3339)
			entry.ExpiresAt = &s
		}
		go func() {
			_ = h.cache.SetCredentialByToken(context.Background(), token, entry, tokenCacheTTL)
		}()
	}

	// Inyectar en contexto
	ctx = context.WithValue(ctx, model.CtxKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, model.CtxKeyCredentialID, credID)
	ctx = context.WithValue(ctx, model.CtxKeyBurstLimit, 60)
	ctx = context.WithValue(ctx, model.CtxKeyClientIP, clientIP)

	// Actualizar last_used (best-effort)
	go func() { h.credRepo.UpdateLastUsed(context.Background(), credID, clientIP) }()

	ct := r.Header.Get("Content-Type")
	if ct != "application/timestamp-query" {
		apierr.WriteError(w, r, &apierr.APIError{
			StatusCode: http.StatusUnsupportedMediaType,
			Code:       "invalid_content_type",
			Message:    "Content-Type must be application/timestamp-query",
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.Proxy.MaxRequestBody+1))
	if err != nil || int64(len(body)) > h.cfg.Proxy.MaxRequestBody || len(body) == 0 {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	req := &proxysvc.Request{
		Body:         body,
		TenantID:     tenantID,
		CredentialID: credID,
		SourceIP:     clientIP,
		RequestID:    rid,
		RequestSize:  len(body),
		UserAgent:    r.Header.Get("User-Agent"),
	}

	result, apiErr := h.svc.Process(ctx, req)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	w.Header().Set("Content-Type", "application/timestamp-reply")
	w.Header().Set("X-Request-ID", rid)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
}

// HandleTSP procesa POST /tsp.
// El TSPAuthMiddleware ya validó la identidad del cliente (por Basic Auth o por IP)
// e inyectó el tenantID en el contexto. Este handler solo procesa el timestamp.
func (h *TimestampHandler) HandleTSP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rid := middleware.GetRequestID(ctx)

	ct := r.Header.Get("Content-Type")
	if ct != "application/timestamp-query" {
		apierr.WriteError(w, r, &apierr.APIError{
			StatusCode: http.StatusUnsupportedMediaType,
			Code:       "invalid_content_type",
			Message:    "Content-Type must be application/timestamp-query",
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.Proxy.MaxRequestBody+1))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}
	if int64(len(body)) > h.cfg.Proxy.MaxRequestBody {
		apierr.WriteError(w, r, &apierr.APIError{
			StatusCode: http.StatusRequestEntityTooLarge,
			Code:       "request_too_large",
		})
		return
	}
	if len(body) == 0 {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	// El TSPAuthMiddleware garantiza que tenantID está en el contexto
	tenantID, ok := ctx.Value(model.CtxKeyTenantID).(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		apierr.WriteError(w, r, apierr.ErrUnauthorized)
		return
	}
	credentialID, _ := ctx.Value(model.CtxKeyCredentialID).(uuid.UUID)
	clientIP, _ := ctx.Value(model.CtxKeyClientIP).(string)
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}

	req := &proxysvc.Request{
		Body:         body,
		TenantID:     tenantID,
		CredentialID: credentialID,
		SourceIP:     clientIP,
		RequestID:    rid,
		RequestSize:  len(body),
		UserAgent:    r.Header.Get("User-Agent"),
	}

	result, apiErr := h.svc.Process(ctx, req)
	if apiErr != nil {
		apierr.WriteError(w, r, apiErr)
		return
	}

	w.Header().Set("Content-Type", "application/timestamp-reply")
	w.Header().Set("X-Request-ID", rid)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
}
