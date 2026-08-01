// Package proxy implementa el servicio de orquestación del timestamp proxy.
// Es el corazón del sistema: recibe el request del cliente, valida cuota,
// llama al upstream y registra el evento.
package proxy

import (
	"context"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/config"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
	"github.com/bigdavi/tsa-proxy/internal/service/geoloc"
	"github.com/bigdavi/tsa-proxy/internal/upstream"
)

// Request contiene todo lo necesario para procesar un timestamp.
type Request struct {
	Body         []byte
	TenantID     uuid.UUID
	CredentialID uuid.UUID
	SourceIP     string
	RequestID    string
	RequestSize  int
	UserAgent    string
}

// Result contiene el resultado de un timestamp procesado.
type Result struct {
	Body              []byte
	UpstreamStatus    int
	LatencyMs         int
	UpstreamLatencyMs int
}

// Service orquesta el flujo completo de un timestamp request.
type Service struct {
	cfg          *config.Config
	upstream     *upstream.Client
	quotaRepo    *postgres.QuotaRepository
	usageRepo    *postgres.UsageRepository
	rateLimiter  *rediscache.RateLimiter
	credRepo     *postgres.CredentialRepository
	upstreamRepo *postgres.UpstreamRepository
	geolocator   *geoloc.Locator
	cache        *rediscache.Cache
}

func NewService(
	cfg *config.Config,
	upstreamClient *upstream.Client,
	quotaRepo *postgres.QuotaRepository,
	usageRepo *postgres.UsageRepository,
	rl *rediscache.RateLimiter,
	credRepo *postgres.CredentialRepository,
	upstreamRepo *postgres.UpstreamRepository,
	geolocator *geoloc.Locator,
	cache *rediscache.Cache,
) *Service {
	return &Service{
		cfg:          cfg,
		upstream:     upstreamClient,
		quotaRepo:    quotaRepo,
		usageRepo:    usageRepo,
		rateLimiter:  rl,
		credRepo:     credRepo,
		upstreamRepo: upstreamRepo,
		geolocator:   geolocator,
		cache:        cache,
	}
}

// Process ejecuta el flujo completo:
// 1. Verificar y decrementar cuota
// 2. Llamar al upstream TSA
// 3. Registrar el evento de uso (async)
// 4. Devolver la respuesta
func (s *Service) Process(ctx context.Context, req *Request) (*Result, *apierr.APIError) {
	start := time.Now()
	now := time.Now()

	// ── 1. Validar bolsa de sellos ────────────────────────────
	// Modelo de bolsa: contratado = SUM(quota_bundles), consumido = SUM(monthly_usage_aggregates)
	// Si consumido >= contratado → rechaza (sin excepciones)
	var quota *model.TenantQuota
	if s.cache != nil {
		quota, _ = s.cache.GetTenantQuota(ctx, req.TenantID)
	}
	if quota == nil {
		var dbErr error
		quota, dbErr = s.quotaRepo.GetByTenant(ctx, req.TenantID)
		if dbErr != nil {
			log.Error().Err(dbErr).Str("tenant_id", req.TenantID.String()).Msg("quota lookup error")
			return nil, apierr.ErrInternal
		}
		if quota != nil && s.cache != nil {
			go func(q *model.TenantQuota) {
				_ = s.cache.SetTenantQuota(context.Background(), req.TenantID, q, 60*time.Second)
			}(quota)
		}
	}

	if quota != nil {
		// Obtener totales: contratado (bolsas) y consumido (all-time)
		contracted, _ := s.quotaRepo.GetTotalContracted(ctx, req.TenantID)
		consumed, _ := s.quotaRepo.GetTotalConsumption(ctx, req.TenantID)

		if consumed >= contracted && contracted > 0 {
			// Bolsa agotada: rechazar
			reason := "quota_exceeded"
			reqSize := req.RequestSize
			go s.recordUsage(context.Background(), req, model.UsageStatusRejected, &reason,
				nil, nil, nil, &reqSize, nil, false)
			return nil, apierr.ErrQuotaExceeded
		}
	}

	// ── 2. Obtener upstream default (cache-first, TTL 30s) ─────
	var tsaUpstream *model.TSAUpstream
	if s.cache != nil {
		tsaUpstream, _ = s.cache.GetDefaultUpstream(ctx)
	}
	if tsaUpstream == nil {
		var dbErr error
		tsaUpstream, dbErr = s.upstreamRepo.GetDefault(ctx)
		if dbErr != nil || tsaUpstream == nil {
			// Fallback a configuración del .env si no hay upstream en BD
			if s.cfg.Upstream.URL == "" {
				log.Error().Msg("no upstream TSA configured in database or .env")
				return nil, apierr.ErrUpstreamUnavailable
			}
			tsaUpstream = &model.TSAUpstream{
				URL:        s.cfg.Upstream.URL,
				TimeoutMs:  int(s.cfg.Upstream.Timeout.Milliseconds()),
				MaxRetries: s.cfg.Upstream.MaxRetries,
			}
		} else if s.cache != nil {
			go func(u *model.TSAUpstream) {
				_ = s.cache.SetDefaultUpstream(context.Background(), u, 30*time.Second)
			}(tsaUpstream)
		}
	}

	// Resolver credenciales: primero las columnas directas, luego las de env vars (legacy)
	user, pass := resolveUpstreamCredentials(tsaUpstream, s.cfg)

	sendParams := upstream.SendParams{
		URL:        tsaUpstream.URL,
		Username:   user,
		Password:   pass,
		MaxRetries: tsaUpstream.MaxRetries,
		MaxBody:    s.cfg.Upstream.MaxBodyBytes,
		Timeout:    time.Duration(tsaUpstream.TimeoutMs) * time.Millisecond,
	}

	// ── 3. Llamar al upstream TSA ──────────────────────────────
	upstreamStart := time.Now()
	upstreamResp, upstreamErr := s.upstream.Send(ctx, req.Body, sendParams)
	upstreamLatencyMs := int(time.Since(upstreamStart).Milliseconds())
	totalLatencyMs := int(time.Since(start).Milliseconds())

	// ── 3. Registrar evento (async para no bloquear respuesta) ──
	var usageStatus model.UsageStatus
	var httpStatus *int
	var rejectionReason *string
	var responseSize *int

	if upstreamErr != nil || upstreamResp == nil || upstreamResp.HTTPStatus >= 500 {
		usageStatus = model.UsageStatusError
		reason := "upstream_error"
		rejectionReason = &reason
	} else {
		usageStatus = model.UsageStatusSuccess
		status := upstreamResp.HTTPStatus
		httpStatus = &status
		size := len(upstreamResp.Body)
		responseSize = &size
	}

	// Con el modelo de bolsa, no hay exceso: solo rechaza o emite.
	exceedsQuota := false

	reqSize := req.RequestSize
	if s.cfg.Proxy.UsageRecordAsync {
		go s.recordUsage(context.Background(), req, usageStatus, rejectionReason,
			httpStatus, &totalLatencyMs, &upstreamLatencyMs,
			&reqSize, responseSize, exceedsQuota)
	} else {
		s.recordUsage(ctx, req, usageStatus, rejectionReason,
			httpStatus, &totalLatencyMs, &upstreamLatencyMs,
			&reqSize, responseSize, exceedsQuota)
	}

	if upstreamErr != nil {
		log.Error().
			Err(upstreamErr).
			Str("request_id", req.RequestID).
			Str("tenant_id", req.TenantID.String()).
			Msg("upstream error")
		return nil, apierr.ErrUpstreamUnavailable
	}

	if upstreamResp.HTTPStatus >= 500 {
		return nil, apierr.ErrUpstreamUnavailable
	}

	return &Result{
		Body:              upstreamResp.Body,
		UpstreamStatus:    upstreamResp.HTTPStatus,
		LatencyMs:         totalLatencyMs,
		UpstreamLatencyMs: upstreamLatencyMs,
	}, nil
}

func (s *Service) recordUsage(
	ctx context.Context,
	req *Request,
	status model.UsageStatus,
	rejectionReason *string,
	upstreamStatus *int,
	latencyMs *int,
	upstreamLatencyMs *int,
	requestSize *int,
	responseSize *int,
	exceedsQuota bool,
) {
	now := time.Now()
	credID := req.CredentialID

	// Capturar User-Agent
	userAgent := req.UserAgent
	if userAgent == "" {
		userAgent = "unknown"
	}

	// Hacer lookup de geolocalización
	var geoInfo *geoloc.GeoInfo
	if s.geolocator != nil {
		// Extraer IP sin puerto
		ip := req.SourceIP
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		geoInfo = s.geolocator.Lookup(ip)
	}

	// Para Basic Auth (/ts) y TSP sin credencial (/tsp sin Authorization),
	// credID será uuid.Nil. En esos casos, no registrar credential_id (NULL).
	var credIDPtr *uuid.UUID
	if credID != uuid.Nil {
		credIDPtr = &credID
	}

	event := &model.UsageEvent{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		CredentialID:      credIDPtr,
		RequestID:         req.RequestID,
		SourceIP:          req.SourceIP,
		Status:            status,
		RejectionReason:   rejectionReason,
		UpstreamStatus:    upstreamStatus,
		LatencyMs:         latencyMs,
		UpstreamLatencyMs: upstreamLatencyMs,
		RequestSizeBytes:  requestSize,
		ResponseSizeBytes: responseSize,
		UserAgent:         &userAgent,
		ExceedsQuota:      exceedsQuota,
		OccurredAt:        now,
	}

	// Agregar geolocalización si está disponible
	if geoInfo != nil {
		if geoInfo.Country != "" {
			event.GeoCountry = &geoInfo.Country
		}
		if geoInfo.City != "" {
			event.GeoCity = &geoInfo.City
		}
		if geoInfo.ASN != "" {
			event.GeoASN = &geoInfo.ASN
		}
	}

	if err := s.usageRepo.InsertEvent(ctx, event); err != nil {
		log.Error().Err(err).Str("request_id", req.RequestID).Msg("failed to record usage event")
	}

	// Actualizar agregado mensual
	year, month, _ := now.Date()
	lat := 0
	if latencyMs != nil {
		lat = *latencyMs
	}
	if err := s.quotaRepo.UpsertMonthlyAggregate(ctx, req.TenantID, year, int(month), status, lat); err != nil {
		log.Error().Err(err).Str("tenant_id", req.TenantID.String()).Msg("failed to upsert monthly aggregate")
	}

	// Actualizar last_used en credencial
	s.credRepo.UpdateLastUsed(ctx, req.CredentialID, req.SourceIP)
}

// resolveUpstreamCredentials obtiene usuario y contraseña para el upstream.
// Prioriza las columnas directas (username/password) sobre las variables de entorno (legacy).
func resolveUpstreamCredentials(u *model.TSAUpstream, cfg *config.Config) (user, pass string) {
	// 1. Credenciales directas en la BD (nuevo esquema)
	if u.Username != nil && *u.Username != "" {
		user = *u.Username
		if u.Password != nil {
			pass = *u.Password
		}
		return
	}
	// 2. Variables de entorno legacy (env_key_user / env_key_pass)
	if u.EnvKeyUser != nil || u.EnvKeyPass != nil {
		return upstream.ResolveCredentials(u.EnvKeyUser, u.EnvKeyPass)
	}
	// 3. Fallback al .env del servicio (TSA_UPSTREAM_USERNAME / TSA_UPSTREAM_PASSWORD)
	if cfg != nil {
		user = cfg.Upstream.Username
		pass = cfg.Upstream.Password
	}
	return
}

// secondsUntilEndOfMonth calcula los segundos hasta fin del mes actual + buffer.
func secondsUntilEndOfMonth(now time.Time) int {
	y, m, _ := now.Date()
	loc := now.Location()
	endOfMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, loc)
	secs := int(endOfMonth.Sub(now).Seconds()) + 300 // buffer 5 min
	if secs < 300 {
		secs = 300
	}
	return secs
}
