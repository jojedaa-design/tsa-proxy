# 🚀 CI/CD SETUP COMPLETO — TSA PROXY

**Estado:** Listo para implementar  
**Fecha:** 2026-07-19  
**Componentes:** GitHub Actions + QA SSL + QA Build

---

## 📋 CHECKLIST RÁPIDO

Aquí está TODO lo que se creó. Ejecuta en este orden:

- [ ] **1. GitHub Repository** — Crear repo en GitHub (5 min)
- [ ] **2. Local Git Setup** — Inicializar y commit (5 min)
- [ ] **3. GitHub Secrets** — Agregar credenciales (10 min)
- [ ] **4. QA SSL Setup** — Certificados Let's Encrypt (10 min)
- [ ] **5. QA First Build** — Primer deployment (15 min)
- [ ] **6. Verify CI/CD** — Probar workflows (5 min)

**Tiempo total: ~50 minutos**

---

## 🔧 ARCHIVOS CREADOS

```
.github/workflows/
├── qa-deploy.yml          ← Auto-deploy a QA en cada push a develop
└── prod-deploy.yml        ← Auto-deploy a Prod en cada push a main

scripts/
├── setup-qa-ssl.sh        ← Configurar SSL certs en QA
└── first-build-qa.sh      ← Primer build y deployment en QA
```

---

## 📍 PASO 1: GitHub Repository (5 min)

### 1.1 Crear repositorio en GitHub

Si aún no tienes cuenta:

1. Ve a: https://github.com/signup
2. Email: `jesus.ojeda@bigdavi.com`
3. Crea contraseña fuerte (24+ caracteres)
4. Username: `jesusojeda` (o el que prefieras)
5. Confirma email

### 1.2 Crear repositorio

1. Ve a https://github.com/new
2. Nombre: `tsa-proxy`
3. Descripción: "TimeStamp Proxy - Multi-environment RFC 3161 server"
4. Visibilidad: **Private** (datos sensibles)
5. **NO** inicialices con README (ya tenemos código local)
6. Haz click "Create repository"

**Copiar la URL del repositorio** (format: `https://github.com/jesusojeda/tsa-proxy.git`)

---

## 📍 PASO 2: Git Local Setup (5 min)

En tu máquina local (C:\dev\tsa_proxy):

```bash
cd C:\dev\tsa_proxy

# 1. Configurar remotes (si no está ya configurado)
git remote -v  # Ver remotes actuales

# Si no tiene remote 'origin', agregar:
git remote add origin https://github.com/jesusojeda/tsa-proxy.git

# 2. Crear rama develop (si no existe)
git branch develop || git checkout develop

# 3. Verificar estado
git status

# 4. Agregar archivos de CI/CD
git add .github/workflows/ scripts/setup-qa-ssl.sh scripts/first-build-qa.sh

# 5. Commit
git commit -m "chore: add CI/CD workflows and QA setup scripts

- Add GitHub Actions workflows for automated deployment
  - qa-deploy.yml: Deploy to QA on push to develop
  - prod-deploy.yml: Deploy to Production on push to main
- Add SSL certificate setup script for QA environment
- Add first build and deployment script for QA
- CI/CD pipeline ready for activation"

# 6. Push a GitHub
git push -u origin develop
git push -u origin main  # Also push main if it exists
```

**Resultado esperado:** Los archivos de CI/CD aparecen en GitHub

---

## 📍 PASO 3: GitHub Secrets (10 min)

Los workflows necesitan credenciales SSH para conectarse a los servidores.

### 3.1 Generar SSH Key (si no tienes)

En tu máquina:

```bash
# Para QA
ssh-keygen -t ed25519 -f ~/.ssh/qa_tsa_proxy_key -N ""

# Para Producción (si usas otra key)
# La clave ya existe: ~/.ssh/encuestas
```

### 3.2 Agregar secrets en GitHub

1. Ve a: `https://github.com/jesusojeda/tsa-proxy/settings/secrets/actions`
2. Haz click "New repository secret"

Agrega estos secrets:

| Nombre | Valor |
|---|---|
| `QA_SERVER_IP` | `54.39.180.141` |
| `QA_SERVER_USER` | `ubuntu` |
| `QA_SSH_KEY` | Contenido de `~/.ssh/qa_tsa_proxy_key` (privada) |
| `PROD_SERVER_IP` | `54.39.181.13` |
| `PROD_SERVER_USER` | `ubuntu` |
| `PROD_SSH_KEY` | Contenido de `~/.ssh/encuestas` (privada) |

### 3.3 Copiar contenido de SSH keys

En PowerShell:

```powershell
# QA key
Get-Content ~/.ssh/qa_tsa_proxy_key -Raw | Set-Clipboard

# Luego pega en GitHub secret QA_SSH_KEY

# Prod key
Get-Content ~/.ssh/encuestas -Raw | Set-Clipboard

# Luego pega en GitHub secret PROD_SSH_KEY
```

**Resultado esperado:** Los 6 secrets aparecen en GitHub Actions settings

---

## 📍 PASO 4: QA SSL Setup (10 min)

En el **servidor QA** (54.39.180.141):

```bash
# 1. Conectar al servidor QA
ssh qa-tsa

# 2. Descargar o crear script (si no lo tienes via git pull)
# Opción A: Si git ya está sincronizado
cd /opt/qa
git pull origin develop
bash scripts/setup-qa-ssl.sh

# Opción B: Si necesitas el script manualmente
# Copia el contenido de scripts/setup-qa-ssl.sh y:
nano setup-qa-ssl.sh  # Pega el contenido
chmod +x setup-qa-ssl.sh
./setup-qa-ssl.sh
```

**Detalles del script:**
- ✅ Instala Certbot
- ✅ Genera certificados Let's Encrypt para qa-tsa.bigdavi.com y qa-ast.bigdavi.com
- ✅ Configura renovación automática (cron)
- ✅ Verifica HTTPS

**Resultado esperado:**
```
✅ SSL certificates generated successfully
✅ qa-tsa.bigdavi.com certificate: /etc/letsencrypt/live/qa-tsa.bigdavi.com
✅ qa-ast.bigdavi.com certificate: /etc/letsencrypt/live/qa-ast.bigdavi.com
✅ Renewal cron job added
```

---

## 📍 PASO 5: QA First Build (15 min)

Aún en el **servidor QA**:

```bash
# Asegúrate de estar en /opt/qa
cd /opt/qa

# 1. Verifica que .env existe (debe estar ya configurado)
cat .env | head -5

# 2. Ejecuta el script de primer build
bash scripts/first-build-qa.sh

# El script hará:
# - Verificar configuración
# - Construir imágenes Docker (3-5 min)
# - Iniciar PostgreSQL y Redis
# - Aplicar migraciones
# - Iniciar backend, frontend, nginx
# - Verificar salud de servicios
# - Crear usuario admin (interactivo)
```

**Resultado esperado:**
```
✅ QA First Build Complete!

🌐 Access QA Environment:
   Admin Panel: https://qa-ast.bigdavi.com
   TSA Proxy:   https://qa-tsa.bigdavi.com
```

**Verificar manualmente:**

```bash
# Ver estado de servicios
ssh qa-tsa "cd /opt/qa && docker compose ps"

# Ver logs
ssh qa-tsa "cd /opt/qa && docker compose logs backend --tail=20"

# Probar salud
curl https://qa-tsa.bigdavi.com/health
```

---

## 📍 PASO 6: Verificar CI/CD (5 min)

Ahora testea que los workflows funcionan:

### 6.1 Test: Deploy a QA

```bash
# En tu máquina local
cd C:\dev\tsa_proxy

# 1. Hacer pequeño cambio en develop
git checkout develop
echo "# CI/CD Tested at $(date)" >> README.md
git add README.md
git commit -m "test: verify CI/CD workflows"
git push origin develop

# 2. Ir a GitHub Actions
# https://github.com/jesusojeda/tsa-proxy/actions

# 3. Ver workflow "Deploy to QA" ejecutándose
# Debe completarse en ~2 minutos

# 4. Verificar en QA
ssh qa-tsa "curl https://qa-tsa.bigdavi.com/health"
```

### 6.2 Test: Deploy a Producción (MANUAL)

```bash
# En tu máquina local
cd C:\dev\tsa_proxy

# 1. Mergear develop a main
git checkout main
git pull origin main
git merge develop
git push origin main

# 2. En GitHub:
# - Ir a Actions → Deploy to Production
# - Click "Run workflow"
# - Confirmar en la ventana de environment (si pide)

# 3. Ver workflow ejecutándose
# Debe completarse en ~2-3 minutos

# 4. Verificar en Producción
ssh prod-tsa "curl https://tsa.bigdavi.com/health"
```

**Resultado esperado:** Ambos workflows completan exitosamente ✅

---

## 🏗️ ARQUITECTURA DEL FLUJO

```
Tu código local
    ↓ git commit + push origin develop
    ↓
GitHub Repository
    ↓ Webhook dispara qa-deploy.yml
    ↓
GitHub Actions Runner
    ├─ Checkout código
    └─ SSH a 54.39.180.141 (QA)
         ├─ git pull origin develop
         ├─ docker compose build
         ├─ docker compose up -d
         └─ Verificar salud
    
Servidor QA (54.39.180.141)
    └─ Servicios actualizados en /opt/qa
       └─ Accesible en qa-tsa.bigdavi.com

════════════════════════════════════════════════════════

Para Producción: Mismo flujo pero
    - Branch: main
    - Server: 54.39.181.13
    - Path: /opt/tsa-proxy
    - Domains: tsa/ast.bigdavi.com
    - Backup automático antes de deploy
```

---

## 📊 CHECKLIST FINAL

Después de completar PASO 1-6:

| Item | Status |
|------|--------|
| GitHub repo creado | ✅ |
| Código pusheado a GitHub | ✅ |
| Secrets configurados | ✅ |
| QA SSL certificates | ✅ |
| QA servicios running | ✅ |
| Workflow QA testeado | ✅ |
| Workflow Prod testeado | ✅ |
| Admin user en QA | ✅ |
| Auto-deploy habilitado | ✅ |

---

## 🎯 CÓMO USAR DESDE AHORA

**Workflow diario:**

```bash
# 1. Crear feature branch
git checkout develop
git pull origin develop
git checkout -b feature/mi-feature

# 2. Hacer cambios
# ... editar código ...

# 3. Commit + push
git add .
git commit -m "feat: mi cambio"
git push origin feature/mi-feature

# 4. Pull Request a develop
# Ir a GitHub → Create Pull Request
# Mergear cuando esté listo

# 5. GitHub Actions dispara automáticamente
# ✅ Build a QA
# ✅ Tests en QA
# ✅ Si OK → mergear a main
# ✅ GitHub Actions dispara Producción

# 6. Prod actualizada automáticamente
# ✅ Nuevos cambios en vivo
```

---

## 🚨 TROUBLESHOOTING

### "SSH key permission denied"

```bash
# Verificar que la SSH key está bien configurada en GitHub
ssh -i ~/.ssh/qa_tsa_proxy_key ubuntu@54.39.180.141 "echo OK"
```

### "docker compose: command not found"

En QA server:

```bash
ssh qa-tsa "docker compose version"
# Si no está instalado:
sudo apt install docker-compose-plugin
```

### "Health check failed"

```bash
# Ver logs del backend
ssh qa-tsa "cd /opt/qa && docker compose logs backend --tail=50"

# Reintentar
ssh qa-tsa "cd /opt/qa && docker compose restart backend"
```

### "Database connection refused"

```bash
# PostgreSQL no está listo
ssh qa-tsa "cd /opt/qa && docker compose logs postgres --tail=20"

# Esperar + reintentar
sleep 30
docker compose restart postgres
```

---

## 📝 PRÓXIMAS MEJORAS

Después de que todo esté funcionando:

- [ ] Slack notifications en GitHub Actions
- [ ] Automated testing en CI/CD (pytest, jest)
- [ ] Database backup automatizado antes de deploy
- [ ] Rollback automático si health check falla
- [ ] Email alerts en deploy a producción
- [ ] Monitoring dashboard (Grafana)

---

**¡CI/CD completo está listo! 🎉**

¿Preguntas? Revisa los logs en GitHub Actions:  
https://github.com/jesusojeda/tsa-proxy/actions

