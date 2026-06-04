package middleware

import (
	"net/http"

	"github.com/bigdavi/tsa-proxy/internal/config"
)

// CORS restringe el acceso al panel admin solo desde el dominio configurado.
// El proxy público no necesita CORS (es una API binary, no llamada desde browser).
func CORS(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				// Solo permitir el dominio del panel admin
				adminOrigin := "https://ast.bigdavi.com"
				if cfg.Environment == "development" {
					adminOrigin = "http://localhost:3000"
				}
				if origin == adminOrigin {
					w.Header().Set("Access-Control-Allow-Origin",  origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
