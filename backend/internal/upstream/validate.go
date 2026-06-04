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
//  1. Esquema: solo http:// y https:// (bloquea file://, gopher://, ftp://, etc.)
//  2. Hostname presente y no vacío.
//  3. Si allowedHosts no está vacío, el hostname debe estar en la lista.
//  4. Resolución DNS: el hostname debe resolver a al menos una IP.
//  5. Ninguna IP resuelta puede ser privada, loopback, link-local, multicast
//     ni no-especificada — bloquea SSRF contra servicios internos, metadatos
//     de cloud (169.254.169.254), Redis, Postgres, etc.
func ValidateURL(rawURL string, allowedHosts []string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("la URL no puede estar vacía")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL inválida: %w", err)
	}

	// Solo http y https (block file://, gopher://, ftp://, etc.)
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("esquema %q no permitido: solo se acepta http:// o https://", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("la URL debe incluir un hostname")
	}

	// Lista blanca de hostnames (opcional)
	if len(allowedHosts) > 0 {
		found := false
		for _, h := range allowedHosts {
			if strings.EqualFold(host, strings.TrimSpace(h)) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("hostname %q no está en la lista de upstreams permitidos (TSA_UPSTREAM_ALLOWLIST)", host)
		}
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
