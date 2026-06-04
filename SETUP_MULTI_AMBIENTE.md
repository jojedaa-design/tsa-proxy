# 🚀 SETUP MULTI-AMBIENTE TSA PROXY
**Fecha:** 2026-06-04 | **Estado:** En Configuración

---

## ✅ COMPLETADO

### 1. Acceso a Servidores (SSH Configured)
```bash
# Alias creados en ~/.ssh/config
ssh qa-tsa      # → 54.39.180.141 (Ubuntu user)
ssh prod-tsa    # → 54.39.181.13 (Ubuntu user)
```

**Archivos de llaves:**
- `~/.ssh/qa_tsa_proxy_key` (QA - RSA format)
- `~/.ssh/encuestas` (Producción)

### 2. Servidor QA (54.39.180.141) - LISTO
```
✅ Docker 29.1.3 instalado
✅ Docker Compose instalado
✅ Git instalado
✅ /opt/qa/ preparado
✅ docker-compose.yml copiado y configurado
✅ .env generado con secrets únicos
✅ Nginx configs adaptados para dominios QA:
   - qa-ast.bigdavi.com (Panel admin)
   - qa-tsa.bigdavi.com (Proxy público)
```

**Directorio estructura:**
```
/opt/qa/
├── docker-compose.yml      ← Copiado desde Prod
├── .env                     ← Generado con secrets QA
└── nginx/
    ├── nginx.conf
    └── conf.d/
        ├── qa-ast.bigdavi.com.conf
        └── qa-tsa.bigdavi.com.conf
```

**Secrets generados (seguros):**
- JWT_SECRET: `c82ac22ed18d387d772b73f8f4f07027d9443b0f96a870b5661d5d88e93ac700`
- POSTGRES_PASSWORD: Generada aleatoriamente
- REDIS_PASSWORD: Generada aleatoriamente
- Database: `tsaproxy_qa` (separada de producción)

---

## ⏳ PENDIENTE - TU ACCIÓN

### GitHub Account Creation
**Haz esto AHORA:**

1. Ve a: https://github.com/signup
2. Email: `jesus.ojeda@bigdavi.com`
3. Password: (segura, 24+ caracteres)
4. Username: `jesusojeda` (o lo que prefieras)
5. **Confirma email** (revisa bandeja)
6. **Avísame cuando esté listo** con:
   - Username exacto
   - Password (si quieres que lo guarde en lastpass o similar)

**Una vez creado:** Podré:
- ✅ Inicializar repositorio Git
- ✅ Crear ramas (main/develop)
- ✅ Configurar CI/CD automático
- ✅ Hacer primer commit

---

## 📋 PRÓXIMOS PASOS (Después de GitHub)

### Fase 1: Git Initialization
```bash
# Local
git init /c/dev/tsa_proxy
git remote add origin https://github.com/jesusojeda/tsa-proxy.git
git checkout -b develop
git add .
git commit -m "Initial commit: TSA Proxy multi-environment setup"
git push -u origin develop
```

### Fase 2: Sync Servers to Git
```bash
# Producción
cd /opt/tsa-proxy
git init
git remote add origin https://github.com/jesusojeda/tsa-proxy.git
git checkout main
git pull origin main

# QA
cd /opt/qa
git init
git remote add origin https://github.com/jesusojeda/tsa-proxy.git
git checkout develop
git pull origin develop
```

### Fase 3: CI/CD Setup (GitHub Actions)
Crear `.github/workflows/`:
- `qa-deploy.yml` — Deploy a QA cuando push a `develop`
- `prod-deploy.yml` — Deploy a Prod cuando push a `main` (manual o automático)

### Fase 4: First Build en QA
```bash
ssh qa-tsa
cd /opt/qa
docker compose build backend frontend
docker compose up -d
# Acceder a: https://qa-ast.bigdavi.com
```

---

## 🌳 Flujo de Trabajo (Operacional)

```
Tu PC (Local Development)
    ↓ git checkout -b feature/nueva-caracteristica
    ↓ Editar código, commit
    ↓ git push origin feature/nueva-caracteristica
    ↓ Crear Pull Request → develop
    
GitHub (Merge automatizado)
    ↓ Webhook dispara CI/CD
    
Servidor QA (54.39.180.141)
    ↓ git pull origin develop
    ↓ docker compose build + up
    ↓ Pruebas en https://qa-ast.bigdavi.com
    
Si OK → Merge develop → main
    ↓
Servidor Producción (54.39.181.13)
    ↓ git pull origin main
    ↓ docker compose build + up
    ↓ Go live en https://ast.bigdavi.com
```

---

## 🔐 Credenciales Guardadas (Seguro)

Guardar en LastPass o 1Password:

| Nombre | Valor | Ambiente |
|--------|-------|----------|
| QA JWT Secret | `c82ac22ed18d387d772b73f8f4f07027d9443b0f96a870b5661d5d88e93ac700` | QA |
| QA DB Password | (en /opt/qa/.env) | QA |
| QA Redis Password | (en /opt/qa/.env) | QA |
| GitHub Username | (cuando crees) | Compartido |
| GitHub PAT | (generar después) | Compartido |

---

## 📊 Resumen Estado Actual

| Componente | Producción | QA | Status |
|-----------|-----------|----|----|
| **Servidor** | 54.39.181.13 | 54.39.180.141 | ✅ Accesible |
| **Docker** | ✅ Instalado | ✅ Instalado | ✅ Listo |
| **Código** | `/opt/tsa-proxy` | `/opt/qa` | ⏳ Esperando Git |
| **Nginx** | ast/tsa.bigdavi.com | qa-ast/qa-tsa.bigdavi.com | ✅ Configurado |
| **SSL Certs** | Let's Encrypt | Requiere setup | ⏳ Después |
| **Database** | tsaproxy | tsaproxy_qa | ✅ Separadas |
| **Git Repo** | ❌ No existe | ❌ No existe | ⏳ Esperando GitHub |
| **CI/CD** | ❌ No existe | ❌ No existe | ⏳ Esperando GitHub |

---

## 📞 Comandos Rápidos

```bash
# Conectarse a servidores
ssh qa-tsa
ssh prod-tsa

# Ver estado de servicios en QA
ssh qa-tsa "cd /opt/qa && docker compose ps"

# Ver logs en QA
ssh qa-tsa "cd /opt/qa && docker compose logs -f backend"

# Detener todo en QA
ssh qa-tsa "cd /opt/qa && docker compose down"

# Reconstruir en QA
ssh qa-tsa "cd /opt/qa && docker compose build --no-cache"
```

---

## 🎯 Checklist Final

- [ ] GitHub account creada
- [ ] GitHub PAT generado (token de acceso personal)
- [ ] Repositorio inicializado
- [ ] Ramas creadas (main, develop)
- [ ] CI/CD workflows configurados
- [ ] DNS apuntando a QA (si necesario)
- [ ] SSL certs para QA (Let's Encrypt)
- [ ] Primer build en QA exitoso
- [ ] Primer build en Prod exitoso
- [ ] Documentación de equipo completada

---

**Próximo paso:** Avisame cuando tengas tu cuenta de GitHub lista 🚀
