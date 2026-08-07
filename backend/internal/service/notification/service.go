// Package notification gestiona el envío de alertas por correo electrónico
// mediante la API transaccional de Brevo (v3).
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const brevoSendURL = "https://api.brevo.com/v3/smtp/email"

// Client es el cliente de la API transaccional de Brevo.
// Si APIKey está vacío, Send es un no-op (servicio deshabilitado).
type Client struct {
	apiKey string
	http   *http.Client
}

// New construye un Client. Si apiKey está vacío, el cliente queda deshabilitado
// y todas las llamadas a Send retornan inmediatamente sin error.
func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Enabled informa si el cliente tiene API key configurada.
func (c *Client) Enabled() bool {
	return c.apiKey != ""
}

// BundleAlertParams contiene los datos para construir el correo de alerta.
type BundleAlertParams struct {
	To               []string // destinatarios (primario + adicionales)
	TenantName       string   // nombre del cliente
	BundleNote       string   // referencia / descripción de la bolsa
	Consumed         int      // sellos consumidos de esta bolsa (FIFO)
	Amount           int      // total de sellos de la bolsa
	ThresholdPercent int      // umbral que disparó la alerta
}

// SendBundleAlert envía un correo de alerta de consumo de bolsa a todos los destinatarios.
// Retorna el messageId de Brevo del primer envío (para tracking) o error.
func (c *Client) SendBundleAlert(ctx context.Context, p BundleAlertParams) (string, error) {
	if !c.Enabled() || len(p.To) == 0 {
		return "", nil
	}

	pct := 0
	if p.Amount > 0 {
		pct = p.Consumed * 100 / p.Amount
	}
	remaining := p.Amount - p.Consumed

	bundleDesc := p.BundleNote
	if bundleDesc == "" {
		bundleDesc = "bolsa de sellos"
	}

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="es">
<head><meta charset="UTF-8"><style>
body { font-family: Arial, sans-serif; color: #222; margin: 0; padding: 0; background: #f4f6f9; }
.container { max-width: 520px; margin: 32px auto; background: #fff; border-radius: 10px;
  box-shadow: 0 2px 8px rgba(0,0,0,.1); overflow: hidden; }
.header { background: #0B1F3A; padding: 24px 32px; }
.header h1 { color: #fff; margin: 0; font-size: 20px; }
.header p { color: #8ca3c0; margin: 4px 0 0; font-size: 13px; }
.body { padding: 28px 32px; }
.alert-box { background: #fff8e1; border-left: 4px solid #f59e0b; border-radius: 4px;
  padding: 14px 16px; margin-bottom: 20px; }
.alert-box p { margin: 0; font-size: 14px; color: #92400e; }
.stats { display: flex; gap: 12px; margin-bottom: 20px; }
.stat { flex: 1; background: #f4f6f9; border-radius: 8px; padding: 14px; text-align: center; }
.stat .value { font-size: 24px; font-weight: bold; color: #0B1F3A; }
.stat .label { font-size: 11px; color: #666; margin-top: 4px; }
.progress-bar { background: #e5e7eb; border-radius: 99px; height: 10px; margin: 16px 0; }
.progress-fill { background: #f59e0b; border-radius: 99px; height: 10px; width: %d%%; }
.footer { padding: 16px 32px; border-top: 1px solid #e5e7eb; font-size: 12px; color: #999; }
</style></head>
<body>
<div class="container">
  <div class="header">
    <h1>BIGDAVI — Alerta de consumo</h1>
    <p>Sistema de Sellos de Tiempo (TSA)</p>
  </div>
  <div class="body">
    <div class="alert-box">
      <p><strong>%s</strong> ha alcanzado el <strong>%d%%</strong> de consumo de la %s.</p>
    </div>
    <div class="stats">
      <div class="stat"><div class="value">%s</div><div class="label">Consumidos</div></div>
      <div class="stat"><div class="value">%s</div><div class="label">Disponibles</div></div>
      <div class="stat"><div class="value">%s</div><div class="label">Total bolsa</div></div>
    </div>
    <div class="progress-bar"><div class="progress-fill"></div></div>
    <p style="font-size:13px;color:#555;">
      Se recomienda contratar una nueva bolsa antes de que se agoten los sellos restantes.
      Ingrese al panel de administración para gestionar sus bolsas.
    </p>
  </div>
  <div class="footer">
    Documento generado automáticamente. Confidencial, uso interno.<br>
    BIGDAVI — Panel de Administración TSA
  </div>
</div>
</body></html>`,
		pct,
		p.TenantName, pct, bundleDesc,
		formatNum(p.Consumed), formatNum(remaining), formatNum(p.Amount),
	)

	subject := fmt.Sprintf("Alerta de consumo: %s ha consumido el %d%% de su bolsa", p.TenantName, pct)

	toList := make([]map[string]string, 0, len(p.To))
	for _, addr := range p.To {
		toList = append(toList, map[string]string{"email": addr})
	}

	payload := map[string]interface{}{
		"sender": map[string]string{
			"name":  "BIGDAVI TSA",
			"email": "noreply@bigdavi.com",
		},
		"to":          toList,
		"subject":     subject,
		"htmlContent": htmlBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("notification: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brevoSendURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("notification: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("notification: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return "", fmt.Errorf("notification: brevo status %d: %v", resp.StatusCode, errBody)
	}

	var result struct {
		MessageID string `json:"messageId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("notification: decode response: %w", err)
	}
	return result.MessageID, nil
}

// SendTestAlert envía un correo de prueba a los destinatarios indicados.
func (c *Client) SendTestAlert(ctx context.Context, tenantName string, to []string) error {
	if !c.Enabled() || len(to) == 0 {
		return nil
	}

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="es">
<head><meta charset="UTF-8"><style>
body { font-family: Arial, sans-serif; color: #222; margin: 0; padding: 0; background: #f4f6f9; }
.container { max-width: 520px; margin: 32px auto; background: #fff; border-radius: 10px;
  box-shadow: 0 2px 8px rgba(0,0,0,.1); overflow: hidden; }
.header { background: #0B1F3A; padding: 24px 32px; }
.header h1 { color: #fff; margin: 0; font-size: 20px; }
.header p { color: #8ca3c0; margin: 4px 0 0; font-size: 13px; }
.body { padding: 28px 32px; }
.info-box { background: #e0f2fe; border-left: 4px solid #1274F2; border-radius: 4px;
  padding: 14px 16px; margin-bottom: 20px; }
.info-box p { margin: 0; font-size: 14px; color: #0c4a6e; }
.footer { padding: 16px 32px; border-top: 1px solid #e5e7eb; font-size: 12px; color: #999; }
</style></head>
<body>
<div class="container">
  <div class="header">
    <h1>BIGDAVI — Prueba de alerta</h1>
    <p>Sistema de Sellos de Tiempo (TSA)</p>
  </div>
  <div class="body">
    <div class="info-box">
      <p>Este es un correo de <strong>prueba</strong> para verificar que las alertas de consumo de bolsa
      lleguen correctamente al cliente <strong>%s</strong>.</p>
    </div>
    <p style="font-size:13px;color:#555;">
      Si recibió este mensaje, la configuración de alertas está funcionando correctamente.
      Recibirá notificaciones similares cuando el consumo de sellos de tiempo alcance
      el umbral configurado para cada bolsa.
    </p>
  </div>
  <div class="footer">
    Documento generado automáticamente. Confidencial, uso interno.<br>
    BIGDAVI — Panel de Administración TSA
  </div>
</div>
</body></html>`, tenantName)

	toList := make([]map[string]string, 0, len(to))
	for _, addr := range to {
		toList = append(toList, map[string]string{"email": addr})
	}

	payload := map[string]interface{}{
		"sender": map[string]string{
			"name":  "BIGDAVI TSA",
			"email": "noreply@bigdavi.com",
		},
		"to":          toList,
		"subject":     fmt.Sprintf("[PRUEBA] Alerta de consumo TSA — %s", tenantName),
		"htmlContent": htmlBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notification: marshal test payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brevoSendURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notification: build test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("notification: http test: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("notification: brevo status %d: %v", resp.StatusCode, errBody)
	}
	return nil
}

func formatNum(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	// separador de miles simple
	s := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
