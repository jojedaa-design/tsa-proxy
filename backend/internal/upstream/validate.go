package upstream

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateURL verifica que una URL es segura para usarla como upstream TSA.
//
// Controles aplicados (en orden):
//  1. Lista blanca de hostnames OBLIGATORIA (fail-closed): si allowedHosts está
//     vacía se rechaza todo. Ver nota de seguridad abajo.
//  2. Esquema: solo http:// y https:// (bloquea file://, gopher://, ftp://, etc.)
//  3. Hostname presente y no vacío.
//  4. El hostname debe estar en allowedHosts.
//  5. Resolución DNS: el hostname debe resolver a al menos una IP.
//  6. Ninguna IP resuelta puede ser privada, loopback, link-local, multicast
//     ni no-especificada — bloquea SSRF contra servicios internos, metadatos
//     de cloud (169.254.169.254), Redis, Postgres, etc.
//
// # Seguridad: por qué la allowlist es obligatoria
//
// El cliente HTTP adjunta las credenciales Basic del upstream a CUALQUIER URL
// configurada. Si un atacante con sesión de admin apunta el upstream a un host
// que controla, el proxy le entrega la credencial de la TSA externa en el
// siguiente sello — sin necesidad de leer la base de datos. La allowlist es el
// control que impide ese vector, así que no puede ser opcional: una lista vacía
// significa "configuración incompleta", no "permitir todo".
func ValidateURL(rawURL string, allowedHosts []string) error {
	host, err := CheckAllowlist(rawURL, allowedHosts)
	if err != nil {
		return err
	}

	// Resolver DNS y rechazar IPs privadas/reservadas
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("no se puede resolver el hostname %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("hostname %q no devolvió ninguna dirección IP", host)
	}

	for _, ip := range ips {
		if isPrivateOrReserved(ip) {
			return fmt.Errorf(
				"hostname %q resuelve a la dirección %s, que es privada o reservada — no permitido",
				host, ip,
			)
		}
	}

	return nil
}

// CheckAllowlist aplica los controles estáticos de ValidateURL (allowlist
// obligatoria, esquema y hostname) y devuelve el hostname validado.
//
// A diferencia de ValidateURL no resuelve DNS, por lo que es seguro llamarla en
// el arranque del servicio: una caída transitoria de DNS no debe impedir el
// boot. La comprobación de IPs privadas se sigue aplicando en cada request vía
// safeDialContext, así que omitirla acá no abre ningún hueco de SSRF.
func CheckAllowlist(rawURL string, allowedHosts []string) (string, error) {
	// Fail-closed: sin allowlist configurada no se acepta ningún upstream.
	if len(allowedHosts) == 0 {
		return "", errors.New(
			"TSA_UPSTREAM_ALLOWLIST no está configurada: no se puede aceptar ningún " +
				"upstream TSA hasta definir la lista blanca de hostnames permitidos",
		)
	}

	if strings.TrimSpace(rawURL) == "" {
		return "", errors.New("la URL no puede estar vacía")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("URL inválida: %w", err)
	}

	// Solo http y https (bloquea file://, gopher://, ftp://, etc.)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("esquema %q no permitido: solo se acepta http:// o https://", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return "", errors.New("la URL debe incluir un hostname")
	}

	for _, h := range allowedHosts {
		if strings.EqualFold(host, strings.TrimSpace(h)) {
			return host, nil
		}
	}
	return "", fmt.Errorf(
		"hostname %q no está en la lista de upstreams permitidos (TSA_UPSTREAM_ALLOWLIST)", host,
	)
}

// SameHost indica si dos URLs apuntan al mismo hostname (comparación
// case-insensitive, ignorando esquema, puerto y path).
//
// Se usa para detectar cuándo un upstream cambia de destino: si el host cambia,
// las credenciales almacenadas dejan de ser válidas para ese destino y deben
// borrarse antes de que el proxy pueda replicarlas hacia el host nuevo.
// Una URL que no parsea se trata como host distinto (fail-closed).
func SameHost(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	ha, hb := ua.Hostname(), ub.Hostname()
	if ha == "" || hb == "" {
		return false
	}
	return strings.EqualFold(ha, hb)
}

// isPrivateOrReserved devuelve true si la IP pertenece a un rango
// no enrutable públicamente o potencialmente explotable vía SSRF:
//   - Loopback      127.0.0.0/8  ::1
//   - Privado       10/8  172.16/12  192.168/16  fc00::/7
//   - Link-local    169.254.0.0/16  fe80::/10  (incluye IMDS de cloud)
//   - Multicast     224.0.0.0/4  ff00::/8
//   - No especif.   0.0.0.0  ::
func isPrivateOrReserved(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}
