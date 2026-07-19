#!/bin/bash
# ============================================================================
# SETUP SSL CERTIFICATES FOR QA ENVIRONMENT
# ============================================================================
# This script configures Let's Encrypt SSL certificates for the QA server
# Run this ONCE on the QA server (54.39.180.141) after Nginx is installed
#
# Usage: bash setup-qa-ssl.sh
# ============================================================================

set -e

echo "=========================================="
echo "QA SSL Certificate Setup"
echo "=========================================="

# Verify running on QA server
if [ ! -d "/opt/qa" ]; then
    echo "❌ Error: /opt/qa directory not found. Are you on the QA server?"
    exit 1
fi

cd /opt/qa

# 1. Install Certbot if not already installed
echo ""
echo "1️⃣  Checking Certbot installation..."
if ! command -v certbot &> /dev/null; then
    echo "📦 Installing Certbot..."
    sudo apt-get update
    sudo apt-get install -y certbot python3-certbot-nginx
else
    echo "✅ Certbot already installed"
fi

# 2. Get email for Let's Encrypt notifications
CERT_EMAIL=${CERT_EMAIL:-"admin@bigdavi.com"}

# 3. Stop Nginx temporarily (Certbot needs port 80 and 443)
echo ""
echo "2️⃣  Stopping Nginx temporarily..."
docker compose stop nginx || true

# 4. Generate certificates for QA domains
echo ""
echo "3️⃣  Generating SSL certificates for QA domains..."
sudo certbot certonly --standalone \
    -d qa-tsa.bigdavi.com \
    -d qa-ast.bigdavi.com \
    --agree-tos \
    -m "$CERT_EMAIL" \
    --non-interactive \
    --keep-until-expiring

# Verify certificates were created
if [ -d "/etc/letsencrypt/live/qa-tsa.bigdavi.com" ]; then
    echo "✅ SSL certificates generated successfully"
else
    echo "❌ Error: SSL certificates not found"
    exit 1
fi

# 5. Create certificate symlinks (if docker-compose expects them elsewhere)
echo ""
echo "4️⃣  Verifying certificate paths..."
cert_dir="/etc/letsencrypt/live"
if [ -f "$cert_dir/qa-tsa.bigdavi.com/fullchain.pem" ]; then
    echo "✅ qa-tsa.bigdavi.com certificate: $cert_dir/qa-tsa.bigdavi.com"
fi
if [ -f "$cert_dir/qa-ast.bigdavi.com/fullchain.pem" ]; then
    echo "✅ qa-ast.bigdavi.com certificate: $cert_dir/qa-ast.bigdavi.com"
fi

# 6. Start Nginx again
echo ""
echo "5️⃣  Starting Nginx container..."
docker compose up -d nginx

# 7. Wait for Nginx to be healthy
echo "⏳ Waiting for Nginx to be healthy..."
sleep 5

# 8. Verify HTTPS connectivity
echo ""
echo "6️⃣  Verifying HTTPS connectivity..."
for domain in qa-tsa.bigdavi.com qa-ast.bigdavi.com; do
    if curl -sf https://$domain/health >/dev/null 2>&1 || \
       curl -sf https://$domain/ >/dev/null 2>&1; then
        echo "✅ $domain is accessible via HTTPS"
    else
        echo "⚠️  $domain is not responding (services may not be running yet)"
    fi
done

# 9. Setup certificate renewal cron job
echo ""
echo "7️⃣  Setting up automatic certificate renewal..."
cron_entry="0 3 1 * * certbot renew --quiet && docker compose restart nginx"
if ! crontab -l 2>/dev/null | grep -q "certbot renew"; then
    (crontab -l 2>/dev/null; echo "$cron_entry") | crontab -
    echo "✅ Renewal cron job added (runs monthly on the 1st at 03:00 UTC)"
else
    echo "✅ Renewal cron job already configured"
fi

echo ""
echo "=========================================="
echo "✅ QA SSL Setup Complete!"
echo "=========================================="
echo ""
echo "Domains configured:"
echo "  - qa-tsa.bigdavi.com (public proxy endpoint)"
echo "  - qa-ast.bigdavi.com (admin panel)"
echo ""
echo "Certificate location:"
echo "  /etc/letsencrypt/live/"
echo ""
echo "Certificate renewal: Automatic (monthly via cron)"
echo ""
echo "Next step: Run ./first-build-qa.sh to start services"
echo ""
