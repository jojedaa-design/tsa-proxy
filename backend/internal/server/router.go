package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/bigdavi/tsa-proxy/internal/config"
	healthhandler "github.com/bigdavi/tsa-proxy/internal/handler/health"
	adminhandler "github.com/bigdavi/tsa-proxy/internal/handler/admin"
	proxyhandler "github.com/bigdavi/tsa-proxy/internal/handler/proxy"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
)

// Deps agrupa todas las dependencias necesarias para construir el router.
type Deps struct {
	Cfg              *config.Config
	HealthHandler    *healthhandler.Handler
	TimestampHandler *proxyhandler.TimestampHandler
	AdminAuth        *adminhandler.AuthHandler
	AdminTenants     *adminhandler.TenantsHandler
	AdminCredentials *adminhandler.CredentialsHandler
	AdminIPAllowlist *adminhandler.IPAllowlistHandler
	AdminQuotas      *adminhandler.QuotasHandler
	AdminReports     *adminhandler.ReportsHandler
	AdminAudit       *adminhandler.AuditHandler
	AdminConfig      *adminhandler.ConfigHandler
	AdminUsers       *adminhandler.UsersHandler
	// Middlewares
	AuthMW      *middleware.AuthMiddleware
	AdminAuthMW *middleware.AdminAuthMiddleware
	RateLimitMW *middleware.RateLimitMiddleware
	IPAllowMW   *middleware.IPAllowlistMiddleware
	BasicAuthMW *middleware.BasicAuthMiddleware
	TSPAuthMW   *middleware.TSPAuthMiddleware
	// Admin handlers adicionales
	AdminBasicAuth *adminhandler.BasicAuthHandler
	AdminNoAuth    *adminhandler.NoAuthHandler
	AdminAlerts    *adminhandler.AlertsHandler
}

// NewRouter construye el árbol de rutas completo.
func NewRouter(d *Deps) http.Handler {
	r := chi.NewRouter()

	// ── Middlewares globales ─────────────────────────────────────
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CORS(d.Cfg))
	r.Use(chimiddleware.RealIP) // extrae IP de X-Forwarded-For (puesto por Nginx)

	// ── Health (sin auth) ────────────────────────────────────────
	r.Get("/health", d.HealthHandler.Health)
	r.Get("/ready",  d.HealthHandler.Ready)

	// ── Proxy público (/api/v1/timestamp — auth por header/query param) ────
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(d.RateLimitMW.Global)
		r.Use(d.AuthMW.Authenticate)
		r.Use(d.IPAllowMW.Check)
		r.Use(d.RateLimitMW.PerTenant)
		r.Post("/timestamp", d.TimestampHandler.Handle)
	})

	// ── /ts y /tsp — Endpoints unificados RFC 3161 ──────────────
	//
	// Ambos endpoints soportan los dos modos de autenticación:
	//
	//   • TSA PRIVADA (con credenciales Basic Auth):
	//     - Compatible con Adobe Acrobat, JSignPdf, DAVISIGN (preemptive)
	//     - Usuario/contraseña en basic_auth_credentials
	//     - Se aplica IP allowlist del tenant (si está configurada)
	//
	//   • TSA PÚBLICA (sin credenciales, por IP):
	//     - Compatible con DAVISIGN, EU DSS y clientes que no envían auth
	//     - IP autorizada en noauth_access + tenant_ip_allowlist
	//     - Requiere que el tenant tenga noauth_access activo
	//
	// En AMBOS modos se aplican:
	//   - IP allowlist del tenant
	//   - Rate limiting global y por tenant
	//   - Cuota mensual (configurable por tenant)
	//
	// Si no hay credenciales y la IP no está autorizada, se devuelve
	// 401 con WWW-Authenticate para que el cliente reintente con Basic Auth.
	unifiedChain := chi.Chain(
		d.RateLimitMW.Global,
		d.TSPAuthMW.Authenticate,
		d.IPAllowMW.Check,
		d.RateLimitMW.PerTenant,
	)

	// /ts — endpoint tradicional RFC 3161
	r.With(unifiedChain...).Get("/ts", d.TimestampHandler.Probe)
	r.With(unifiedChain...).Get("/ts/", d.TimestampHandler.Probe)
	r.With(unifiedChain...).Post("/ts", d.TimestampHandler.HandleTSP)
	r.With(unifiedChain...).Post("/ts/", d.TimestampHandler.HandleTSP)

	// POST /ts/{urlToken} — token embebido en URL, sin usuario ni contraseña
	r.With(d.RateLimitMW.Global).Post("/ts/{urlToken}", d.TimestampHandler.HandleByToken)

	// /tsp — alias del endpoint /ts (mismo comportamiento)
	// Mantenido para compatibilidad con clientes que fueron configurados con /tsp
	r.With(unifiedChain...).Get("/tsp", d.TimestampHandler.Probe)
	r.With(unifiedChain...).Get("/tsp/", d.TimestampHandler.Probe)
	r.With(unifiedChain...).Post("/tsp", d.TimestampHandler.HandleTSP)
	r.With(unifiedChain...).Post("/tsp/", d.TimestampHandler.HandleTSP)

	// ── Admin API ────────────────────────────────────────────────
	r.Route("/api/admin/v1", func(r chi.Router) {

		// Auth endpoints (sin JWT requerido)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login",             d.AdminAuth.Login)
			r.Post("/logout",            d.AdminAuth.Logout)
			r.Post("/refresh",           d.AdminAuth.Refresh)
			r.Post("/2fa/verify",        d.AdminAuth.VerifyTOTP)
			// 2FA obligatorio: flujo de setup forzado en el login (sin JWT — el
			// usuario todavía no tiene sesión, se identifica por setup_token).
			r.Post("/2fa/setup-required",  d.AdminAuth.SetupTOTPForLogin)
			r.Post("/2fa/complete-setup",  d.AdminAuth.CompleteTOTPSetup)
			r.With(d.AdminAuthMW.Require).Get("/me",          d.AdminAuth.Me)
			r.With(d.AdminAuthMW.Require).Get("/2fa/setup",   d.AdminAuth.SetupTOTP)
			r.With(d.AdminAuthMW.Require).Post("/2fa/enable", d.AdminAuth.EnableTOTP)
			// /2fa/disable eliminado: el 2FA es obligatorio y permanente, no
			// tiene autoservicio de desactivación (ver /users/{id}/reset-2fa).
		})

		// Todos los demás endpoints requieren JWT válido
		r.Group(func(r chi.Router) {
			r.Use(d.AdminAuthMW.Require)

			// ── Recursos administrativos: solo superadmin/admin ──────
			// (gestión de tenants, credenciales, config, usuarios de la plataforma)
			r.Group(func(r chi.Router) {
				r.Use(d.AdminAuthMW.RequireRole("superadmin", "admin"))

				// Tenants
				r.Route("/tenants", func(r chi.Router) {
					r.Get("/",  d.AdminTenants.List)
					r.Post("/", d.AdminTenants.Create)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/",   d.AdminTenants.Get)
						r.Put("/",   d.AdminTenants.Update)
						r.Delete("/", d.AdminTenants.Delete)
						r.Post("/suspend",    d.AdminTenants.Suspend)
						r.Post("/reactivate", d.AdminTenants.Reactivate)
						r.Post("/verify-delete", d.AdminTenants.VerifyAndDelete)

						// Sub-recursos del tenant
						r.Get("/credentials",  d.AdminCredentials.ListByTenant)
						r.Post("/credentials", d.AdminCredentials.Create)

						r.Get("/ip-allowlist",  d.AdminIPAllowlist.ListByTenant)
						r.Post("/ip-allowlist", d.AdminIPAllowlist.Create)

						r.Get("/quota", d.AdminQuotas.Get)
						r.Put("/quota", d.AdminQuotas.Update)
						r.Post("/bundles", d.AdminQuotas.AddBundle)
						r.Get("/bundles", d.AdminQuotas.ListBundles)

						r.Get("/basic-auth",  d.AdminBasicAuth.ListByTenant)
						r.Post("/basic-auth", d.AdminBasicAuth.Create)

						// Alert emails adicionales
						r.Get("/alert-emails",  d.AdminAlerts.List)
						r.Post("/alert-emails", d.AdminAlerts.Add)
						r.Post("/alert-emails/test", d.AdminAlerts.Test)
						r.Delete("/alert-emails/{emailId}", d.AdminAlerts.Delete)

						// No-Auth Access (acceso por IP sin credenciales)
						r.Get("/noauth",          d.AdminNoAuth.GetByTenant)
						r.Post("/noauth/enable",  d.AdminNoAuth.Enable)
						r.Post("/noauth/disable", d.AdminNoAuth.Disable)
						r.Delete("/noauth",       d.AdminNoAuth.Delete)
					})
				})

				// Credenciales (acciones por ID de credencial)
				r.Route("/credentials/{id}", func(r chi.Router) {
					r.Post("/rotate", d.AdminCredentials.Rotate)
					r.Post("/revoke", d.AdminCredentials.Revoke)
				})

				// Basic Auth (revocar por ID)
				r.Route("/basic-auth/{id}", func(r chi.Router) {
					r.Post("/revoke", d.AdminBasicAuth.Revoke)
				})

				// IP allowlist (eliminar por ID de entrada)
				r.Route("/ip-allowlist/{id}", func(r chi.Router) {
					r.Put("/",    d.AdminIPAllowlist.Update)
					r.Delete("/", d.AdminIPAllowlist.Delete)
				})

				// Configuración (upstreams TSA)
				r.Route("/config/upstreams", func(r chi.Router) {
					r.Get("/",  d.AdminConfig.ListUpstreams)
					r.Post("/", d.AdminConfig.CreateUpstream)
					r.Route("/{id}", func(r chi.Router) {
						r.Put("/",             d.AdminConfig.UpdateUpstream)
						r.Delete("/",          d.AdminConfig.DeleteUpstream)
						r.Post("/set-default", d.AdminConfig.SetDefault)
					})
				})

				// Planes disponibles (solo lectura para el panel)
				r.Get("/plans", d.AdminConfig.ListPlans)

				// Usuarios de la plataforma
				r.Route("/users", func(r chi.Router) {
					r.Get("/",  d.AdminUsers.List)
					r.Post("/", d.AdminUsers.Create)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/",    d.AdminUsers.Get)
						r.Put("/",    d.AdminUsers.Update)
						r.Delete("/", d.AdminUsers.Delete)
						r.Post("/reset-password", d.AdminUsers.ResetPassword)
						r.Post("/reset-2fa",      d.AdminUsers.ResetTOTP)
					})
				})
				r.Get("/roles", d.AdminUsers.ListRoles)
			})

			// ── Reportes y auditoría: cualquier rol autenticado ──────
			// (el filtrado por scope de tenant para "viewer" ocurre dentro del handler)
			r.Route("/reports", func(r chi.Router) {
				r.Get("/usage",           d.AdminReports.Usage)
				r.Get("/usage.csv",       d.AdminReports.UsageCSV)
				r.Get("/usage/summary",   d.AdminReports.Summary)
				r.Get("/top-ips",         d.AdminReports.TopIPs)
				r.Get("/top-user-agents", d.AdminReports.TopUserAgents)
				r.Get("/top-countries",   d.AdminReports.TopCountries)
				r.Get("/failures",        d.AdminReports.FailureBreakdown)
				r.Get("/bundles", d.AdminQuotas.GetBundleReport)
				r.Post("/next-number", d.AdminReports.NextReportNumber)
			})

			r.Get("/audit-events", d.AdminAudit.List)
		})
	})

	return r
}
