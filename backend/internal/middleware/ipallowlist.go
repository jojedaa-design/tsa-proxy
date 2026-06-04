package middleware

import (
	"context"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

// IPAllowlistMiddleware valida que la IP del cliente esté en la allowlist del tenant.
type IPAllowlistMiddleware struct {
	ipRepo     *postgres.IPAllowlistRepository
	cache      *rediscache.Cache
	failedRepo *postgres.FailedRequestRepository
}

func NewIPAllowlistMiddleware(
	ipRepo *postgres.IPAllowlistRepository,
	cache *rediscache.Cache,
	failedRepo *postgres.FailedRequestRepository,
) *IPAllowlistMiddleware {
	return &IPAllowlistMiddleware{
		ipRepo:     ipRepo,
		cache:      cache,
		failedRepo: failedRepo,
	}
}

// Check verifica si la IP del request está permitida para el tenant.
// Si el tenant no tiene ninguna IP configurada, se permite el acceso (política abierta).
func (m *IPAllowlistMiddleware) Check(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rid := GetRequestID(ctx)

		tenantID, ok := ctx.Value(model.CtxKeyTenantID).(uuid.UUID)
		if !ok {
			// Si no hay tenant en contexto (no debería pasar), dejar pasar
			next.ServeHTTP(w, r)
			return
		}

		clientIP := ctx.Value(model.CtxKeyClientIP).(string)
		if clientIP == "" {
			clientIP = r.Header.Get("X-Real-IP")
		}

		// Parsear IP del cliente
		ip := net.ParseIP(clientIP)
		if ip == nil {
			log.Warn().Str("request_id", rid).Str("raw_ip", clientIP).Msg("could not parse client IP")
			apierr.WriteError(w, r, apierr.ErrIPNotAllowed)
			return
		}

		// Obtener CIDRs del tenant (con cache)
		cidrs, err := m.getCIDRs(ctx, tenantID)
		if err != nil {
			log.Error().Err(err).Str("request_id", rid).Msg("error fetching ip allowlist")
			// En caso de error de BD, permitir (fail-open) para no bloquear tráfico legítimo
			next.ServeHTTP(w, r)
			return
		}

		// Sin restricciones configuradas → permitir todo
		if len(cidrs) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Verificar si la IP está en alguno de los CIDRs
		for _, cidrStr := range cidrs {
			_, network, err := net.ParseCIDR(cidrStr)
			if err != nil {
				continue
			}
			if network.Contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// IP no permitida
		log.Warn().
			Str("request_id", rid).
			Str("tenant_id", tenantID.String()).
			Str("client_ip", clientIP).
			Msg("ip not in allowlist")

		go func() {
			tid := tenantID
			_ = m.failedRepo.Insert(context.Background(), &model.FailedRequest{
				TenantID:      &tid,
				SourceIP:      clientIP,
				FailureReason: "ip_not_allowed",
				RequestID:     &rid,
			})
		}()

		apierr.WriteError(w, r, apierr.ErrIPNotAllowed)
	})
}

func (m *IPAllowlistMiddleware) getCIDRs(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	// Intentar cache Redis
	cidrs, err := m.cache.GetIPAllowlist(ctx, tenantID)
	if err == nil {
		return cidrs, nil
	}

	// Cache miss → consultar Postgres
	entries, err := m.ipRepo.FindByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	cidrs = make([]string, len(entries))
	for i, e := range entries {
		cidrs[i] = e.CIDR
	}

	// Guardar en cache 60 segundos
	_ = m.cache.SetIPAllowlist(ctx, tenantID, cidrs, 60)

	return cidrs, nil
}
