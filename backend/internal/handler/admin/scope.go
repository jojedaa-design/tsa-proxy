package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/bigdavi/tsa-proxy/internal/apierr"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
)

// resolveTenantScope determina a qué tenants tiene acceso el usuario autenticado
// para endpoints de solo lectura (reportes, auditoría).
//
//   - superadmin / admin: sin restricción — devuelve nil (el filtro de tenant_id
//     solicitado en la query, si lo hay, se respeta tal cual, comportamiento actual).
//   - viewer sin scope configurado: sin restricción — devuelve nil (acceso a "todos
//     los clientes").
//   - viewer con scope configurado: si requestedTenantID no es nil, debe pertenecer
//     al scope (si no, error 403); si es nil, devuelve el scope completo.
func resolveTenantScope(
	ctx context.Context,
	userRepo *postgres.AdminUserRepository,
	requestedTenantID *uuid.UUID,
) ([]uuid.UUID, *apierr.APIError) {
	roles := middleware.GetAdminRoles(ctx)
	for _, role := range roles {
		if role == "superadmin" || role == "admin" {
			return nil, nil
		}
	}

	userID, ok := middleware.GetAdminUserID(ctx)
	if !ok {
		return nil, apierr.ErrAdminUnauthorized
	}

	scope, err := userRepo.GetTenantScope(ctx, userID)
	if err != nil {
		return nil, apierr.ErrInternal
	}
	if len(scope) == 0 {
		return nil, nil
	}

	if requestedTenantID != nil {
		for _, tid := range scope {
			if tid == *requestedTenantID {
				return []uuid.UUID{*requestedTenantID}, nil
			}
		}
		return nil, apierr.ErrForbidden
	}

	return scope, nil
}
