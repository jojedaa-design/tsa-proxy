package health

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/config"
)

// Handler expone /health y /ready.
type Handler struct {
	cfg    *config.Config
	pgPool *pgxpool.Pool
	redis  *redis.Client
}

func NewHandler(cfg *config.Config, pg *pgxpool.Pool, rc *redis.Client) *Handler {
	return &Handler{cfg: cfg, pgPool: pg, redis: rc}
}

// Health es un liveness check simple. Siempre responde 200 si el proceso está vivo.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	apierr.WriteJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": h.cfg.Version,
		"env":     h.cfg.Environment,
	})
}

// Ready verifica que las dependencias críticas estén disponibles.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	pgStatus := "ok"
	if err := h.pgPool.Ping(ctx); err != nil {
		pgStatus = "error: " + err.Error()
	}

	redisStatus := "ok"
	if err := h.redis.Ping(ctx).Err(); err != nil {
		redisStatus = "error: " + err.Error()
	}

	ready := pgStatus == "ok" && redisStatus == "ok"
	status := http.StatusOK
	statusStr := "ready"

	if !ready {
		status = http.StatusServiceUnavailable
		statusStr = "not_ready"
	}

	apierr.WriteJSON(w, status, map[string]string{
		"status":   statusStr,
		"postgres": pgStatus,
		"redis":    redisStatus,
	})
}
