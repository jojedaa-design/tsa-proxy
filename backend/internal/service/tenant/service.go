package tenant

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
	rediscache "github.com/bigdavi/tsa-proxy/internal/repository/redis"
)

// Service gestiona operaciones de tenants.
type Service struct {
	repo  *postgres.TenantRepository
	cache *rediscache.Cache
	audit *postgres.AuditRepository
}

func NewService(
	repo *postgres.TenantRepository,
	cache *rediscache.Cache,
	audit *postgres.AuditRepository,
) *Service {
	return &Service{repo: repo, cache: cache, audit: audit}
}

// CreateRequest define los datos para crear un tenant.
type CreateRequest struct {
	Name         string
	Slug         string
	Description  *string
	ContactEmail *string
	CreatedBy    uuid.UUID
}

func (s *Service) Create(ctx context.Context, req *CreateRequest) (*model.Tenant, error) {
	// Validar slug
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if slug == "" {
		slug = slugify(req.Name)
	}

	// Verificar unicidad del slug
	existing, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("check slug: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("slug '%s' already exists", slug)
	}

	createdBy := req.CreatedBy
	t := &model.Tenant{
		Name:         req.Name,
		Slug:         slug,
		Description:  req.Description,
		ContactEmail: req.ContactEmail,
		CreatedBy:    &createdBy,
	}

	created, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	// Auditoría
	go func() {
		_ = s.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &createdBy,
			ActorType:  "admin",
			Action:     "tenant.create",
			EntityType: "tenant",
			EntityID:   &created.ID,
			Changes:    map[string]interface{}{"name": created.Name, "slug": created.Slug},
		})
	}()

	return created, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, actorID uuid.UUID, t *model.Tenant) (*model.Tenant, error) {
	before, _ := s.repo.GetByID(ctx, id)

	t.ID = id
	updated, err := s.repo.Update(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("update tenant: %w", err)
	}
	if updated == nil {
		return nil, fmt.Errorf("tenant not found")
	}

	go func() {
		changes := map[string]interface{}{}
		if before != nil {
			changes["before"] = map[string]interface{}{"name": before.Name, "contact_email": before.ContactEmail}
			changes["after"]  = map[string]interface{}{"name": updated.Name, "contact_email": updated.ContactEmail}
		}
		_ = s.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "tenant.update",
			EntityType: "tenant",
			EntityID:   &id,
			Changes:    changes,
		})
	}()

	return updated, nil
}

func (s *Service) Suspend(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	if err := s.repo.SetStatus(ctx, id, model.TenantStatusSuspended); err != nil {
		return fmt.Errorf("suspend tenant: %w", err)
	}

	// Invalidar caches
	_ = s.cache.InvalidateTenantStatus(ctx, id)
	_ = s.cache.InvalidateIPAllowlist(ctx, id)

	go func() {
		_ = s.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "tenant.suspend",
			EntityType: "tenant",
			EntityID:   &id,
			Changes:    map[string]interface{}{"before": map[string]string{"status": "active"}, "after": map[string]string{"status": "suspended"}},
		})
	}()

	return nil
}

func (s *Service) Reactivate(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	if err := s.repo.SetStatus(ctx, id, model.TenantStatusActive); err != nil {
		return fmt.Errorf("reactivate tenant: %w", err)
	}

	_ = s.cache.InvalidateTenantStatus(ctx, id)

	go func() {
		_ = s.audit.Insert(context.Background(), &model.AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			Action:     "tenant.reactivate",
			EntityType: "tenant",
			EntityID:   &id,
			Changes:    map[string]interface{}{"before": map[string]string{"status": "suspended"}, "after": map[string]string{"status": "active"}},
		})
	}()

	return nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*model.Tenant, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f postgres.TenantFilter) ([]*model.Tenant, int, error) {
	return s.repo.List(ctx, f)
}

// slugify convierte un nombre en un slug URL-safe.
func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, slug)
	// Reducir múltiples guiones
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}
