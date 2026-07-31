package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/config"
	healthhandler "github.com/bigdavi/tsa-proxy/internal/handler/health"
	adminhandler "github.com/bigdavi/tsa-proxy/internal/handler/admin"
	proxyhandler "github.com/bigdavi/tsa-proxy/internal/handler/proxy"
	"github.com/bigdavi/tsa-proxy/internal/jobs"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
	"github.com/bigdavi/tsa-proxy/internal/server"
	authsvc "github.com/bigdavi/tsa-proxy/internal/service/auth"
	basicauthsvc "github.com/bigdavi/tsa-proxy/internal/service/basicauth"
	credsvc "github.com/bigdavi/tsa-proxy/internal/service/credential"
	"github.com/bigdavi/tsa-proxy/internal/service/geoloc"
	proxysvc "github.com/bigdavi/tsa-proxy/internal/service/proxy"
	tenantsvc "github.com/bigdavi/tsa-proxy/internal/service/tenant"
	"github.com/bigdavi/tsa-proxy/internal/upstream"
)

func main() {
	// ── Logging ──────────────────────────────────────────────────
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.With().Caller().Logger()

	// ── Config ───────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	if cfg.Environment == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Info().
		Str("version", cfg.Version).
		Str("environment", cfg.Environment).
		Msg("starting tsa-proxy")

	// ── Postgres ─────────────────────────────────────────────────
	pgPool, err := postgres.NewPool(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pgPool.Close()

	// ── Redis ─────────────────────────────────────────────────────
	redisClient, err := rediscache.NewClient(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisClient.Close()

	// ── Repositories ──────────────────────────────────────────────
	adminUserRepo  := postgres.NewAdminUserRepository(pgPool)
	tenantRepo     := postgres.NewTenantRepository(pgPool)
	credRepo       := postgres.NewCredentialRepository(pgPool)
	ipAllowRepo    := postgres.NewIPAllowlistRepository(pgPool)
	quotaRepo      := postgres.NewQuotaRepository(pgPool)
	usageRepo      := postgres.NewUsageRepository(pgPool)
	auditRepo      := postgres.NewAuditRepository(pgPool)
	failedRepo     := postgres.NewFailedRequestRepository(pgPool)
	upstreamRepo   := postgres.NewUpstreamRepository(pgPool)
	basicAuthRepo  := postgres.NewBasicAuthRepository(pgPool)
	noauthRepo     := postgres.NewNoAuthRepository(pgPool)

	cache      := rediscache.NewCache(redisClient)
	rateLimiter := rediscache.NewRateLimiter(redisClient).WithFailedRepo(failedRepo)

	// ── Upstream client ───────────────────────────────────────────
	tsaClient := upstream.NewClient(cfg.Upstream.Timeout)

	// ── Geolocation (opcional) ────────────────────────────────────
	dbPath := "/opt/tsa-proxy/data/GeoLite2-City.mmdb" // Ruta por defecto
	geolocator, err := geoloc.New(dbPath)
	if err != nil {
		log.Warn().Err(err).Msg("geolocation database not available")
		geolocator = nil
	}
	if geolocator != nil {
		defer geolocator.Close()
	}

	// ── Services ──────────────────────────────────────────────────
	authService   := authsvc.NewService(cfg, adminUserRepo, redisClient)
	tenantService := tenantsvc.NewService(tenantRepo, cache, auditRepo)
	credService      := credsvc.NewService(credRepo, cache)
	basicAuthService := basicauthsvc.NewService(basicAuthRepo, cache)
	proxyService  := proxysvc.NewService(cfg, tsaClient, quotaRepo, usageRepo, rateLimiter, credRepo, upstreamRepo, geolocator, cache)

	// ── Middlewares ───────────────────────────────────────────────
	authMW      := middleware.NewAuthMiddleware(credRepo, cache, failedRepo)
	adminAuthMW := middleware.NewAdminAuthMiddleware(cfg)
	rateLimitMW := middleware.NewRateLimitMiddleware(rateLimiter, cfg)
	ipAllowMW   := middleware.NewIPAllowlistMiddleware(ipAllowRepo, cache, failedRepo)
	basicAuthMW := middleware.NewBasicAuthMiddleware(basicAuthRepo, cache)
	tspAuthMW   := middleware.NewTSPAuthMiddleware(basicAuthRepo, noauthRepo, cache)

	// ── Handlers ─────────────────────────────────────────────────
	healthH    := healthhandler.NewHandler(cfg, pgPool, redisClient)
	timestampH := proxyhandler.NewTimestampHandler(proxyService, cfg, credRepo, cache)

	adminAuthH    := adminhandler.NewAuthHandler(authService, adminUserRepo)
	adminTenantsH := adminhandler.NewTenantsHandler(tenantService, quotaRepo)
	adminCredsH     := adminhandler.NewCredentialsHandler(credService)
	adminBasicAuthH := adminhandler.NewBasicAuthHandler(basicAuthService)
	adminNoAuthH    := adminhandler.NewNoAuthHandler(noauthRepo)
	adminIPH      := adminhandler.NewIPAllowlistHandler(ipAllowRepo, cache, auditRepo)
	adminQuotasH  := adminhandler.NewQuotasHandler(quotaRepo, auditRepo, cache)
	adminReportsH := adminhandler.NewReportsHandler(usageRepo, failedRepo, adminUserRepo)
	adminAuditH   := adminhandler.NewAuditHandler(auditRepo, adminUserRepo)
	adminConfigH  := adminhandler.NewConfigHandler(upstreamRepo, quotaRepo, cache, cfg.Upstream.AllowedHosts)
	adminUsersH   := adminhandler.NewUsersHandler(adminUserRepo, tenantRepo)

	// ── Router ────────────────────────────────────────────────────
	router := server.NewRouter(&server.Deps{
		Cfg:              cfg,
		HealthHandler:    healthH,
		TimestampHandler: timestampH,
		AdminAuth:        adminAuthH,
		AdminTenants:     adminTenantsH,
		AdminCredentials: adminCredsH,
		AdminIPAllowlist: adminIPH,
		AdminQuotas:      adminQuotasH,
		AdminReports:     adminReportsH,
		AdminAudit:       adminAuditH,
		AdminConfig:      adminConfigH,
		AdminUsers:       adminUsersH,
		AuthMW:           authMW,
		AdminAuthMW:      adminAuthMW,
		RateLimitMW:      rateLimitMW,
		IPAllowMW:        ipAllowMW,
		BasicAuthMW:      basicAuthMW,
		TSPAuthMW:        tspAuthMW,
		AdminBasicAuth:   adminBasicAuthH,
		AdminNoAuth:      adminNoAuthH,
	})

	// ── HTTP Server ───────────────────────────────────────────────
	srv := server.New(cfg, router)

	// ── Background jobs ───────────────────────────────────────────
	cleanupJob   := jobs.NewCleanupJob(pgPool)
	partitionJob := jobs.NewPartitionJob(pgPool)

	go runScheduler(cleanupJob, partitionJob)

	// ── Graceful shutdown ─────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-quit
	log.Info().Msg("shutdown signal received")
	srv.Shutdown()
}

// runScheduler ejecuta jobs periódicos.
func runScheduler(cleanup *jobs.CleanupJob, partition *jobs.PartitionJob) {
	// Ejecutar partitionJob al arrancar para asegurar particiones
	ctx := context.Background()
	partition.EnsureNextMonth(ctx)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()

		// Cleanup diario a las 03:00 UTC
		if now.Hour() == 3 && now.Minute() < 5 {
			go cleanup.Run(ctx)
		}

		// Crear partición del mes siguiente el día 25
		if now.Day() == 25 && now.Hour() == 12 {
			go partition.EnsureNextMonth(ctx)
		}
	}
}
