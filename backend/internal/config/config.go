package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contiene toda la configuración de la aplicación
// cargada desde variables de entorno.
type Config struct {
	Environment string
	LogLevel    string
	Version     string

	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Upstream UpstreamConfig
	Proxy    ProxyConfig
	RateLimit RateLimitConfig
}

type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type PostgresConfig struct {
	Host            string
	Port            string
	DB              string
	User            string
	Password        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d",
		c.Host, c.Port, c.DB, c.User, c.Password, c.SSLMode, c.MaxOpenConns,
	)
}

type RedisConfig struct {
	Host         string
	Port         string
	Password     string
	DB           int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
}

func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type UpstreamConfig struct {
	URL          string
	Username     string
	Password     string
	Timeout      time.Duration
	MaxRetries   int
	MaxBodyBytes int64
	// AllowedHosts es la lista blanca de hostnames permitidos como upstream TSA.
	// Se lee de TSA_UPSTREAM_ALLOWLIST (coma-separada). Si está vacía, se permite
	// cualquier hostname público (solo se bloquean IPs privadas/loopback).
	// Ejemplo: "timestamp.digicert.com,tsuq.camerfirma.com"
	AllowedHosts []string
}

type ProxyConfig struct {
	MaxRequestBody  int64
	UsageRecordAsync bool
}

type RateLimitConfig struct {
	GlobalRPS   int
	BurstWindow int // seconds
}

// Load carga la configuración desde variables de entorno.
// Falla con error detallado si faltan variables críticas.
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.Environment = getEnv("ENVIRONMENT", "production")
	cfg.LogLevel = getEnv("LOG_LEVEL", "info")
	cfg.Version = getEnv("APP_VERSION", "1.0.0")

	// Server
	cfg.Server = ServerConfig{
		Port:            getEnv("BACKEND_PORT", "8080"),
		ReadTimeout:     getDuration("BACKEND_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:    getDuration("BACKEND_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     getDuration("BACKEND_IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout: getDuration("BACKEND_SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	// Postgres
	cfg.Postgres = PostgresConfig{
		Host:            getEnv("POSTGRES_HOST", "localhost"),
		Port:            getEnv("POSTGRES_PORT", "5432"),
		DB:              getEnv("POSTGRES_DB", "tsaproxy"),
		User:            getEnv("POSTGRES_USER", "tsaproxy"),
		Password:        mustEnv("POSTGRES_PASSWORD"),
		SSLMode:         getEnv("POSTGRES_SSL_MODE", "disable"),
		MaxOpenConns:    getInt("POSTGRES_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getInt("POSTGRES_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getDuration("POSTGRES_CONN_MAX_LIFETIME", 5*time.Minute),
	}

	// Redis
	cfg.Redis = RedisConfig{
		Host:         getEnv("REDIS_HOST", "localhost"),
		Port:         getEnv("REDIS_PORT", "6379"),
		Password:     mustEnv("REDIS_PASSWORD"),
		DB:           getInt("REDIS_DB", 0),
		MaxRetries:   getInt("REDIS_MAX_RETRIES", 3),
		DialTimeout:  getDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
		ReadTimeout:  getDuration("REDIS_READ_TIMEOUT", 3*time.Second),
		WriteTimeout: getDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),
		PoolSize:     getInt("REDIS_POOL_SIZE", 10),
	}

	// JWT
	cfg.JWT = JWTConfig{
		Secret:     mustEnv("JWT_SECRET"),
		AccessTTL:  getDuration("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL: getDuration("JWT_REFRESH_TTL", 168*time.Hour),
	}

	// Upstream TSA — optional, service reads from DB with .env as fallback
	cfg.Upstream = UpstreamConfig{
		URL:          getEnv("TSA_UPSTREAM_URL", ""),
		Username:     getEnv("TSA_UPSTREAM_USERNAME", ""),
		Password:     getEnv("TSA_UPSTREAM_PASSWORD", ""),
		Timeout:      getDuration("TSA_UPSTREAM_TIMEOUT", 10*time.Second),
		MaxRetries:   getInt("TSA_UPSTREAM_MAX_RETRIES", 1),
		MaxBodyBytes: int64(getInt("TSA_UPSTREAM_MAX_BODY_BYTES", 32768)),
		AllowedHosts: parseAllowedHosts(getEnv("TSA_UPSTREAM_ALLOWLIST", "")),
	}

	// Proxy
	cfg.Proxy = ProxyConfig{
		MaxRequestBody:  int64(getInt("PROXY_MAX_REQUEST_BODY", 16384)),
		UsageRecordAsync: getBool("PROXY_USAGE_RECORD_ASYNC", true),
	}

	// Rate limiting
	cfg.RateLimit = RateLimitConfig{
		GlobalRPS:   getInt("RATE_LIMIT_GLOBAL_RPS", 200),
		BurstWindow: getInt("RATE_LIMIT_BURST_WINDOW", 60),
	}

	return cfg, nil
}

// ─── helpers ────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("variable de entorno requerida no configurada: %s", key))
	}
	return v
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// parseAllowedHosts parsea una lista de hostnames separados por comas.
// Retorna nil si la cadena está vacía (sin restricción de lista blanca).
func parseAllowedHosts(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
