# TimeStamp Proxy

Proxy RFC 3161 con panel de administración. Expone una URL pública propia para
sellos de tiempo, enrutando al upstream TSA externo, con autenticación, cuotas,
IP allowlist y auditoría completa.

---

## Arquitectura

```
Internet → Nginx (TLS) → Backend Go :8080 → PostgreSQL + Redis
                       → Frontend Next.js :3000
                       → TSA Upstream externa (solo el backend la conoce)
```

- **`tsa.bigdavi.com`** — Proxy público RFC 3161
- **`ast.bigdavi.com`** — Panel de administración

---

## Requisitos previos en la VM

- Ubuntu 24.04 LTS
- Docker Engine ≥ 26
- Docker Compose ≥ 2.28
- Dominio `tsa.bigdavi.com` y `ast.bigdavi.com` apuntando a la IP de la VM
- Puerto 80 y 443 abiertos en el firewall

---

## Despliegue inicial paso a paso

### 1. Instalar Docker

```bash
curl -fsSL https://get.docker.com | bash
sudo usermod -aG docker $USER
newgrp docker
docker --version
```

### 2. Clonar / subir el proyecto

```bash
# Desde tu máquina local:
rsync -avz --exclude='.git' \
  -e "ssh -i ~/.ssh/encuestas" \
  /c/dev/tsa_proxy/ \
  ubuntu@54.39.181.13:/opt/tsa-proxy/

# En el servidor:
cd /opt/tsa-proxy
```

### 3. Configurar variables de entorno

```bash
cp .env.example .env
nano .env
```

Variables **obligatorias** a cambiar:

| Variable | Descripción |
|---|---|
| `POSTGRES_PASSWORD` | Contraseña fuerte para Postgres |
| `REDIS_PASSWORD` | Contraseña fuerte para Redis |
| `JWT_SECRET` | `openssl rand -hex 32` |
| `TSA_UPSTREAM_URL` | URL de tu TSA externa |
| `TSA_UPSTREAM_USERNAME` | Usuario en la TSA externa |
| `TSA_UPSTREAM_PASSWORD` | Contraseña en la TSA externa |

```bash
# Generar JWT_SECRET seguro:
openssl rand -hex 32
```

### 4. Obtener certificados TLS (Certbot)

```bash
# Instalar certbot
sudo apt update && sudo apt install -y certbot

# Obtener certificados (antes de levantar Nginx con HTTPS)
sudo certbot certonly --standalone \
  -d tsa.bigdavi.com \
  -d ast.bigdavi.com \
  --agree-tos -m admin@bigdavi.com --non-interactive

# Los certs quedan en /etc/letsencrypt/live/
# El docker-compose los monta como volumen read-only
```

### 5. Iniciar los servicios

```bash
cd /opt/tsa-proxy

# Construir imágenes (primera vez, tarda ~3 min)
docker compose build

# Levantar base de datos y Redis primero
docker compose up -d postgres redis

# Esperar healthchecks (unos 15 segundos)
docker compose ps

# Ejecutar migraciones SQL
bash scripts/run-migrations.sh

# Levantar el resto
docker compose up -d

# Verificar que todo esté up
docker compose ps
docker compose logs backend --tail=50
```

### 6. Crear el superadmin inicial

```bash
# Opción A: usar el script de seed
SEED_ADMIN_USERNAME=admin \
SEED_ADMIN_EMAIL=admin@bigdavi.com \
SEED_ADMIN_PASSWORD=CambiarEsto123! \
bash scripts/seed-data.sh

# Opción B: insertar directamente el hash generado por el backend
docker exec tsa_backend /tsa-proxy help   # ver subcomandos disponibles
```

> El script seed genera un hash placeholder. Para producción, usa la utilidad
> del backend o cambia la contraseña inmediatamente desde el panel.

### 7. Verificar el servicio

```bash
# Health check del backend
curl https://tsa.bigdavi.com/health

# Ready check (verifica Postgres + Redis)
curl https://tsa.bigdavi.com/ready

# Panel admin
# Abrir en browser: https://ast.bigdavi.com
```

---

## Actualizar el sistema

```bash
cd /opt/tsa-proxy

# 1. Subir nuevo código
rsync -avz --exclude='.git' -e "ssh -i ~/.ssh/encuestas" \
  /c/dev/tsa_proxy/ ubuntu@54.39.181.13:/opt/tsa-proxy/

# 2. Reconstruir solo las imágenes que cambiaron
docker compose build backend
docker compose build frontend

# 3. Aplicar migraciones nuevas si las hay
bash scripts/run-migrations.sh

# 4. Hacer rolling restart (sin downtime)
docker compose up -d --no-deps backend
docker compose up -d --no-deps frontend
```

---

## Operación diaria

### Ver logs en tiempo real

```bash
# Todos los servicios
docker compose logs -f

# Solo el backend
docker compose logs -f backend

# Solo Nginx (access log)
docker compose logs -f nginx
```

### Backup manual de Postgres

```bash
BACKUP_DEST_DIR=/var/backups/tsaproxy bash scripts/backup-db.sh
```

### Configurar cron para backups automáticos

```bash
sudo crontab -e

# Agregar:
# Backup diario a las 02:00 UTC
0 2 * * * cd /opt/tsa-proxy && bash scripts/backup-db.sh >> /var/log/tsaproxy-backup.log 2>&1

# Crear partición del mes siguiente el día 25
0 12 25 * * cd /opt/tsa-proxy && bash scripts/create-partition.sh >> /var/log/tsaproxy-partitions.log 2>&1

# Renovar certificados TLS (Certbot lo hace automáticamente, pero como respaldo)
0 3 1 * * certbot renew --quiet && docker compose restart nginx
```

### Limpieza manual de datos antiguos

```bash
# Ejecutar limpieza manual (el backend lo hace automáticamente a las 03:00 UTC)
bash scripts/cleanup-old-data.sh
```

---

## Configurar el primer cliente

1. Abrir `https://ast.bigdavi.com` → Login
2. **Clientes** → **Nuevo cliente** → completar nombre, slug, email
3. En el detalle del cliente → **Credenciales** → **Crear credencial**
   - ⚠️ Copiar el API key mostrado — no se puede recuperar después
4. En **IP Allowlist** → agregar las IPs del cliente (vacío = cualquier IP)
5. En **Cuota** → configurar límite mensual y burst por minuto

### Probar el endpoint con openssl

```bash
# Generar un timestamp request RFC 3161
openssl ts -query -data archivo.txt -sha256 -out request.tsq

# Enviarlo al proxy
curl -s -X POST https://tsa.bigdavi.com/api/v1/timestamp \
  -H "Authorization: Bearer tsp_TU_API_KEY_AQUI" \
  -H "Content-Type: application/timestamp-query" \
  --data-binary @request.tsq \
  -o response.tsr

# Verificar la respuesta
openssl ts -reply -in response.tsr -text
```

---

## Hardening básico recomendado

### Firewall UFW

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow ssh
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status
```

### Restringir acceso al panel admin por IP

En `/opt/tsa-proxy/nginx/conf.d/ast.bigdavi.com.conf`, agregar dentro del bloque `server` de HTTPS:

```nginx
# Solo permitir acceso al panel desde estas IPs
allow 203.0.113.10;    # IP de la oficina
allow 198.51.100.5;    # IP del admin remoto
deny all;
```

Luego recargar Nginx:
```bash
docker compose exec nginx nginx -s reload
```

### Fail2ban para SSH

```bash
sudo apt install -y fail2ban
sudo systemctl enable fail2ban --now
```

### Deshabilitar login SSH con contraseña

```bash
sudo sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config
sudo systemctl restart sshd
```

### Actualizar el sistema regularmente

```bash
sudo apt update && sudo apt upgrade -y
# Reiniciar si hay actualizaciones de kernel
sudo reboot   # solo si es necesario
```

---

## Estructura de archivos

```
/opt/tsa-proxy/
├── backend/          # Código Go + migraciones SQL
├── frontend/         # Panel Next.js
├── nginx/            # Configuración Nginx
├── scripts/          # Scripts operativos
├── docker-compose.yml
├── .env              # Variables de entorno (NO commitear)
└── .env.example      # Plantilla de variables
```

---

## Variables de entorno — referencia completa

Ver [.env.example](.env.example) con descripción de cada variable.

---

## Solución de problemas

### El backend no inicia

```bash
docker compose logs backend --tail=100
# Verificar: POSTGRES_PASSWORD, REDIS_PASSWORD, JWT_SECRET, TSA_UPSTREAM_URL
```

### Nginx da 502

```bash
docker compose ps        # verificar que backend y frontend estén "healthy"
docker compose logs nginx
```

### Error de certificado TLS

```bash
sudo certbot certificates         # ver estado
sudo certbot renew --force-renewal -d tsa.bigdavi.com -d ast.bigdavi.com
docker compose restart nginx
```

### Postgres no inicia

```bash
docker compose logs postgres
# Si hay corrupción de datos:
docker compose down
docker volume inspect tsa-proxy_pgdata
# Restaurar desde backup si es necesario
```

### Revisar consumo de recursos

```bash
docker stats                      # CPU y RAM por contenedor
df -h                             # Espacio en disco
du -sh /var/lib/docker/volumes/   # Tamaño de volúmenes
```
