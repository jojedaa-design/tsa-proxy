// Package geoloc proporciona búsqueda de geolocalización de IPs usando MaxMind GeoLite2.
// La BD se descarga automáticamente si no existe y MAXMIND_LICENSE_KEY está configurada.
package geoloc

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/oschwald/geoip2-golang"
	"github.com/rs/zerolog/log"
)

// Locator proporciona búsquedas de geolocalización.
type Locator struct {
	reader *geoip2.Reader
}

// GeoInfo contiene información de geolocalización.
type GeoInfo struct {
	Country string
	City    string
	ASN     string
}

// New crea un nuevo locator. Si el archivo no existe y MAXMIND_LICENSE_KEY
// está configurada, intenta descargarlo automáticamente desde MaxMind.
func New(dbPath string) (*Locator, error) {
	// Si el archivo no existe, intentar descargarlo si hay licencia disponible
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if os.Getenv("MAXMIND_LICENSE_KEY") != "" {
			log.Info().Str("path", dbPath).Msg("GeoLite2 database not found, downloading from MaxMind...")
			if dlErr := DownloadDB(dbPath); dlErr != nil {
				log.Warn().Err(dlErr).Msg("Failed to download GeoLite2 database, geolocation disabled")
				return &Locator{reader: nil}, nil
			}
		} else {
			log.Warn().
				Str("path", dbPath).
				Msg("GeoLite2 database not found and MAXMIND_LICENSE_KEY not set — geolocation disabled")
			return &Locator{reader: nil}, nil
		}
	}

	reader, err := geoip2.Open(dbPath)
	if err != nil {
		log.Warn().Err(err).Msg("Could not open GeoLite2 database, geolocation disabled")
		return &Locator{reader: nil}, nil
	}

	log.Info().Str("path", dbPath).Msg("GeoLite2 database loaded successfully")
	return &Locator{reader: reader}, nil
}

// Lookup busca información de geolocalización para una IP.
// Si no hay data o reader es nil, devuelve nil (sin error).
func (l *Locator) Lookup(ipStr string) *GeoInfo {
	if l == nil || l.reader == nil {
		return nil
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil
	}

	// Hacer lookup de ciudad (que incluye ASN también)
	record, err := l.reader.City(ip)
	if err != nil {
		return nil
	}

	info := &GeoInfo{
		Country: record.Country.IsoCode, // ej: "AR", "US"
		City:    record.City.Names["en"],
	}

	// Lookup de ASN por separado si es necesario (requiere otra BD)
	// Por ahora solo dejamos Country y City

	return info
}

// Close cierra la conexión a la BD.
func (l *Locator) Close() error {
	if l.reader != nil {
		return l.reader.Close()
	}
	return nil
}

// DownloadDB descarga la BD GeoLite2-City desde MaxMind.
// Requiere MAXMIND_LICENSE_KEY en la variable de entorno.
// Esta función es para setup inicial; idealmente se corre una sola vez.
func DownloadDB(dbPath string) error {
	licenseKey := os.Getenv("MAXMIND_LICENSE_KEY")
	if licenseKey == "" {
		return fmt.Errorf("MAXMIND_LICENSE_KEY environment variable not set")
	}

	// Crear directorio si no existe
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// URL de descarga
	url := fmt.Sprintf(
		"https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=%s&suffix=tar.gz",
		licenseKey,
	)

	log.Info().Str("url", url).Msg("Downloading GeoLite2-City database")

	// Descargar
	resp, err := DownloadFile(url)
	if err != nil {
		return err
	}
	defer resp.Close()

	// Extraer del tar.gz
	gz, err := gzip.NewReader(resp)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Buscar el archivo .mmdb
		if strings.HasSuffix(header.Name, "GeoLite2-City.mmdb") {
			out, err := os.Create(dbPath)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, tr); err != nil {
				return err
			}

			log.Info().Str("path", dbPath).Msg("GeoLite2-City database downloaded successfully")
			return nil
		}
	}

	return fmt.Errorf("GeoLite2-City.mmdb not found in archive")
}

// DownloadFile descarga una URL y devuelve su Body para streaming.
// El caller es responsable de cerrar el ReadCloser.
func DownloadFile(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url) //nolint:gosec // URL construida con licencia del operador
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("MaxMind respondió %d — verifica que MAXMIND_LICENSE_KEY sea válida", resp.StatusCode)
	}
	return resp.Body, nil
}
