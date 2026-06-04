package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

const RequestIDHeader = "X-Request-ID"

// RequestID genera un UUID único por request y lo inyecta
// en el contexto y en el header de respuesta.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(RequestIDHeader)
		if rid == "" {
			rid = uuid.New().String()
		}
		ctx := context.WithValue(r.Context(), model.CtxKeyRequestID, rid)
		w.Header().Set(RequestIDHeader, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extrae el request ID del contexto.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(model.CtxKeyRequestID).(string); ok {
		return v
	}
	return ""
}
