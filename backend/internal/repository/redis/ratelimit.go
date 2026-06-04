package rediscache

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

// RateLimiter implementa rate limiting en Redis.
type RateLimiter struct {
	client     *redis.Client
	failedRepo interface {
		Insert(context.Context, *model.FailedRequest) error
	}
}

func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

func (r *RateLimiter) WithFailedRepo(repo interface {
	Insert(context.Context, *model.FailedRequest) error
}) *RateLimiter {
	r.failedRepo = repo
	return r
}

func (r *RateLimiter) FailedRepo() interface {
	Insert(context.Context, *model.FailedRequest) error
} {
	return r.failedRepo
}

// CheckGlobal verifica el rate limit global del sistema.
// Usa una ventana fija de 1 segundo.
// Devuelve: allowed, remaining, error.
func (r *RateLimiter) CheckGlobal(ctx context.Context, maxRPS int) (bool, int, error) {
	key := fmt.Sprintf("rl:global:%d", time.Now().Unix())

	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Second) // ventana de 2s para evitar race condition
	if _, err := pipe.Exec(ctx); err != nil {
		return true, maxRPS, err // fail-open
	}

	count := int(incr.Val())
	remaining := maxRPS - count
	if remaining < 0 {
		remaining = 0
	}
	return count <= maxRPS, remaining, nil
}

// CheckTenant verifica el burst limit del tenant.
// Ventana de 60 segundos (burst_per_minute).
// Devuelve: allowed, remaining, error.
func (r *RateLimiter) CheckTenant(ctx context.Context, tenantID uuid.UUID, burstLimit int) (bool, int, error) {
	key := fmt.Sprintf("rl:burst:%s", tenantID)

	// Lua script para INCR + EXPIRE atómico
	script := redis.NewScript(`
		local current = redis.call('INCR', KEYS[1])
		if current == 1 then
			redis.call('EXPIRE', KEYS[1], 60)
		end
		return current
	`)

	result, err := script.Run(ctx, r.client, []string{key}).Int()
	if err != nil {
		return true, burstLimit, err // fail-open
	}

	remaining := burstLimit - result
	if remaining < 0 {
		remaining = 0
	}
	return result <= burstLimit, remaining, nil
}

// ─── Quota ────────────────────────────────────────────────────

func quotaKey(tenantID uuid.UUID, year, month int) string {
	return fmt.Sprintf("quota:month:%s:%04d-%02d", tenantID, year, month)
}

// IncrQuota incrementa el contador de cuota mensual.
// Devuelve el nuevo valor (después del incremento).
func (r *RateLimiter) IncrQuota(ctx context.Context, tenantID uuid.UUID, year, month int, ttlSeconds int) (int64, error) {
	key := quotaKey(tenantID, year, month)
	script := redis.NewScript(`
		local current = redis.call('INCR', KEYS[1])
		if current == 1 then
			redis.call('EXPIRE', KEYS[1], ARGV[1])
		end
		return current
	`)
	val, err := script.Run(ctx, r.client, []string{key}, ttlSeconds).Int64()
	return val, err
}

// DecrQuota revierte un INCR en caso de que la cuota esté excedida.
func (r *RateLimiter) DecrQuota(ctx context.Context, tenantID uuid.UUID, year, month int) error {
	key := quotaKey(tenantID, year, month)
	return r.client.Decr(ctx, key).Err()
}

// GetQuota obtiene el valor actual del contador de cuota.
func (r *RateLimiter) GetQuota(ctx context.Context, tenantID uuid.UUID, year, month int) (int64, error) {
	key := quotaKey(tenantID, year, month)
	val, err := r.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// SetQuota establece el valor base del contador (cargado desde Postgres).
func (r *RateLimiter) SetQuota(ctx context.Context, tenantID uuid.UUID, year, month int, value int64, ttlSeconds int) error {
	key := quotaKey(tenantID, year, month)
	return r.client.Set(ctx, key, value, time.Duration(ttlSeconds)*time.Second).Err()
}
