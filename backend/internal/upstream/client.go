// Package upstream implementa el cliente HTTP para la TSA externa.
// Las credenciales nunca salen de este paquete ni se loguean.
package upstream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog/log"
)

// Response contiene la respuesta del upstream TSA.
type Response struct {
	Body       []byte
	HTTPStatus int
	LatencyMs  int
}

// SendParams contiene los parámetros de envío resueltos para un request.
type SendParams struct {
	URL        string
	Username   string
	Password   string
	MaxRetries int
	MaxBody    int64
	Timeout    time.Duration
}

// Client es el cliente HTTP hacia la TSA upstream.
// No guarda URL ni credenciales — las recibe dinámicamente por request,
// lo que permite cambiar el upstream desde el panel de administración sin reiniciar.
type Client struct {
	httpClient *http.Client
}

// NewClient crea un cliente HTTP reutilizable (pool de conexiones).
//
// El transport usa safeDialContext para rechazar conexiones a IPs privadas,
// loopback, link-local y multicast, protegiendo contra SSRF y DNS rebinding
// incluso si la URL pasó la validación estática en el handler.
func NewClient(timeout time.Duration) *Client {
	transport := &http.Transport{
		DialContext:         safeDialContext,
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 64,
		MaxConnsPerHost:     128,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

// safeDialContext resuelve el hostname vía DNS y bloquea la conexión si cualquiera
// de las IPs resueltas es privada, loopback o reservada.
//
// Esto es la segunda capa de defensa contra SSRF: actúa en el momento exacto
// en que se abre el socket TCP, protegiéndose contra ataques de DNS rebinding
// donde un hostname público puede cambiar su resolución a una IP interna
// después de haber pasado la validación en CreateUpstream/UpdateUpstream.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("dirección de upstream inválida %q: %w", addr, err)
	}

	// Resolver DNS manualmente antes de abrir el socket
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup para upstream %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("DNS lookup para upstream %q: sin resultados", host)
	}

	// Rechazar si CUALQUIER IP resuelta es privada/reservada
	for _, ip := range ips {
		if isPrivateOrReserved(ip) {
			return nil, fmt.Errorf(
				"upstream bloqueado (SSRF): %q resuelve a %s (dirección privada/reservada)",
				host, ip,
			)
		}
	}

	// Todas las IPs son públicas — conectar a la primera disponible
	d := &net.Dialer{}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

// ResolveCredentials lee las credenciales desde variables de entorno.
// Si envKeyUser o envKeyPass están vacíos, devuelve strings vacíos (TSA sin auth).
func ResolveCredentials(envKeyUser, envKeyPass *string) (user, pass string) {
	if envKeyUser != nil && *envKeyUser != "" {
		user = os.Getenv(*envKeyUser)
	}
	if envKeyPass != nil && *envKeyPass != "" {
		pass = os.Getenv(*envKeyPass)
	}
	return
}

// Send envía un TimeStampRequest RFC 3161 al upstream con los parámetros dados.
// Los parámetros (URL, credenciales) se resuelven dinámicamente desde la BD.
//
// El timeout por request se aplica vía context (ver doRequest), no mutando
// c.httpClient.Timeout: ese campo es compartido por todos los goroutines que
// usan este *Client (uno solo, reutilizado para todo el proxy), así que
// escribirlo aquí era una carrera de datos real — el timeout de un tenant
// podía terminar aplicándose a la petición en vuelo de otro.
func (c *Client) Send(ctx context.Context, body []byte, p SendParams) (*Response, error) {
	if p.MaxBody <= 0 {
		p.MaxBody = 32768
	}

	var lastErr error
	var resp *Response

	for attempt := 0; attempt <= p.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*200) * time.Millisecond):
			}
		}

		resp, lastErr = c.doRequest(ctx, body, p)
		if lastErr == nil {
			return resp, nil
		}

		// No reintentar en errores 4xx del upstream
		if resp != nil && resp.HTTPStatus >= 400 && resp.HTTPStatus < 500 {
			return resp, fmt.Errorf("upstream returned status %d", resp.HTTPStatus)
		}

		log.Warn().
			Int("attempt", attempt+1).
			Int("max_retries", p.MaxRetries).
			Msg("upstream request failed, retrying")
	}

	return nil, fmt.Errorf("upstream unavailable after %d attempts", p.MaxRetries+1)
}

func (c *Client) doRequest(ctx context.Context, body []byte, p SendParams) (*Response, error) {
	start := time.Now()

	// Timeout por request vía context — c.httpClient.Timeout (fijado una vez
	// en NewClient) queda como backstop compartido y nunca se reescribe.
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/timestamp-query")
	req.Header.Set("Accept", "application/timestamp-reply")

	// Solo enviar Basic Auth si hay credenciales
	if p.Username != "" || p.Password != "" {
		req.SetBasicAuth(p.Username, p.Password)
	}

	httpResp, err := c.httpClient.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("upstream request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, p.MaxBody))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	return &Response{
		Body:       respBody,
		HTTPStatus: httpResp.StatusCode,
		LatencyMs:  latencyMs,
	}, nil
}
