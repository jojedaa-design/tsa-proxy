package admin

import (
	"net/http"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/service/sysmetrics"
)

type SystemHandler struct {
	sampler *sysmetrics.Sampler
}

func NewSystemHandler(sampler *sysmetrics.Sampler) *SystemHandler {
	return &SystemHandler{sampler: sampler}
}

// Hardware — GET /api/admin/v1/system/hardware
func (h *SystemHandler) Hardware(w http.ResponseWriter, r *http.Request) {
	apierr.WriteJSON(w, http.StatusOK, h.sampler.Get())
}
