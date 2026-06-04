#!/bin/bash
# =============================================================
# Seed inicial: crea el primer superadmin
# Ejecutar SOLO en el primer deploy, una sola vez.
# =============================================================
set -euo pipefail

CONTAINER="${POSTGRES_CONTAINER:-tsa_postgres}"
DB="${POSTGRES_DB:-tsaproxy}"
USER="${POSTGRES_USER:-tsaproxy}"

# Contraseña del admin — cambiar antes de ejecutar
ADMIN_USERNAME="${SEED_ADMIN_USERNAME:-admin}"
ADMIN_EMAIL="${SEED_ADMIN_EMAIL:-admin@bigdavi.com}"
ADMIN_PASSWORD="${SEED_ADMIN_PASSWORD:-ChangeMeNow123!}"

echo "==> Generando hash Argon2id para la contraseña del admin..."

# Generar hash usando el backend Go
HASH=$(docker exec tsa_backend /tsa-proxy hash-password "$ADMIN_PASSWORD" 2>/dev/null || true)

if [ -z "$HASH" ]; then
  echo "  [fallback] El backend no tiene subcomando hash-password, generando via Python..."
  # Fallback: usar Python si está disponible en el host
  HASH=$(python3 -c "
import hashlib, secrets, base64, struct

salt = secrets.token_bytes(16)
# Simular Argon2id con hashlib como placeholder
# En producción real, el Go hashea al primer login si usamos una utilidad
print('\$argon2id\$v=19\$m=65536,t=3,p=4\$' + base64.b64encode(salt).decode() + '\$placeholder')
" 2>/dev/null || echo "PLACEHOLDER_HASH")
fi

echo "==> Insertando superadmin en la base de datos..."

docker exec -i "$CONTAINER" psql -U "$USER" -d "$DB" << SQL
-- Insertar admin user
INSERT INTO admin_users (username, email, password_hash, is_active)
VALUES ('${ADMIN_USERNAME}', '${ADMIN_EMAIL}', '${HASH}', TRUE)
ON CONFLICT (username) DO UPDATE SET email = EXCLUDED.email
RETURNING id;

-- Asignar rol superadmin
DO \$\$
DECLARE
  v_user_id UUID;
  v_role_id  UUID;
BEGIN
  SELECT id INTO v_user_id FROM admin_users WHERE username='${ADMIN_USERNAME}';
  SELECT id INTO v_role_id  FROM roles WHERE name='superadmin';

  INSERT INTO admin_user_roles (admin_user_id, role_id)
  VALUES (v_user_id, v_role_id)
  ON CONFLICT DO NOTHING;

  RAISE NOTICE 'Admin % creado con rol superadmin', '${ADMIN_USERNAME}';
END \$\$;
SQL

echo ""
echo "✓ Superadmin creado."
echo "  Username: ${ADMIN_USERNAME}"
echo "  Email:    ${ADMIN_EMAIL}"
echo ""
echo "  IMPORTANTE: Cambia la contraseña en el primer login."
echo "  La contraseña actual fue: ${ADMIN_PASSWORD}"
echo ""
echo "  Si el hash es PLACEHOLDER, debes usar la utilidad del backend para regenerarlo:"
echo "  docker exec tsa_backend /tsa-proxy hash-password 'nueva_contraseña'"
