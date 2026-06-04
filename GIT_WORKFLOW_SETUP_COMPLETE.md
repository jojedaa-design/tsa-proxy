# ✅ GIT WORKFLOW - SETUP COMPLETADO

**Fecha:** 2026-06-04 | **Estado:** 🟢 LISTO PARA USAR

---

## 📍 REPOSITORIES

**GitHub:** https://github.com/jojedaa-design/tsa-proxy

**Acceso con PAT:** `ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` (Guardado en GitHub Config)

---

## 🌳 BRANCHES

| Rama | Ambiente | Servidor | Status |
|------|----------|----------|--------|
| **main** | Producción | 54.39.181.13 | ✅ Sincronizado |
| **develop** | QA/Staging | 54.39.180.141 | ✅ Sincronizado |

---

## 💻 FLUJO DE TRABAJO OPERACIONAL

### 1️⃣ Desarrollo Local (Tu PC)

```bash
# Clonar repositorio (si aún no lo has hecho)
git clone https://github.com/jojedaa-design/tsa-proxy.git
cd tsa-proxy

# Crear rama de feature
git checkout develop
git pull origin develop
git checkout -b feature/nombre-caracteristica

# Hacer cambios y commits
git add .
git commit -m "feat: descripción de la característica"

# Pushear a GitHub
git push origin feature/nombre-caracteristica

# En GitHub: Crear Pull Request → develop
```

### 2️⃣ Testing en QA Server

**URL QA:**
- Admin Panel: https://qa-ast.bigdavi.com
- TSA Proxy: https://qa-tsa.bigdavi.com

```bash
# El servidor QA se actualiza automáticamente o manual:
ssh qa-tsa
cd /opt/qa
git pull origin develop
docker compose build backend frontend
docker compose up -d
docker compose logs -f
```

### 3️⃣ Merge a Producción

Cuando QA ✅ aprueba:

```bash
# Local: Merge develop → main
git checkout main
git pull origin main
git merge develop
git push origin main

# O: Crear Pull Request develop → main en GitHub
```

**Producción se actualiza:**

```bash
ssh prod-tsa
cd /opt/tsa-proxy
git pull origin main
docker compose build backend frontend
docker compose up -d
```

---

## 🔐 Credenciales Guardadas

| Credencial | Ubicación | Valor |
|------------|-----------|-------|
| **GitHub PAT** | LastPass (NO en GitHub) | `ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` |
| **SSH Key QA** | `~/.ssh/qa_tsa_proxy_key` | Privada |
| **SSH Alias qa-tsa** | `~/.ssh/config` | Automático |
| **SSH Alias prod-tsa** | `~/.ssh/config` | Automático |

---

## 📊 Estado Actual de Servidores

### Servidor QA (54.39.180.141)
```
✅ Git inicializado
✅ Branch: develop
✅ URL: https://qa-ast.bigdavi.com | https://qa-tsa.bigdavi.com
✅ Docker Compose listo
⏳ SSL certs: Requiere Let's Encrypt setup
⏳ Primer deploy: Próximo paso
```

**Comandos útiles:**
```bash
ssh qa-tsa
cd /opt/qa && docker compose logs -f
git status && git log --oneline -5
docker compose down
docker compose build --no-cache
```

### Servidor Producción (54.39.181.13)
```
✅ Git inicializado
✅ Branch: main
✅ URL: https://ast.bigdavi.com | https://tsa.bigdavi.com
✅ Docker Compose corriendo
✅ SSL certs: Activos (Let's Encrypt)
✅ Production: EN VIVO
```

**Comandos útiles:**
```bash
ssh prod-tsa
cd /opt/tsa-proxy && docker compose ps
git status && git log --oneline -5
curl -sf https://ast.bigdavi.com/ready
```

---

## 🚀 Primer Deploy Test (Opcional)

Para probar el flujo completo sin cambios reales:

```bash
# Local
git checkout develop
echo "# Test" >> README.md
git add README.md
git commit -m "test: verificar workflow"
git push origin develop

# En GitHub: Crear PR develop → main (sin mergear aún)

# QA Server
ssh qa-tsa "cd /opt/qa && git pull && echo 'QA updated'"

# Si está bien, mergear en GitHub a main
# Producción se actualiza automáticamente
ssh prod-tsa "cd /opt/tsa-proxy && git pull && echo 'Prod updated'"
```

---

## ⚡ Shortcuts para Terminal

Agregar a tu `.bashrc` o `.zshrc`:

```bash
# SSH Shortcuts
alias qa="ssh qa-tsa"
alias prod="ssh prod-tsa"

# Git Shortcuts
alias gf="git fetch --all"
alias gp="git pull"
alias gB="git branch -a"
alias gl="git log --oneline -10"

# Docker Shortcuts (cuando estés en los servidores)
alias dc="docker compose"
alias dcu="docker compose up -d"
alias dcd="docker compose down"
alias dcs="docker compose ps"
alias dcl="docker compose logs -f"
alias dcb="docker compose build"
```

---

## 📋 Checklist de Operaciones Diarias

- [ ] **Mañana:** Revisar logs en QA
- [ ] **Antes de deploy Prod:** Verificar tests/builds en QA
- [ ] **Deploy Prod:** Git pull → docker compose up
- [ ] **Post-deploy:** Verificar https://ast.bigdavi.com/ready
- [ ] **Weekly:** Backup de Prod (en cron ya existe)

---

## 🔧 Próximos Pasos (No Urgente)

1. **CI/CD Automático (GitHub Actions):**
   - Deploy automático a QA cuando push a develop
   - Deploy automático a Prod cuando merge a main
   - Tests automáticos antes de mergear

2. **SSL/TLS para QA:**
   - Agregar qa-ast.bigdavi.com y qa-tsa.bigdavi.com a Let's Encrypt
   - Comando: `certbot certonly --expand -d ast.bigdavi.com -d qa-ast.bigdavi.com`

3. **Monitoring:**
   - Setup de alertas para fallos de deploy
   - Log aggregation (ELK/Datadog)

4. **Rollback Plan:**
   - Script para revertir a commit anterior
   - Documented runbook

---

## 📞 Comandos de Emergencia

### Si algo sale mal en QA:
```bash
ssh qa-tsa
cd /opt/qa
git reset --hard origin/develop  # Revertir cambios locales
git pull origin develop
docker compose down && docker compose up -d
```

### Si algo sale mal en Prod:
```bash
ssh prod-tsa
cd /opt/tsa-proxy
git reset --hard HEAD~1  # Revertir último commit
docker compose down && docker compose up -d
# Luego: git push --force origin main (si necesario, CUIDADO)
```

---

**Todo listo para empezar a trabajar con el flujo Git multi-ambiente.** 🚀

¿Preguntas o necesitas ajustar algo?
