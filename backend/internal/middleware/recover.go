package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
)

// Recoverer captura panics, los loguea con stack trace y
// responde 500 al cliente sin exponer detalles internos.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				rid := GetRequestID(r.Context())
				log.Error().
					Str("request_id", rid).
					Interface("panic", rec).
					Bytes("stack", debug.Stack()).
					Msg("panic recovered")
				apierr.WriteError(w, r, apierr.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
