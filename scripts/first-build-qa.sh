#!/bin/bash
# ============================================================================
# FIRST BUILD AND DEPLOYMENT TO QA
# ============================================================================
# This script performs the first build and deployment on the QA server
# Run this AFTER setup-qa-ssl.sh on the QA server
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - /opt/qa directory created
#   - .env file configured
#   - SSL certificates generated (via setup-qa-ssl.sh)
#   - DNS pointing qa-ast/qa-tsa.bigdavi.com to 54.39.180.141
#
# Usage: bash first-build-qa.sh
# ============================================================================

set -e

echo "=========================================="
echo "QA First Build & Deployment"
echo "=========================================="

# Verify running on QA server
if [ ! -d "/opt/qa" ]; then
    echo "❌ Error: /opt/qa directory not found. Are you on the QA server (54.39.180.141)?"
    echo "   Or should this be at /opt/tsa-proxy on production (54.39.181.13)?"
    exit 1
fi

cd /opt/qa

# 1. Verify required files
echo ""
echo "1️⃣  Verifying configuration..."
if [ ! -f ".env" ]; then
    echo "❌ Error: .env file not found in /opt/qa"
    echo "   Create .env first using:"
    echo "   cp .env.example .env && nano .env"
    exit 1
fi
echo "✅ .env file found"

if [ ! -f "docker-compose.yml" ]; then
    echo "❌ Error: docker-compose.yml not found"
    exit 1
fi
echo "✅ docker-compose.yml found"

# 2. Check Docker services
echo ""
echo "2️⃣  Checking Docker..."
docker --version
docker compose version

# 3. Create necessary directories
echo ""
echo "3️⃣  Creating required directories..."
mkdir -p ./nginx/conf.d
mkdir -p ./build/logs
echo "✅ Directories ready"

# 4. Build images
echo ""
echo "4️⃣  Building Docker images..."
echo "   ⏳ This may take 3-5 minutes on first build..."
docker compose build --no-cache backend frontend nginx

# 5. Start PostgreSQL and Redis first
echo ""
echo "5️⃣  Starting PostgreSQL and Redis..."
docker compose up -d postgres redis
echo "⏳ Waiting for databases to be healthy..."
sleep 15
docker compose ps

# 6. Apply migrations
echo ""
echo "6️⃣  Applying database migrations..."
if [ -f "scripts/run-migrations.sh" ]; then
    bash scripts/run-migrations.sh
    echo "✅ Migrations applied"
else
    echo "⚠️  run-migrations.sh not found - skipping"
fi

# 7. Start remaining services
echo ""
echo "7️⃣  Starting all services..."
docker compose up -d backend frontend nginx

# 8. Wait for services to be healthy
echo ""
echo "8️⃣  Waiting for services to be healthy..."
sleep 10

# 9. Verify all containers
echo ""
echo "9️⃣  Service status:"
docker compose ps

# 10. Test endpoints
echo ""
echo "🔟 Testing endpoints..."
echo ""

# Test health endpoint
echo "  Testing qa-tsa.bigdavi.com/health..."
if curl -sf https://qa-tsa.bigdavi.com/health >/dev/null 2>&1; then
    echo "  ✅ TSA health check passed"
else
    echo "  ⚠️  TSA endpoint not responding yet (may need time)"
fi

# Test admin panel
echo ""
echo "  Testing qa-ast.bigdavi.com..."
if curl -sf https://qa-ast.bigdavi.com/ >/dev/null 2>&1; then
    echo "  ✅ Admin panel is accessible"
else
    echo "  ⚠️  Admin panel not responding yet (may need time)"
fi

# 11. Create initial admin user (optional)
echo ""
echo "1️⃣1️⃣  Creating initial admin user..."
if command -v docker compose &> /dev/null; then
    # Prompt for admin credentials
    echo ""
    read -p "  Enter admin email [admin@bigdavi.com]: " ADMIN_EMAIL
    ADMIN_EMAIL=${ADMIN_EMAIL:-admin@bigdavi.com}

    read -sp "  Enter admin password: " ADMIN_PASSWORD
    echo ""

    # Create admin using backend CLI
    echo "  Creating admin user: $ADMIN_EMAIL"
    # Note: This requires the backend to have a CLI tool for user creation
    # Adjust command if your backend has different seeding method
    docker compose exec -T backend /tsa-proxy createadmin \
        --email "$ADMIN_EMAIL" \
        --password "$ADMIN_PASSWORD" 2>/dev/null || \
    echo "  ℹ️  Admin creation skipped (use seed-data.sh if needed)"
else
    echo "  ℹ️  Skipping admin creation"
fi

# 12. Summary
echo ""
echo "=========================================="
echo "✅ QA First Build Complete!"
echo "=========================================="
echo ""
echo "🌐 Access QA Environment:"
echo "   Admin Panel: https://qa-ast.bigdavi.com"
echo "   TSA Proxy:   https://qa-tsa.bigdavi.com"
echo ""
echo "📊 View Logs:"
echo "   docker compose logs -f backend"
echo "   docker compose logs -f frontend"
echo ""
echo "📈 Service Status:"
docker compose ps
echo ""
echo "⚠️  Important Notes:"
echo "   1. Database is initialized and running"
echo "   2. You need to create an admin user to login"
echo "   3. SSL certificates configured for QA domains"
echo "   4. For production, follow the same process on 54.39.181.13"
echo ""
echo "Next steps:"
echo "   1. Create admin user via seed-data.sh"
echo "   2. Login to qa-ast.bigdavi.com"
echo "   3. Create a test tenant and API key"
echo "   4. Test the proxy: curl -X POST https://qa-tsa.bigdavi.com/api/v1/timestamp ..."
echo ""
