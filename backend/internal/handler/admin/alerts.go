package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	notifysvc "github.com/bigdavi/tsa-proxy/internal/service/notification"
	tenantsvc "github.com/bigdavi/tsa-proxy/internal/service/tenant"
)

type AlertsHandler struct {
	alertEmailRepo *postgres.AlertEmailRepository
	tenantSvc      *tenantsvc.Service
	notifier       *notifysvc.Client
}

func NewAlertsHandler(alertEmailRepo *postgres.AlertEmailRepository, tenantSvc *tenantsvc.Service, notifier *notifysvc.Client) *AlertsHandler {
	return &AlertsHandler{alertEmailRepo: alertEmailRepo, tenantSvc: tenantSvc, notifier: notifier}
}

// List — GET /api/admin/v1/tenants/{id}/alert-emails
func (h *AlertsHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	emails, err := h.alertEmailRepo.List(r.Context(), tenantID)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, emails)
}

// Add — POST /api/admin/v1/tenants/{id}/alert-emails
func (h *AlertsHandler) Add(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	var req struct {
		Email string `json:"email"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if req.Email == "" {
		apierr.WriteValidationError(w, r, map[string]string{"email": "required"})
		return
	}

	added, err := h.alertEmailRepo.Add(r.Context(), tenantID, req.Email, req.Label)
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusCreated, added)
}

// Delete — DELETE /api/admin/v1/tenants/{id}/alert-emails/{emailId}
func (h *AlertsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	emailID, err := uuid.Parse(chi.URLParam(r, "emailId"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	if err := h.alertEmailRepo.Delete(r.Context(), emailID); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Test — POST /api/admin/v1/tenants/{id}/alert-emails/test
func (h *AlertsHandler) Test(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, r, apierr.ErrBadRequest)
		return
	}

	tenant, err := h.tenantSvc.GetByID(r.Context(), tenantID)
	if err != nil || tenant == nil {
		apierr.WriteError(w, r, apierr.ErrNotFound)
		return
	}

	// Recolectar todos los destinatarios
	recipients := make([]string, 0, 4)
	if tenant.ContactEmail != nil && *tenant.ContactEmail != "" {
		recipients = append(recipients, *tenant.ContactEmail)
	}
	extras, _ := h.alertEmailRepo.ListEmails(r.Context(), tenantID)
	recipients = append(recipients, extras...)

	if len(recipients) == 0 {
		apierr.WriteValidationError(w, r, map[string]string{"email": "no hay destinatarios configurados para este cliente"})
		return
	}

	if h.notifier == nil || !h.notifier.Enabled() {
		apierr.WriteValidationError(w, r, map[string]string{"notifier": "el servicio de notificaciones no está configurado (falta BREVO_API_KEY)"})
		return
	}

	if err := h.notifier.SendTestAlert(r.Context(), tenant.Name, recipients); err != nil {
		apierr.WriteError(w, r, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"sent_to": recipients,
		"message": "Correo de prueba enviado correctamente",
	})
}
