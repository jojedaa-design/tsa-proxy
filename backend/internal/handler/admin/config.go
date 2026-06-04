package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
	"github.com/bigdavi/tsa-proxy/internal/upstream"
)

type ConfigHandler struct {
	upstreamRepo *postgres.UpstreamRepository
	quotaRepo    *postgres.QuotaRepository
	cache        *rediscache.Cache
	// allowedHosts es la lista blanca de hostnames permitidos como upstream TSA.
	// Si está vacía, solo se validan el esquema y las IPs privadas (no hay restricción de dominio).
	allowedHosts []string
}

func NewConfigHandler(
	upstreamRepo *postgres.UpstreamRepository,
	quotaRepo *postgres.QuotaRepository,
	cache *rediscache.Cache,
	allowedHosts []string,
) *ConfigHandler {
	return &ConfigHandler{
		upstreamRepo: upstreamRepo,
		quotaRepo:    quotaRepo,
		cache:        cache,
		allowedHosts: allowedHosts,
	}
}

func upstreamResponse(u *model.TSAUpstream) map[string]interface{} {
	resp := map[string]interface{}{
		"id":           u.ID,
		"name":         u.Name,
		"url":          u.URL,
		"timeout_ms":   u.TimeoutMs,
		"max_retries":  u.MaxRetries,
		"is_active":    u.IsActive,
		"is_default":   u.IsDefault,
		"has_password": u.HasPassword,
		"created_at":   u.CreatedAt,
		"updated_at":   u.UpdatedAt,
	}
	if u.Username != nil && *u.Username != "" {
		resp["username"] = *u.Username
	}
	return resp
}

// ListUpstreams — GET /api/admin/v1/config/upstreams
func (h *ConfigHandler) ListUpstreams(w http.ResponseWriter, r *http.Request) {
	allUpstreams, err := h.upstreamRepo.List(r.Context())
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	data := make([]map[string]interface{}, 0)
	for _, u := range allUpstreams {
		if u.IsActive {
			data = append(data, upstreamResponse(u))
		}
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

// CreateUpstream — POST /api/admin/v1/config/upstreams
func (h *ConfigHandler) CreateUpstream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string  `json:"name"`
		URL        string  `json:"url"`
		Username   *string `json:"username"`
		Password   *string `json:"password"`
		TimeoutMs  int     `json:"timeout_ms"`
		MaxRetries int     `json:"max_retries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	fields := map[string]string{}
	if req.Name == "" {
		fields["name"] = "required"
	}
	if req.URL == "" {
		fields["url"] = "required"
	}
	if len(fields) > 0 {
		apierr.WriteValidationError(w, r, fields)
		return
	}

	// Validación de seguridad: esquema, IPs privadas y lista blanca de hostnames
	if err := upstream.ValidateURL(req.URL, h.allowedHosts); err != nil {
		apierr.WriteValidationError(w, r, map[string]string{"url": err.Error()})
		return
	}

	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 10000
	}
	if req.MaxRetries < 0 {
		req.MaxRetries = 1
	}

	if req.Username != nil && *req.Username == "" {
		req.Username = nil
	}
	if req.Password != nil && *req.Password == "" {
		req.Password = nil
	}

	created, err := h.upstreamRepo.Create(r.Context(), &model.TSAUpstream{
		Name:       req.Name,
		URL:        req.URL,
		Username:   req.Username,
		Password:   req.Password,
		TimeoutMs:  req.TimeoutMs,
		MaxRetries: req.MaxRetries,
	})
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusCreated, upstreamResponse(created))
}

// UpdateUpstream — PUT /api/admin/v1/config/upstreams/:id
func (h *ConfigHandler) UpdateUpstream(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Name       string  `json:"name"`
		URL        string  `json:"url"`
		Username   *string `json:"username"`
		Password   *string `json:"password"`
		TimeoutMs  int     `json:"timeout_ms"`
		MaxRetries int     `json:"max_retries"`
		IsActive   *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	current, err := h.upstreamRepo.GetByID(r.Context(), id)
	if err != nil || current == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	if req.Name != "" {
		current.Name = req.Name
	}
	if req.URL != "" {
		// Validación de seguridad al actualizar la URL del upstream
		if err := upstream.ValidateURL(req.URL, h.allowedHosts); err != nil {
			apierr.WriteValidationError(w, r, map[string]string{"url": err.Error()})
			return
		}
		current.URL = req.URL
	}
	if req.Username != nil {
		if *req.Username == "" {
			current.Username = nil
		} else {
			current.Username = req.Username
		}
	}
	if req.Password != nil {
		if *req.Password == "" {
			current.Password = nil
		} else {
			current.Password = req.Password
		}
	}
	if req.TimeoutMs > 0 {
		current.TimeoutMs = req.TimeoutMs
	}
	if req.MaxRetries >= 0 {
		current.MaxRetries = req.MaxRetries
	}
	if req.IsActive != nil {
		current.IsActive = *req.IsActive
	}

	updated, err := h.upstreamRepo.Update(r.Context(), current)
	if err != nil || updated == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	// Invalidar cache si era el default
	if updated.IsDefault && h.cache != nil {
		go func() { _ = h.cache.InvalidateDefaultUpstream(context.Background()) }()
	}

	apierr.WriteJSON(w, http.StatusOK, upstreamResponse(updated))
}

// SetDefault — POST /api/admin/v1/config/upstreams/:id/set-default
func (h *ConfigHandler) SetDefault(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if err := h.upstreamRepo.SetDefault(r.Context(), id); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Invalidar cache del upstream default
	if h.cache != nil {
		go func() { _ = h.cache.InvalidateDefaultUpstream(context.Background()) }()
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "default_set"})
}

// DeleteUpstream — DELETE /api/admin/v1/config/upstreams/:id
func (h *ConfigHandler) DeleteUpstream(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	record, err := h.upstreamRepo.GetByID(r.Context(), id)
	if err != nil || record == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	record.IsActive = false
	if _, err := h.upstreamRepo.Update(r.Context(), record); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	// Invalidar cache si era el default
	if record.IsDefault && h.cache != nil {
		go func() { _ = h.cache.InvalidateDefaultUpstream(context.Background()) }()
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListPlans — GET /api/admin/v1/plans
func (h *ConfigHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.quotaRepo.ListPlans(r.Context())
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": plans})
}
