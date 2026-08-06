// Package apierr centraliza todos los errores HTTP de la API.
// Ningún handler debe escribir errores directamente — siempre
// se usa este paquete para garantizar consistencia y evitar
// filtración de detalles internos.
package apierr

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

// APIError es el tipo de error que se devuelve a los clientes.
type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"error"`
	Message    string `json:"message,omitempty"`
}

func (e *APIError) Error() string { return e.Code }

// Errores predefinidos del sistema
var (
	ErrUnauthorized     = &APIError{StatusCode: http.StatusUnauthorized,          Code: "invalid_credentials"}
	ErrAdminUnauthorized = &APIError{StatusCode: http.StatusUnauthorized,         Code: "unauthorized"}
	ErrForbidden        = &APIError{StatusCode: http.StatusForbidden,             Code: "forbidden"}
	ErrTenantSuspended  = &APIError{StatusCode: http.StatusForbidden,             Code: "tenant_suspended"}
	ErrIPNotAllowed     = &APIError{StatusCode: http.StatusForbidden,             Code: "ip_not_allowed"}
	ErrQuotaExceeded    = &APIError{StatusCode: http.StatusTooManyRequests,       Code: "quota_exceeded"}
	ErrRateLimited      = &APIError{StatusCode: http.StatusTooManyRequests,       Code: "rate_limited"}
	ErrUpstreamUnavailable = &APIError{StatusCode: http.StatusBadGateway,        Code: "upstream_unavailable"}
	ErrInternal         = &APIError{StatusCode: http.StatusInternalServerError,   Code: "internal_error"}
	ErrNotFound         = &APIError{StatusCode: http.StatusNotFound,              Code: "not_found"}
	ErrBadRequest       = &APIError{StatusCode: http.StatusBadRequest,            Code: "bad_request"}
	ErrConflict         = &APIError{StatusCode: http.StatusConflict,              Code: "conflict"}
	ErrUsernameTaken    = &APIError{StatusCode: http.StatusConflict, Code: "username_taken", Message: "El nombre de usuario ya está en uso"}
	ErrEmailTaken       = &APIError{StatusCode: http.StatusConflict, Code: "email_taken", Message: "El email ya está en uso por otro usuario"}
)

// errorResponse es la estructura JSON enviada al cliente.
type errorResponse struct {
	Error     string                 `json:"error"`
	Message   string                 `json:"message,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// WriteError escribe un APIError al response writer.
func WriteError(w http.ResponseWriter, r *http.Request, err *APIError) {
	WriteErrorWithExtra(w, r, err, nil)
}

// WriteErrorWithExtra escribe un APIError con campos adicionales (retry_after, etc.).
func WriteErrorWithExtra(w http.ResponseWriter, r *http.Request, err *APIError, extra map[string]interface{}) {
	rid := ""
	if r != nil {
		rid = r.Header.Get("X-Request-ID")
	}

	resp := errorResponse{
		Error:     err.Code,
		Message:   err.Message,
		RequestID: rid,
		Extra:     extra,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(err.StatusCode)

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		log.Error().Err(encErr).Msg("failed to encode error response")
	}
}

// WriteValidationError escribe un error 400 con detalles de validación.
func WriteValidationError(w http.ResponseWriter, r *http.Request, fields map[string]string) {
	rid := ""
	if r != nil {
		rid = r.Header.Get("X-Request-ID")
	}

	resp := struct {
		Error     string            `json:"error"`
		RequestID string            `json:"request_id,omitempty"`
		Fields    map[string]string `json:"fields"`
	}{
		Error:     "validation_error",
		RequestID: rid,
		Fields:    fields,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(resp)
}

// WriteJSON escribe una respuesta JSON exitosa.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Error().Err(err).Msg("failed to encode JSON response")
		}
	}
}
