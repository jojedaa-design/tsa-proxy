package credential

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

// Service gestiona el ciclo de vida de credenciales API.
type Service struct {
	repo  *postgres.CredentialRepository
	cache *rediscache.Cache
}

func NewService(repo *postgres.CredentialRepository, cache *rediscache.Cache) *Service {
	return &Service{repo: repo, cache: cache}
}

// stampBaseURL es la base para construir las URLs de sello.
// Lee de la variable de entorno TSA_PUBLIC_URL, con fallback a producción.
var stampBaseURL = getStampBaseURL()

func getStampBaseURL() string {
	if url := os.Getenv("TSA_PUBLIC_URL"); url != "" {
		return url
	}
	return "https://tsa.bigdavi.com"
}

// Create genera un nuevo API key para el tenant.
// El API key completo solo se devuelve en este momento.
func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, name *string) (*model.CreateCredentialResult, error) {
	// Generar API key: "tsp_" + 32 bytes aleatorios hex
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	apiKey := "tsp_" + hex.EncodeToString(raw)

	// key_id: identificador público (primeros 16 chars del hex)
	keyID := "tsp_kid_" + hex.EncodeToString(raw[:8])

	// key_prefix: para mostrar en UI (ej: "tsp_a1b2****")
	keyPrefix := apiKey[:12] + "****"

	// Hash SHA-256 para almacenar en DB
	hash := sha256.Sum256([]byte(apiKey))

	// url_token: token aleatorio de 16 bytes (32 hex chars) para URL de software de firma
	tokenRaw := make([]byte, 16)
	if _, err := rand.Read(tokenRaw); err != nil {
		return nil, fmt.Errorf("generate url_token: %w", err)
	}
	urlToken := hex.EncodeToString(tokenRaw)

	cred := &model.APICredential{
		TenantID:  tenantID,
		KeyID:     keyID,
		KeyHash:   hash[:],
		KeyPrefix: keyPrefix,
		URLToken:  urlToken,
		Name:      name,
	}

	created, err := s.repo.Create(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	stampURL := stampBaseURL + "/ts/" + urlToken

	return &model.CreateCredentialResult{
		APICredential: *created,
		APIKey:        apiKey,   // única vez que se expone
		StampURL:      stampURL, // URL para software de firma (siempre visible)
	}, nil
}

// Rotate revoca la credencial existente y crea una nueva.
// Devuelve la nueva credencial con el API key visible.
func (s *Service) Rotate(ctx context.Context, credID uuid.UUID, revokedBy uuid.UUID) (*model.CreateCredentialResult, error) {
	// Obtener la credencial actual para conocer el tenant
	existing, err := s.repo.GetByID(ctx, credID)
	if err != nil {
		return nil, fmt.Errorf("get credential: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("credential not found")
	}
	if existing.Status != model.CredentialStatusActive {
		return nil, fmt.Errorf("credential is not active")
	}

	// Revocar la antigua
	if err := s.repo.Revoke(ctx, credID, revokedBy); err != nil {
		return nil, fmt.Errorf("revoke credential: %w", err)
	}

	// Invalidar cache Redis del hash antiguo
	go func() {
		_ = s.cache.InvalidateCredential(context.Background(), existing.KeyHash)
	}()

	// Crear nueva con el mismo nombre
	return s.Create(ctx, existing.TenantID, existing.Name)
}

// Revoke revoca una credencial.
func (s *Service) Revoke(ctx context.Context, credID uuid.UUID, revokedBy uuid.UUID) error {
	existing, err := s.repo.GetByID(ctx, credID)
	if err != nil {
		return fmt.Errorf("get credential: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("credential not found")
	}

	if err := s.repo.Revoke(ctx, credID, revokedBy); err != nil {
		return fmt.Errorf("revoke credential: %w", err)
	}

	// Invalidar cache Redis
	go func() {
		_ = s.cache.InvalidateCredential(context.Background(), existing.KeyHash)
	}()

	return nil
}

func (s *Service) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*model.APICredential, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}
