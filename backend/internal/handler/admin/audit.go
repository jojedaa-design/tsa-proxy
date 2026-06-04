package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
)

type AuditHandler struct {
	repo *postgres.AuditRepository
}

func NewAuditHandler(repo *postgres.AuditRepository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// List — GET /api/admin/v1/audit-events
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1  { page = 1 }
	if limit < 1 { limit = 50 }

	f := postgres.AuditFilter{
		Action:     q.Get("action"),
		EntityType: q.Get("entity_type"),
		Page:       page,
		Limit:      limit,
	}

	if aid := q.Get("actor_id"); aid != "" {
		id, err := uuid.Parse(aid)
		if err == nil {
			f.ActorID = &id
		}
	}
	if eid := q.Get("entity_id"); eid != "" {
		id, err := uuid.Parse(eid)
		if err == nil {
			f.EntityID = &id
		}
	}
	if from := q.Get("from"); from != "" {
		t, err := time.Parse("2006-01-02", from)
		if err == nil {
			f.From = &t
		}
	}
	if to := q.Get("to"); to != "" {
		t, err := time.Parse("2006-01-02", to)
		if err == nil {
			end := t.Add(24*time.Hour - time.Second)
			f.To = &end
		}
	}

	events, total, err := h.repo.List(r.Context(), f)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	pagination := model.NewPagination(page, limit, total)
	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data":       events,
		"pagination": pagination,
	})
}
