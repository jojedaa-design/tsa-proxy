#!/bin/bash
# =============================================================
# Ejecuta las migraciones SQL en orden
# Se llama una vez en el primer deploy y después de updates
# =============================================================
set -euo pipefail

CONTAINER="${POSTGRES_CONTAINER:-tsa_postgres}"
DB="${POSTGRES_DB:-tsaproxy}"
USER="${POSTGRES_USER:-tsaproxy}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-./backend/migrations}"

echo "[$(date -Iseconds)] Ejecutando migraciones desde: ${MIGRATIONS_DIR}"

for f in $(ls "${MIGRATIONS_DIR}"/*.up.sql 2>/dev/null | sort); do
  echo "  → Aplicando: $(basename $f)"
  docker exec -i "$CONTAINER" psql -U "$USER" -d "$DB" < "$f"
  echo "    ✓ OK"
done

echo "[$(date -Iseconds)] Migraciones completadas."
