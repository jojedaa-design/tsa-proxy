package rediscache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

// Cache implementa el cache de credenciales, allowlists y estado de tenants.
type Cache struct {
	client *redis.Client
}

func NewCache(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// ─── Credenciales ────────────────────────────────────────────

func credKey(hash []byte) string {
	return fmt.Sprintf("cred:hash:%x", hash)
}

func (c *Cache) GetCredential(ctx context.Context, hash []byte) (*model.CachedCredential, error) {
	val, err := c.client.Get(ctx, credKey(hash)).Bytes()
	if err != nil {
		return nil, err
	}
	var cred model.CachedCredential
	if err := json.Unmarshal(val, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Cache) SetCredential(ctx context.Context, hash []byte, cred *model.CachedCredential, ttl time.Duration) error {
	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, credKey(hash), data, ttl).Err()
}

func (c *Cache) InvalidateCredential(ctx context.Context, hash []byte) error {
	return c.client.Del(ctx, credKey(hash)).Err()
}

// ─── IP Allowlist ─────────────────────────────────────────────

func ipKey(tenantID uuid.UUID) string {
	return fmt.Sprintf("tenant:ip:%s", tenantID)
}

func (c *Cache) GetIPAllowlist(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	val, err := c.client.Get(ctx, ipKey(tenantID)).Bytes()
	if err != nil {
		return nil, err
	}
	var cidrs []string
	if err := json.Unmarshal(val, &cidrs); err != nil {
		return nil, err
	}
	return cidrs, nil
}

func (c *Cache) SetIPAllowlist(ctx context.Context, tenantID uuid.UUID, cidrs []string, ttlSeconds int) error {
	data, err := json.Marshal(cidrs)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, ipKey(tenantID), data, time.Duration(ttlSeconds)*time.Second).Err()
}

func (c *Cache) InvalidateIPAllowlist(ctx context.Context, tenantID uuid.UUID) error {
	return c.client.Del(ctx, ipKey(tenantID)).Err()
}

// ─── Estado de tenant ─────────────────────────────────────────

func tenantStatusKey(tenantID uuid.UUID) string {
	return fmt.Sprintf("tenant:status:%s", tenantID)
}

func (c *Cache) InvalidateTenantStatus(ctx context.Context, tenantID uuid.UUID) error {
	return c.client.Del(ctx, tenantStatusKey(tenantID)).Err()
}

// ─── Credencial por URL token ─────────────────────────────

type CachedTokenCredential struct {
	TenantID     string  `json:"tenant_id"`
	TenantStatus string  `json:"tenant_status"`
	CredID       string  `json:"cred_id"`
	ExpiresAt    *string `json:"expires_at,omitempty"`
}

func credTokenKey(token string) string {
	return fmt.Sprintf("cred:token:%s", token)
}

func (c *Cache) GetCredentialByToken(ctx context.Context, token string) (*CachedTokenCredential, error) {
	val, err := c.client.Get(ctx, credTokenKey(token)).Bytes()
	if err != nil {
		return nil, err
	}
	var cred CachedTokenCredential
	if err := json.Unmarshal(val, &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Cache) SetCredentialByToken(ctx context.Context, token string, cred *CachedTokenCredential, ttl time.Duration) error {
	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, credTokenKey(token), data, ttl).Err()
}

func (c *Cache) InvalidateCredentialByToken(ctx context.Context, token string) error {
	return c.client.Del(ctx, credTokenKey(token)).Err()
}

// ─── Upstream default ─────────────────────────────────────

func upstreamDefaultKey() string {
	return "upstream:default"
}

func (c *Cache) GetDefaultUpstream(ctx context.Context) (*model.TSAUpstream, error) {
	val, err := c.client.Get(ctx, upstreamDefaultKey()).Bytes()
	if err != nil {
		return nil, err
	}
	var u model.TSAUpstream
	if err := json.Unmarshal(val, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Cache) SetDefaultUpstream(ctx context.Context, u *model.TSAUpstream, ttl time.Duration) error {
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, upstreamDefaultKey(), data, ttl).Err()
}

func (c *Cache) InvalidateDefaultUpstream(ctx context.Context) error {
	return c.client.Del(ctx, upstreamDefaultKey()).Err()
}

// ─── Quota por tenant ─────────────────────────────────────

func tenantQuotaKey(tenantID uuid.UUID) string {
	return fmt.Sprintf("tenant:quota:%s", tenantID)
}

func (c *Cache) GetTenantQuota(ctx context.Context, tenantID uuid.UUID) (*model.TenantQuota, error) {
	val, err := c.client.Get(ctx, tenantQuotaKey(tenantID)).Bytes()
	if err != nil {
		return nil, err
	}
	var q model.TenantQuota
	if err := json.Unmarshal(val, &q); err != nil {
		return nil, err
	}
	return &q, nil
}

func (c *Cache) SetTenantQuota(ctx context.Context, tenantID uuid.UUID, q *model.TenantQuota, ttl time.Duration) error {
	data, err := json.Marshal(q)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, tenantQuotaKey(tenantID), data, ttl).Err()
}

func (c *Cache) InvalidateTenantQuota(ctx context.Context, tenantID uuid.UUID) error {
	return c.client.Del(ctx, tenantQuotaKey(tenantID)).Err()
}

// ─── Basic Auth Credentials ───────────────────────────────────

type CachedBasicAuth struct {
	CredID       string `json:"cred_id"`
	TenantID     string `json:"tenant_id"`
	TenantStatus string `json:"tenant_status"`
	KeyHash      string `json:"key_hash"` // hex del SHA-256
	CredStatus   string `json:"cred_status"`
}

func basicAuthKey(username string) string {
	return fmt.Sprintf("basicauth:user:%s", username)
}

func (c *Cache) GetBasicAuth(ctx context.Context, username string) (*CachedBasicAuth, error) {
	val, err := c.client.Get(ctx, basicAuthKey(username)).Bytes()
	if err != nil {
		return nil, err
	}
	var ba CachedBasicAuth
	if err := json.Unmarshal(val, &ba); err != nil {
		return nil, err
	}
	return &ba, nil
}

func (c *Cache) SetBasicAuth(ctx context.Context, username string, ba *CachedBasicAuth, ttl time.Duration) error {
	data, err := json.Marshal(ba)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, basicAuthKey(username), data, ttl).Err()
}

func (c *Cache) InvalidateBasicAuth(ctx context.Context, username string) error {
	return c.client.Del(ctx, basicAuthKey(username)).Err()
}
