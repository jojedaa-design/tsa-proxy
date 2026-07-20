package basicauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

type Service struct {
	repo  *postgres.BasicAuthRepository
	cache *rediscache.Cache
}

func NewService(repo *postgres.BasicAuthRepository, cache *rediscache.Cache) *Service {
	return &Service{repo: repo, cache: cache}
}

// stampBaseURL para BasicAuth es la misma URL base
var stampBaseURL = getStampBaseURL()

func getStampBaseURL() string {
	if url := os.Getenv("TSA_PUBLIC_URL"); url != "" {
		return url
	}
	return "https://tsa.bigdavi.com"
}

// Create genera nuevas credenciales Basic Auth para un tenant.
// La contraseña se genera aleatoriamente (16 bytes hex) y solo se muestra una vez.
func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, username string, name *string) (*model.CreateBasicAuthResult, error) {
	// Validar formato de username (alfanumérico, guiones, puntos)
	if len(username) < 3 || len(username) > 100 {
		return nil, fmt.Errorf("username must be between 3 and 100 characters")
	}

	// Generar contraseña: "tsp_ba_" + 16 bytes hex
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}
	password := "tsp_ba_" + hex.EncodeToString(raw)

	// key_prefix para display: "tsp_ba_a1b2****"
	keyPrefix := password[:14] + "****"

	// Hash SHA-256 para almacenar en DB
	keyHash := s.repo.HashPassword(password)

	cred := &model.BasicAuthCredential{
		TenantID:  tenantID,
		Username:  username,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Name:      name,
	}

	created, err := s.repo.Create(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("create basic auth credential: %w", err)
	}

	return &model.CreateBasicAuthResult{
		BasicAuthCredential: *created,
		Password:            password, // única vez visible
	}, nil
}

// Revoke revoca una credencial e invalida el cache.
func (s *Service) Revoke(ctx context.Context, id uuid.UUID, revokedBy uuid.UUID) error {
	cred, err := s.repo.GetByID(ctx, id)
	if err != nil || cred == nil {
		return fmt.Errorf("credential not found")
	}
	if err := s.repo.Revoke(ctx, id, revokedBy); err != nil {
		return err
	}
	// Invalidar cache
	go func() { _ = s.cache.InvalidateBasicAuth(context.Background(), cred.Username) }()
	return nil
}

// ListByTenant lista las credenciales activas de un tenant.
func (s *Service) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*model.BasicAuthCredential, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

// TSAEndpoint devuelve la URL del endpoint TSA para los clientes.
func TSAEndpoint() string {
	return stampBaseURL + "/ts"
}
