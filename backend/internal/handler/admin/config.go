package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
	"github.com/bigdavi/tsa-proxy/internal/upstream"
)

type ConfigHandler struct {
	upstreamRepo *postgres.UpstreamRepository
	quotaRepo    *postgres.QuotaRepository
	cache        *rediscache.Cache
	audit        *postgres.AuditRepository
	// allowedHosts es la lista blanca de hostnames permitidos como upstream TSA.
	// Es obligatoria: si está vacía, upstream.ValidateURL rechaza toda URL.
	allowedHosts []string
}

func NewConfigHandler(
	upstreamRepo *postgres.UpstreamRepository,
	quotaRepo *postgres.QuotaRepository,
	cache *rediscache.Cache,
	audit *postgres.AuditRepository,
	allowedHosts []string,
) *ConfigHandler {
	return &ConfigHandler{
		upstreamRepo: upstreamRepo,
		quotaRepo:    quotaRepo,
		cache:        cache,
		audit:        audit,
		allowedHosts: allowedHosts,
	}
}

// recordUpstreamChange deja rastro de cualquier cambio sobre tsa_upstreams.
//
// Toda modificación acá es sensible: la URL determina a qué host se le entregan
// las credenciales Basic de la TSA externa. Por eso además del evento de
// auditoría se emite un log de nivel warn con la marca "security_event", pensado
// para que el pipeline de logs dispare una alerta out-of-band.
//
// Nunca se registran valores de credenciales — solo indicadores booleanos.
func (h *ConfigHandler) recordUpstreamChange(
	r *http.Request, action string, upstreamID uuid.UUID, changes map[string]interface{},
) {
	actorID, _ := middleware.GetAdminUserID(r.Context())

	log.Warn().
		Str("security_event", "tsa_upstream_change").
		Str("action", action).
		Str("actor_id", actorID.String()).
		Str("upstream_id", upstreamID.String()).
		Interface("changes", changes).
		Msg("configuración de upstream TSA modificada")

	if h.audit == nil {
		return
	}
	eid := upstreamID
	go func() {
		_ = h.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     action,
			EntityType: "tsa_upstream",
			EntityID:   &eid,
			Changes:    changes,
		})
	}()
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

	h.recordUpstreamChange(r, "upstream.create", created.ID, map[string]interface{}{
		"name":            created.Name,
		"url":             created.URL,
		"has_credentials": req.Username != nil,
	})

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

	previousURL := current.URL

	if req.Name != "" {
		current.Name = req.Name
	}
	// credentialsCleared indica que se borraron las credenciales almacenadas por
	// un cambio de host — se refleja en la auditoría y en la respuesta.
	credentialsCleared := false
	if req.URL != "" {
		// Validación de seguridad al actualizar la URL del upstream
		if err := upstream.ValidateURL(req.URL, h.allowedHosts); err != nil {
			apierr.WriteValidationError(w, r, map[string]string{"url": err.Error()})
			return
		}

		// Seguridad: si el upstream cambia de host, las credenciales guardadas
		// dejan de pertenecer a ese destino. Se borran ANTES de aplicar los
		// campos del request para que el proxy no pueda replicar la credencial
		// vieja hacia el host nuevo. Si el admin quiere mover el upstream de
		// verdad, debe reingresar las credenciales (puede hacerlo en este mismo
		// request, más abajo).
		if !upstream.SameHost(previousURL, req.URL) && (current.Username != nil || current.Password != nil) {
			current.Username = nil
			current.Password = nil
			credentialsCleared = true
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
	// Tras un cambio de host, no dejar media credencial: el formulario del panel
	// no puede reenviar el password (la API nunca lo devuelve), así que un update
	// que solo trae el username produciría un Basic Auth con password vacío y un
	// 401 difícil de diagnosticar. Si no llegó password nuevo, se descarta también
	// el username y el upstream queda explícitamente sin credenciales.
	if credentialsCleared && current.Password == nil {
		current.Username = nil
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

	changes := map[string]interface{}{
		"name":            updated.Name,
		"has_credentials": updated.Username != nil,
		"is_active":       updated.IsActive,
	}
	if updated.URL != previousURL {
		changes["url_before"] = previousURL
		changes["url_after"] = updated.URL
	}
	if credentialsCleared {
		changes["credentials_cleared"] = true
		changes["reason"] = "el host del upstream cambió; se requiere reingresar credenciales"
	}
	h.recordUpstreamChange(r, "upstream.update", updated.ID, changes)

	resp := upstreamResponse(updated)
	if credentialsCleared {
		resp["credentials_cleared"] = true
	}
	apierr.WriteJSON(w, http.StatusOK, resp)
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

	h.recordUpstreamChange(r, "upstream.set_default", id, map[string]interface{}{
		"is_default": true,
	})

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

	h.recordUpstreamChange(r, "upstream.delete", id, map[string]interface{}{
		"name":      record.Name,
		"url":       record.URL,
		"is_active": false,
	})

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
