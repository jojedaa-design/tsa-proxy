package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Logger es un middleware de logging estructurado por request.
// No loguea ningún header de autorización ni credenciales.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := GetRequestID(r.Context())

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		log.Info().
			Str("request_id", rid).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("client_ip", r.RemoteAddr).
			Int("status", wrapped.statusCode).
			Int64("bytes", wrapped.bytesWritten).
			Dur("latency_ms", duration).
			Msg("request")
	})
}

// responseWriter captura el status code y bytes escritos.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}
