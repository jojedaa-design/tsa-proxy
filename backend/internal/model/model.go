// Package model define todos los tipos de dominio del sistema.
package model

import (
	"time"

	"github.com/google/uuid"
)

// ─── Admin ────────────────────────────────────────────────────

type AdminUser struct {
	ID           uuid.UUID  `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	IsActive     bool       `json:"is_active"`
	TOTPSecret   *string    `json:"-"`
	TOTPEnabled  bool       `json:"totp_enabled"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP  *string    `json:"last_login_ip,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Roles        []string   `json:"roles,omitempty"`
}

type Role struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
}

// ─── Tenant ───────────────────────────────────────────────────

type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDeleted   TenantStatus = "deleted"
)

type Tenant struct {
	ID           uuid.UUID    `json:"id"`
	Name         string       `json:"name"`
	Slug         string       `json:"slug"`
	Description  *string      `json:"description,omitempty"`
	Status       TenantStatus `json:"status"`
	ContactEmail *string      `json:"contact_email,omitempty"`
	CreatedBy    *uuid.UUID   `json:"created_by,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	// Agregados opcionales
	Quota *TenantQuota `json:"quota,omitempty"`
}

// ─── API Credentials ─────────────────────────────────────────

type CredentialStatus string

const (
	CredentialStatusActive  CredentialStatus = "active"
	CredentialStatusRevoked CredentialStatus = "revoked"
	CredentialStatusExpired CredentialStatus = "expired"
)

type APICredential struct {
	ID         uuid.UUID        `json:"id"`
	TenantID   uuid.UUID        `json:"tenant_id"`
	KeyID      string           `json:"key_id"`
	KeyHash    []byte           `json:"-"`
	KeyPrefix  string           `json:"key_prefix"`
	URLToken   string           `json:"url_token"` // token para URL de software de firma
	Name       *string          `json:"name,omitempty"`
	Status     CredentialStatus `json:"status"`
	ExpiresAt  *time.Time       `json:"expires_at,omitempty"`
	LastUsedAt *time.Time       `json:"last_used_at,omitempty"`
	LastUsedIP *string          `json:"last_used_ip,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	RevokedAt  *time.Time       `json:"revoked_at,omitempty"`
	RevokedBy  *uuid.UUID       `json:"revoked_by,omitempty"`
}

// CreateCredentialResult se devuelve SOLO al crear/rotar una credencial.
// El APIKey completo solo se expone en este momento.
type CreateCredentialResult struct {
	APICredential
	APIKey  string `json:"api_key"`  // solo visible en creación/rotación
	StampURL string `json:"stamp_url"` // URL lista para copiar en software de firma
}

// ─── IP Allowlist ─────────────────────────────────────────────

type IPAllowlistEntry struct {
	ID        uuid.UUID  `json:"id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	CIDR      string     `json:"cidr"`
	Label     *string    `json:"label,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
}

// ─── Plans & Quotas ───────────────────────────────────────────

type Plan struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Description   *string   `json:"description,omitempty"`
	MonthlyLimit  int       `json:"monthly_limit"`
	BurstPerMinute int      `json:"burst_per_minute"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type TenantQuota struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	PlanID         *uuid.UUID `json:"plan_id,omitempty"`
	MonthlyLimit   int        `json:"monthly_limit"`
	BurstPerMinute int        `json:"burst_per_minute"`
	HardLimit      bool       `json:"hard_limit"`
	AutoSuspend    bool       `json:"auto_suspend"`
	ResetDay       int        `json:"reset_day"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ─── TSA Upstream ─────────────────────────────────────────────

type TSAUpstream struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	EnvKeyUser  *string   `json:"env_key_user,omitempty"`
	EnvKeyPass  *string   `json:"env_key_pass,omitempty"`
	Username    *string   `json:"username,omitempty"`
	Password    *string   `json:"password,omitempty"`
	TimeoutMs   int       `json:"timeout_ms"`
	MaxRetries  int       `json:"max_retries"`
	IsActive    bool      `json:"is_active"`
	IsDefault   bool      `json:"is_default"`
	HasPassword bool      `json:"has_password"` // campo calculado, no en BD
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ─── Usage ────────────────────────────────────────────────────

type UsageStatus string

const (
	UsageStatusSuccess  UsageStatus = "success"
	UsageStatusError    UsageStatus = "error"
	UsageStatusRejected UsageStatus = "rejected"
)

type UsageEvent struct {
	ID                 uuid.UUID   `json:"id"`
	TenantID           uuid.UUID   `json:"tenant_id"`
	CredentialID       *uuid.UUID  `json:"credential_id,omitempty"`
	RequestID          string      `json:"request_id"`
	SourceIP           string      `json:"source_ip"`
	Status             UsageStatus `json:"status"`
	RejectionReason    *string     `json:"rejection_reason,omitempty"`
	UpstreamStatus     *int        `json:"upstream_status,omitempty"`
	LatencyMs          *int        `json:"latency_ms,omitempty"`
	UpstreamLatencyMs  *int        `json:"upstream_latency_ms,omitempty"`
	RequestSizeBytes   *int        `json:"request_size_bytes,omitempty"`
	ResponseSizeBytes  *int        `json:"response_size_bytes,omitempty"`
	TSAUpstreamID      *uuid.UUID  `json:"tsa_upstream_id,omitempty"`
	UserAgent          *string     `json:"user_agent,omitempty"`
	GeoCountry         *string     `json:"geo_country,omitempty"`
	GeoCity            *string     `json:"geo_city,omitempty"`
	GeoASN             *string     `json:"geo_asn,omitempty"`
	ExceedsQuota       bool        `json:"exceeds_quota"`
	OccurredAt         time.Time   `json:"occurred_at"`
}

type MonthlyAggregate struct {
	TenantID           uuid.UUID `json:"tenant_id"`
	TenantName         string    `json:"tenant_name,omitempty"`
	Year               int       `json:"year"`
	Month              int       `json:"month"`
	TotalRequests      int       `json:"total_requests"`
	SuccessfulRequests int       `json:"successful_requests"`
	FailedRequests     int       `json:"failed_requests"`
	RejectedRequests   int       `json:"rejected_requests"`
	TotalLatencyMs     int64     `json:"total_latency_ms"`
	AvgLatencyMs       float64   `json:"avg_latency_ms"`
	MonthlyLimit       *int      `json:"monthly_limit,omitempty"`
	ExceededRequests   *int      `json:"exceeded_requests,omitempty"`
	LastUpdatedAt      time.Time `json:"last_updated_at"`
}

type FailedRequest struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        *uuid.UUID `json:"tenant_id,omitempty"`
	CredentialKeyID *string    `json:"credential_key_id,omitempty"`
	SourceIP        string     `json:"source_ip"`
	FailureReason   string     `json:"failure_reason"`
	RequestID       *string    `json:"request_id,omitempty"`
	OccurredAt      time.Time  `json:"occurred_at"`
}

// ─── Audit ────────────────────────────────────────────────────

type AuditEvent struct {
	ID         uuid.UUID              `json:"id"`
	ActorID    *uuid.UUID             `json:"actor_id,omitempty"`
	ActorType  string                 `json:"actor_type"`
	ActorName  string                 `json:"actor_name,omitempty"`
	Action     string                 `json:"action"`
	EntityType string                 `json:"entity_type"`
	EntityID   *uuid.UUID             `json:"entity_id,omitempty"`
	Changes    map[string]interface{} `json:"changes,omitempty"`
	IPAddress  *string                `json:"ip_address,omitempty"`
	UserAgent  *string                `json:"user_agent,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
	TenantID   *uuid.UUID             `json:"tenant_id,omitempty"`
}

// ─── Pagination ───────────────────────────────────────────────

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func NewPagination(page, limit, total int) Pagination {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 20
	}
	totalPages := total / limit
	if total%limit != 0 {
		totalPages++
	}
	return Pagination{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
}

func (p Pagination) Offset() int {
	return (p.Page - 1) * p.Limit
}

// ─── Context keys ─────────────────────────────────────────────

type contextKey string

const (
	CtxKeyRequestID    contextKey = "request_id"
	CtxKeyTenantID     contextKey = "tenant_id"
	CtxKeyCredentialID contextKey = "credential_id"
	CtxKeyBurstLimit   contextKey = "burst_limit"
	CtxKeyAdminUser    contextKey = "admin_user"
	CtxKeyAdminRoles   contextKey = "admin_roles"
	CtxKeyClientIP     contextKey = "client_ip"
)

// ─── Cache types ──────────────────────────────────────────────

// CachedCredential es lo que se guarda en Redis para lookups rápidos.
type CachedCredential struct {
	CredentialID   uuid.UUID
	TenantID       uuid.UUID
	TenantStatus   TenantStatus
	BurstPerMinute int
}

// ─── HTTP Basic Auth Credentials ─────────────────────────────

type BasicAuthCredential struct {
	ID        uuid.UUID        `json:"id"`
	TenantID  uuid.UUID        `json:"tenant_id"`
	Username  string           `json:"username"`
	KeyHash   []byte           `json:"-"`
	KeyPrefix string           `json:"key_prefix"`
	Name      *string          `json:"name,omitempty"`
	Status    CredentialStatus `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	RevokedAt *time.Time       `json:"revoked_at,omitempty"`
	RevokedBy *uuid.UUID       `json:"revoked_by,omitempty"`
}

type CreateBasicAuthResult struct {
	BasicAuthCredential
	Password string `json:"password"` // solo visible en creación/rotación
}

// ─── No-Auth Access (acceso por IP sin contraseña) ────────────

type NoAuthAccessStatus string

const (
	NoAuthAccessStatusActive    NoAuthAccessStatus = "active"
	NoAuthAccessStatusSuspended NoAuthAccessStatus = "suspended"
)

type NoAuthAccess struct {
	ID        uuid.UUID          `json:"id"`
	TenantID  uuid.UUID          `json:"tenant_id"`
	Name      *string            `json:"name,omitempty"`
	Status    NoAuthAccessStatus `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}
