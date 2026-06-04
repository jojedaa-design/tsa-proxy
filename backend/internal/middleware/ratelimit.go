package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/config"
	"github.com/bigdavi/tsa-proxy/internal/model"
	redisrl "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

// RateLimitMiddleware aplica rate limiting global y por tenant.
type RateLimitMiddleware struct {
	rl  *redisrl.RateLimiter
	cfg *config.Config
}

func NewRateLimitMiddleware(rl *redisrl.RateLimiter, cfg *config.Config) *RateLimitMiddleware {
	return &RateLimitMiddleware{rl: rl, cfg: cfg}
}

// Global aplica un límite de RPS para todo el sistema.
// Usa una ventana deslizante de 1 segundo en Redis.
func (m *RateLimitMiddleware) Global(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, remaining, err := m.rl.CheckGlobal(r.Context(), m.cfg.RateLimit.GlobalRPS)
		if err != nil {
			// Si Redis falla, fail-open (no bloqueamos tráfico legítimo)
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit",     strconv.Itoa(m.cfg.RateLimit.GlobalRPS))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			w.Header().Set("Retry-After", "1")
			apierr.WriteErrorWithExtra(w, r, apierr.ErrRateLimited, map[string]interface{}{
				"retry_after": 1,
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// PerTenant aplica el burst limit configurado para cada tenant.
// Ventana de 60 segundos, límite definido en tenant_quotas.burst_per_minute.
func (m *RateLimitMiddleware) PerTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rid := GetRequestID(ctx)

		tenantID, ok := ctx.Value(model.CtxKeyTenantID).(uuid.UUID)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		burstLimit, _ := ctx.Value(model.CtxKeyBurstLimit).(int)
		if burstLimit <= 0 {
			burstLimit = 60 // default conservador
		}

		allowed, remaining, err := m.rl.CheckTenant(ctx, tenantID, burstLimit)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Tenant-Limit",     strconv.Itoa(burstLimit))
		w.Header().Set("X-RateLimit-Tenant-Remaining", strconv.Itoa(remaining))

		if !allowed {
			go func() {
				tid := tenantID
				_ = recordFailedFromCtx(context.Background(), m.rl.FailedRepo(), &tid, nil, "", "rate_limited", rid)
			}()
			w.Header().Set("Retry-After", "60")
			apierr.WriteErrorWithExtra(w, r, apierr.ErrRateLimited, map[string]interface{}{
				"retry_after": 60,
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func recordFailedFromCtx(
	ctx context.Context,
	repo interface{ Insert(context.Context, *model.FailedRequest) error },
	tenantID *uuid.UUID,
	keyID *string,
	ip, reason, requestID string,
) error {
	return repo.Insert(ctx, &model.FailedRequest{
		TenantID:        tenantID,
		CredentialKeyID: keyID,
		SourceIP:        ip,
		FailureReason:   reason,
		RequestID:       &requestID,
	})
}
