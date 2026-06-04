# Informe de Seguridad y Rendimiento — TSA Proxy

**Proyecto:** TSA Proxy RFC 3161 Multi-tenant (bigdavi)
**Alcance:** backend Go, frontend Next.js, Nginx, PostgreSQL, Redis, despliegue docker-compose en `54.39.181.13`
**Fecha:** 2026-04-18
**Autor:** auditoría automatizada

---

## Resumen ejecutivo

**Seguridad.** El proyecto tiene una base sólida: queries parametrizadas, Argon2id para admins, TOTP 2FA, segmentación de red en Docker, rate-limits en dos capas, headers de seguridad en Nginx, cookies `Secure+HttpOnly+SameSite=Strict` y audit events. Los riesgos principales se concentran en **(1) SSRF explotable por admin**, **(2) autorización plana sin roles**, **(3) dependencias desactualizadas (Next.js con CVE público)** y **(4) credenciales upstream en texto plano**. Implementando las 4 primeras acciones priorizadas se elimina ~70 % del riesgo explotable.

**Rendimiento.** El hardware (4 vCPU / 7.6 GiB RAM / 48 GB disco) está **sub-utilizado > 95 %**. El cuello de botella no es CPU/RAM/DB, es la **latencia del upstream TSA externo** (Camerfirma ~600 ms; DigiCert ~100-150 ms). Se recomienda un **techo sostenido de 30 req/s (~1.800 req/min, ~2.6 M/mes)** con burst de 60 req/s (<2 s). El `RATE_LIMIT_GLOBAL_RPS=200` actual es inerte porque Nginx ya topa en 20 r/s.

---

# Parte 1 — Seguridad (OWASP Top 10:2021)

## Tabla-resumen

| Categoría | Estado | Hallazgos críticos |
|---|---|---|
| **A01 Broken Access Control** | 🟠 Riesgo | `RequireRole` existe pero no se aplica; `/ts/{urlToken}` sin IP allowlist ni rate-limit por tenant; subrecursos por ID sin validar tenant |
| **A02 Cryptographic Failures** | 🟠 Parcial | Basic Auth con SHA-256 sin salt; contraseñas upstream en **texto plano** en BD; Postgres/Redis sin TLS interno |
| **A03 Injection** | 🟢 OK | Todas las queries con placeholders `$1..$n`; no hay interpolación de input |
| **A04 Insecure Design** | 🟠 Riesgo | **SSRF** (ver A10); **fail-open** generalizado en RateLimit e IPAllowlist; burst de 100 muy alto |
| **A05 Security Misconfiguration** | 🟠 Parcial | `POSTGRES_SSL_MODE=disable`; CSP con `'unsafe-eval'`; backend corre como root; X-XSS-Protection obsoleto |
| **A06 Vulnerable & Outdated Components** | 🔴 Riesgo | **`next@14.2.5` CVE-2025-29927** (middleware bypass); Go 1.22; `go.sum` ausente del repo |
| **A07 Identification & Authentication** | 🟠 Parcial | Sin account-lockout por usuario; 2FA no obligatorio; refresh token sin rotación; TOTP sin replay protection |
| **A08 Software & Data Integrity** | 🟠 Parcial | Upstream HTTP sin validación de esquema; TSR no se valida criptográficamente antes de devolverlo |
| **A09 Security Logging & Monitoring** | 🟠 Parcial | `audit_events` no registra IP/UA; login/logout no se auditan; sin alertas activas |
| **A10 SSRF** | 🔴 **Riesgo Alto** | URL upstream configurable por admin sin validación — puede apuntar a localhost, IMDS cloud, servicios internos |

### Top 10 acciones priorizadas

| # | Acción | OWASP | Severidad | Esfuerzo |
|---|---|---|---|---|
| 1 | **Validar URL upstream**: forzar `https://`, bloquear IPs privadas/loopback/link-local en `handler/admin/config.go` y `upstream/client.go` | A10, A04 | Alta | Bajo |
| 2 | **Actualizar `next` a 14.2.25+** (o 15.2.3+) y commit `package-lock.json` | A06 | Alta | Bajo |
| 3 | Aplicar `AdminAuthMW.RequireRole(...)` en router; segregar lectura/escritura | A01 | Media-Alta | Medio |
| 4 | **Cifrar `tsa_upstreams.password`** en reposo (AES-GCM con KEK); nunca devolverla en GET | A02 | Alta | Medio |
| 5 | Lockout por usuario (`locked_until`) tras 5 logins fallidos + 2FA obligatorio para admin | A07 | Media | Medio |
| 6 | Commit `go.sum`, quitar `go mod tidy` del Dockerfile, subir a Go 1.23+ | A06, A08 | Media | Bajo |
| 7 | IP allowlist + rate-limit por tenant en `/ts/{urlToken}`; enriquecer audit_events con IP/UA | A01, A09 | Media | Bajo |
| 8 | Pepper (HMAC) sobre hash SHA-256 de Basic Auth usando `JWT_SECRET` | A02 | Media | Medio |
| 9 | Cambiar **fail-open a fail-closed** en rate-limit e IP allowlist cuando Redis/DB caigan | A04 | Media | Bajo |
| 10 | Postgres TLS (`verify-full`), Redis TLS, backend como `USER` no-root, quitar `'unsafe-eval'` del CSP | A05, A02 | Media | Medio |

---

## A01 — Broken Access Control · 🟠 Riesgo

### Hallazgos

- **A01-1 — `RequireRole` definido pero no usado** (CWE-862, Media)
  `middleware/adminauth.go:70-89` define el helper, pero `server/router.go:110-202` no lo invoca en ninguna ruta. Todo admin con JWT válido puede crear/suspender tenants, modificar upstream TSA global y exportar CSV de uso de todos los tenants. El `viewer` puede revocar credenciales.

- **A01-2 — Falta validación cross-tenant en subrecursos por ID** (CWE-639 IDOR, Media)
  Rutas `POST /api/admin/v1/credentials/{id}/rotate`, `POST /basic-auth/{id}/revoke`, `PUT/DELETE /ip-allowlist/{id}` reciben un ID sin vincularlo al tenant del admin actual. No explotable hoy (todos los admins ven todos los tenants), pero rompe aislamiento al momento de introducir roles "admin-de-tenant".

- **A01-3 — `/ts/{urlToken}` no aplica IP allowlist ni rate-limit por tenant** (CWE-284, Baja-Media)
  `router.go:100`: solo aplica `RateLimitMW.Global`. Quien tenga el token puede usarlo desde cualquier IP.

- **A01-4 — `audit_events` sin `ip_address`/`user_agent`** (Baja)
  Los handlers `ipallowlist`, `quotas`, `noauth` no rellenan estos campos. Dificulta forense.

### Remediación

```go
// router.go — ejemplos
r.With(d.AdminAuthMW.RequireRole("admin","superadmin")).Route("/config/upstreams", ...)
r.With(d.AdminAuthMW.RequireRole("superadmin")).Delete("/tenants/{id}", ...)

// router.go — añadir a /ts/{urlToken}:
r.With(d.RateLimitMW.Global, d.IPAllowMW.Check, d.RateLimitMW.PerTenant).
    Post("/ts/{urlToken}", d.TimestampHandler.HandleByToken)
```

---

## A02 — Cryptographic Failures · 🟠 Parcial

### Hallazgos

- **A02-1 — Basic Auth con SHA-256 sin salt** (CWE-916, Alta)
  `repository/postgres/basicauth.go:121-124`: `sha256.Sum256([]byte(password))`. Dos tenants con la misma contraseña → mismo hash. Un dump de `basic_auth_credentials` permite precomputar rainbow tables. Aceptable criptográficamente si las contraseñas son de 128 bits aleatorios, pero añadir pepper cuesta poco y elimina el escenario.

- **A02-2 — Contraseñas de upstream en texto plano** (CWE-312, Alta)
  `repository/postgres/upstreams.go:93-111` y `service/proxy/service.go:344`. Un dump de `tsa_upstreams` expone las credenciales del proveedor TSA real.

- **A02-3 — `POSTGRES_SSL_MODE=disable`** (CWE-319, Media)
  `config/config.go:119`. Tráfico SQL plano dentro de la red `proxy-internal`.

- **A02-4 — Redis sin TLS** (Baja)
  Sesiones MFA, hashes Basic Auth y refresh tokens pasan en claro.

- **A02-5 — JWT HS256 con secret compartido y sin rotación** (CWE-798, Baja)
  `service/auth/service.go:288`. Ningún mecanismo de rotación con grace period.

### Remediación

1. Migrar `basic_auth_credentials.key_hash` a **HMAC-SHA256** con pepper del `JWT_SECRET` (compatible, se re-hashea al primer uso) o a **Argon2id**.
2. Cifrar `tsa_upstreams.password` con **AES-GCM** y clave en `.env` (KEK). Nunca devolver en GET: `{"has_password": true}`.
3. `POSTGRES_SSL_MODE=verify-full` + certs internos.
4. Redis con TLS (`rediss://`).

---

## A03 — Injection · 🟢 OK

Revisión exhaustiva: todas las queries en `internal/repository/postgres/*` usan `pgx` con placeholders `$1, $2, ...`. Los usos de `fmt.Sprintf` son solo para construir placeholders dinámicos según filtros. Nginx escapa CRLF en logs (desde 1.11.8).

**Recomendación menor:** usar `subtle.ConstantTimeCompare` en comparaciones de hashes aunque el riesgo práctico sea bajo.

---

## A04 — Insecure Design · 🟠 Riesgo

### Hallazgos

- **A04-1 — SSRF en upstream** (ver A10).

- **A04-2 — Fail-open generalizado** (CWE-755, Media)
  `middleware/ratelimit.go:31-35, 68-73` y `middleware/ipallowlist.go:65-70`: si Redis o BD fallan, el request se **permite**. Un atacante que tire Redis desactiva todos los rate-limits.

- **A04-3 — `noauth_access` solo por IP** (Baja)
  `middleware/tsp.go:150-169`. Si la IP del cliente es NAT compartido, cualquiera tras el NAT firma en nombre del tenant.

- **A04-4 — Burst de 100 en Nginx** (Baja)
  `tsa.bigdavi.com.conf:44`: `burst=100 nodelay` sobre `20r/s`. Combinado con fail-open, permite 100 TSRs instantáneos antes de que tomen efecto los demás rate-limits.

### Remediación

- Cambiar **fail-closed** en rate-limit e IP allowlist con excepción documentada para health-checks.
- Reducir `burst` a 30-60 y considerar `delay=N`.
- Modelado de amenazas formal sobre: admin comprometido, upstream mentiroso.

---

## A05 — Security Misconfiguration · 🟠 Parcial

### Hallazgos

| # | Ítem | Archivo | Severidad |
|---|---|---|---|
| A05-1 | CSP con `'unsafe-inline'` + `'unsafe-eval'` | `nginx/conf.d/ast.bigdavi.com.conf:143` | Media |
| A05-2 | Backend y Postgres corren como root | `backend/Dockerfile` | Media |
| A05-3 | `X-XSS-Protection` obsoleto | `nginx/nginx.conf:58` | Baja |
| A05-4 | `ssl_stapling` comentado | ambos `.conf` | Baja |
| A05-5 | Cert `ast.bigdavi.com-0001` se usa para `tsa.bigdavi.com` (SAN?) | `tsa.bigdavi.com.conf:26-27` | Media |
| A05-6 | `server_tokens off` | OK | — |
| A05-7 | Solo Nginx expone `80/443` | `docker-compose.yml` | OK |
| A05-8 | `.env` debe estar en `.dockerignore` y `.gitignore` | verificar | Baja |

### Remediación

```dockerfile
# backend/Dockerfile — añadir al final
USER 1000:1000
```

```nginx
# nginx/nginx.conf — quitar X-XSS-Protection; añadir:
add_header Cross-Origin-Opener-Policy    "same-origin"       always;
add_header Cross-Origin-Resource-Policy  "same-site"         always;
```

Verificar SAN del certificado con `openssl x509 -in fullchain.pem -text -noout | grep -A1 "Subject Alternative Name"`.

---

## A06 — Vulnerable & Outdated Components · 🔴 Riesgo

### Hallazgos

| # | Componente | Versión actual | Riesgo | Acción |
|---|---|---|---|---|
| A06-1 | **`next`** | `14.2.5` | **CVE-2025-29927** middleware bypass | ⚠️ Upgrade YA a `14.2.25+` o `15.2.3+` |
| A06-2 | `golang.org/x/crypto` | `v0.24.0` | Desactualizado (jun-2024) | Subir a `v0.31.0+` |
| A06-3 | Go runtime | `1.22` | EOL; CVE-2024-45336, CVE-2025-22870 fijados en 1.23+ | Subir a `1.23-alpine` |
| A06-4 | `axios` | `^1.7.2` | CVE-2024-39338 (SSRF) fijado en 1.7.4 | Asegurar `package-lock.json` en repo |
| A06-5 | `backend/go.sum` | **ausente del repo** | Build regenera desde internet; pérdida de reproducibilidad | Commit `go.sum`, build con `-mod=readonly` |

### Remediación

```json
// frontend/package.json
{ "next": "14.2.25", "axios": "1.7.9" }
```

```dockerfile
# backend/Dockerfile
FROM golang:1.23-alpine AS builder
# quitar: RUN go mod tidy
COPY go.mod go.sum ./
RUN go mod download
RUN go build -mod=readonly -ldflags="-w -s" -o /app/tsa-proxy ./cmd/server
```

CI: añadir `govulncheck ./...` y `npm audit --omit=dev`.

---

## A07 — Identification & Authentication · 🟠 Parcial

### Hallazgos

- **A07-1 — Sin lockout por usuario** (CWE-307, Media) — el rate-limit de Nginx es por IP, evadible con botnet.
- **A07-2 — 2FA no obligatorio** (CWE-308, Media) — un admin puede simplemente no habilitarlo.
- **A07-3 — TOTP sin replay protection** (CWE-294, Baja) — el mismo código 6 dígitos se acepta en la ventana de 30 s.
- **A07-4 — Refresh token sin rotación** (CWE-287, Baja) — válido 7 días sin invalidar al usarlo.
- **A07-5 — MFA token no vinculado a IP/UA** (Baja) — reutilizable 5 min desde cualquier origen.
- **A07-6 — Timing-attack mitigation en login** — ✅ correcto (`dummy_hash` path).

### Remediación

```sql
-- nueva tabla
CREATE TABLE login_attempts (
    user_id UUID PRIMARY KEY,
    failed_count INT NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ
);
-- lock 15 min tras 5 fallos
```

```go
// en Refresh: rotar refresh_token + revocar el anterior, detectar reuso
// en TOTP validate: SETNX Redis con key tot_used:{user}:{code} TTL 45s
```

---

## A08 — Software & Data Integrity · 🟠 Parcial

### Hallazgos

- **A08-1 — Cliente upstream sin validación de esquema** (CWE-295, Media) — acepta `http://`.
- **A08-2 — TSR upstream no validado** (CWE-345, Media) — basura del upstream se propaga al cliente.
- **A08-3 — `go.sum` ausente** (ver A06-5, CWE-353).
- **A08-4 — Migraciones SQL sin checksum** (Baja).

### Remediación

```go
// service/proxy/service.go — antes de devolver:
var resp TimeStampResp
if _, err := asn1.Unmarshal(body, &resp); err != nil || resp.Status.Status != 0 /*granted*/ {
    return nil, fmt.Errorf("invalid TSR from upstream")
}
// verificar messageImprint coincide con el del TSQ del cliente
```

---

## A09 — Security Logging & Monitoring · 🟠 Parcial

### Hallazgos

- **A09-1 — `audit_events` incompletos**: no se rellenan IP/UA; login/logout/refresh/2fa no se auditan.
- **A09-2 — `User-Agent` sin truncar/sanitizar** antes de persistir (512 chars + escape).
- **A09-3 — Sin detección activa de brute force**: `failed_requests` se puebla pero sin alertas.
- **A09-4 — Sin centralización de logs** (SIEM/Loki/ELK).

### Remediación

1. Middleware que enriquezca `AuditEvent` con IP/UA automáticamente.
2. Auditar `login`, `logout`, `refresh`, `2fa_enable`, `2fa_disable`, `tenant_delete`, `upstream_change`.
3. Job cada 5 min: alerta si una IP supera 50 fallos en 10 min.

---

## A10 — SSRF · 🔴 **Riesgo Alto**

### Hallazgo principal

**A10-1 — Upstream TSA configurable sin validación** (CWE-918, Alta)
Archivos: `handler/admin/config.go:63-117` (Create), `:120-189` (Update), `upstream/client.go:71-109`.

Flujo explotable:
1. Admin (o atacante con admin comprometido) envía `POST /api/admin/v1/config/upstreams {"url":"http://127.0.0.1:5432"}`.
2. Backend guarda sin validar; lo marca como `default`.
3. El próximo `POST /ts` hace que el backend conecte al puerto interno arbitrario (Postgres, Redis, frontend, IMDS cloud).
4. El response vuelve al cliente HTTP con `Content-Type: application/timestamp-reply`.

Vectores: recon de red interna, lectura de metadatos cloud (`http://169.254.169.254/`), amplificación contra servicios internos.

### Remediación (prioritaria)

```go
// handler/admin/config.go — en Create y Update:
u, err := url.Parse(req.URL)
if err != nil || u.Scheme != "https" {
    apierr.WriteValidationError(w, r, map[string]string{"url":"must be https://"})
    return
}
host := u.Hostname()
ips, _ := net.LookupIP(host)
for _, ip := range ips {
    if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
       ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
        apierr.WriteValidationError(w, r, map[string]string{"url":"private addresses not allowed"})
        return
    }
}
// defensa en profundidad: replicar check en upstream/client.go:doRequest justo antes de Do()
```

Alternativa más segura: **lista blanca de dominios** (`TSA_UPSTREAM_ALLOWLIST=timestamp.digicert.com,tsuq.camerfirma.com,...`) en `.env` y validar contra ella.

---

# Parte 2 — Rendimiento y límites de carga

## 2.1 Hardware y utilización actual

| Métrica | Valor | Observación |
|---|---|---|
| CPU | 4 vCPU (Intel Haswell, 1 core/sock × 4) | load avg 0.01 (ocioso) |
| RAM | 7.6 GiB (6.7 GiB libre) | todos los contenedores < 50 MiB |
| Disco | 48 GB (34 % usado) | sin presión |
| Swap | 0 | aceptable con RAM holgada |
| `tsa_backend` | 0–0.58 % CPU, 6–9 MiB RAM | sin pressure ni en bursts |
| `tsa_postgres` | 0–0.07 % CPU, 47 MiB | ídem |
| `tsa_redis` | 0.7 % CPU, 6.6 MiB | ídem |

**Conclusión:** la máquina está sub-utilizada > 95 %. El hardware **no** es el bottleneck.

## 2.2 Límites configurados hoy

| Capa | Parámetro | Valor actual | Archivo |
|---|---|---|---|
| Nginx zona pública | `limit_req_zone tsa_public` | **20 r/s**, bucket 10 MB | `nginx/nginx.conf:64` |
| Nginx /ts /tsp | `burst=100 nodelay` | sostenido 20, pico 100 | `nginx/conf.d/tsa.bigdavi.com.conf:44` |
| Nginx panel admin | 10 r/s burst=30 | OK navegación | `ast.bigdavi.com.conf:43` |
| Nginx login | 5 r/m burst=3 | OK anti brute-force | `ast.bigdavi.com.conf:56` |
| Nginx worker | `auto` (4) × `worker_connections 2048` | 8 192 conexiones totales | `nginx.conf:2-7` |
| Nginx → backend | `keepalive 32` | bajo, conviene subir | `nginx.conf:78` |
| Backend global | `RATE_LIMIT_GLOBAL_RPS=200` | **inerte** (nginx ya topa antes) | `.env` en vivo |
| Backend per-tenant | `burst_per_minute` desde DB (default 60/min) | por tenant | `ratelimit.go:65-68` |
| HTTP upstream pool | `MaxIdleConnsPerHost=20`, **sin `MaxConnsPerHost`** | pool chico | `upstream/client.go:43-46` |
| Upstream timeout | `10s`, `MaxRetries=1` | razonable | `config.go:151` |
| Postgres pool | `MaxOpenConns=25`, `MaxIdleConns=5` | sobra | `config.go:120-121` |
| Redis pool | `REDIS_POOL_SIZE=10` | sobra | `config.go:135` |
| Backend HTTP server | `Read/WriteTimeout=30s`, `IdleTimeout=120s` | OK | `config.go:106-109` |

**Incoherencia 1.** Nginx edge tope sostenido = 20 r/s; backend global = 200 r/s → el `RATE_LIMIT_GLOBAL_RPS=200` **nunca se activa**.

**Incoherencia 2.** El default real en DB es Camerfirma (`tsuq.camerfirma.com:5004`), no DigiCert. El `.env` mantiene un placeholder `upstream-tsa-provider.example.com` — si el default de DB falla o desaparece, el fallback apunta a un host inexistente.

## 2.3 Ruta crítica y bottleneck

`cliente → nginx (TLS + limit_req) → backend (auth → allowlist → RL tenant → quota → HTTP upstream) → respuesta TSR`

**Latencias medidas hoy** (desde el propio servidor contra `https://tsa.bigdavi.com`):

| Tramo | p50 | Método |
|---|---|---|
| `/health` (TLS + nginx + backend) | 95–110 ms | 5 curls seriales |
| `/ts` 401 sin auth (pipeline sin upstream) | 55–60 ms | 10 curls seriales |
| Nginx → backend interno (`/ts` 401) | < 10 ms | wget interno |
| `/ts` con body → upstream (Camerfirma) 4xx → 502 | 540–720 ms | 10 seriales + 20 paralelas |
| 20 concurrentes: 10 completados + 10 rechazados **429 por Nginx** | wall-clock 0.91 s | burst local |

**Bottleneck:** **latencia del upstream externo**. El backend mismo + Redis + PG añaden < 10 ms al camino crítico. CPU del backend con 20 concurrentes: **0.58 %**.

## 2.4 Cálculo de capacidad (Little's law)

Con Go net/http, cada request al upstream es I/O-bound. La concurrencia sostenible = pool idle del transport.

| Upstream | Latencia L | C=20 (actual) | C=64 (recomendado) | C=100 |
|---|---|---|---|---|
| Camerfirma (L=600 ms) | | **33 req/s** | 107 req/s | 167 req/s |
| DigiCert (L=150 ms) | | 133 req/s | 427 req/s | 667 req/s |

En la práctica **no conviene superar 30–50 req/s sostenidos** contra un upstream público externo sin acuerdo previo (riesgo de throttle/ban).

## 2.5 Recomendaciones finales

### Nginx

```nginx
# nginx/nginx.conf
limit_req_zone $binary_remote_addr zone=tsa_public:10m rate=30r/s;  # 20 → 30
upstream backend {
    server backend:8080;
    keepalive 64;   # 32 → 64
}

# nginx/conf.d/tsa.bigdavi.com.conf — /ts y /tsp
limit_req zone=tsa_public burst=60 nodelay;  # burst 100 → 60
```

### Backend `.env` producción

```bash
RATE_LIMIT_GLOBAL_RPS=50          # 200 → 50 (alineado al hardware + upstream)
POSTGRES_MAX_IDLE_CONNS=10        # 5 → 10
REDIS_POOL_SIZE=20                # 10 → 20
TSA_UPSTREAM_URL=http://timestamp.digicert.com   # limpiar placeholder
TSA_UPSTREAM_USERNAME=
TSA_UPSTREAM_PASSWORD=
```

### Backend código — `backend/internal/upstream/client.go:43-46`

```go
transport := &http.Transport{
    MaxIdleConns:        128,
    MaxIdleConnsPerHost: 64,            // 20 → 64
    MaxConnsPerHost:     128,           // nuevo: techo duro
    IdleConnTimeout:     90 * time.Second,
    ForceAttemptHTTP2:   true,          // opcional: DigiCert soporta H2
}
```

### Cuotas por tenant sugeridas

| Perfil | `monthly_limit` | `burst_per_minute` |
|---|---|---|
| Starter / trial | 10 000 | 30 |
| Pro | 100 000 | 120 |
| Enterprise | 1 000 000 | 600 |
| Interno / bigdavi | ilimitado (`hard_limit=false`) | 1 200 |

### Techo recomendado del sistema

| Métrica | Valor | Justificación |
|---|---|---|
| **Sostenido** | **30 req/s = 1 800 req/min ≈ 2.6 M/mes** | techo del upstream externo |
| **Burst corto (<2 s)** | **60 req/s** | permitido por `burst=60` |
| **Concurrencia hacia upstream** | **64** | nuevo `MaxConnsPerHost` |
| **Cuota default nuevo tenant** | **10 000/mes, burst 30/min** | protege cupo global |

## 2.6 Riesgos y supuestos del análisis de rendimiento

**Supuestos.**
- Latencia upstream medida hoy: Camerfirma ~600 ms. DigiCert ~100-150 ms (medido en tests previos, no re-medido).
- Carga real actual << 30 req/s (sistema completamente ocioso).
- Los TSA públicos no tienen SLA contractual — abusarlos puede resultar en throttling silencioso.

**Riesgos.**
- **Placeholder en `TSA_UPSTREAM_URL`** del `.env` productivo: si el default de DB se rompe, el fallback apunta a `upstream-tsa-provider.example.com` (502). Limpiar ya.
- **Redis fail-open** en rate-limit: si Redis cae, ambos limitadores devuelven "allowed". Resiliente pero permite ráfagas durante la caída.
- **Bench no estadístico**: 10-20 curls, no `wrk`/`hey`. Para un número firme, instalar `hey` en el servidor y correr 60 s a concurrencia 20 contra un TSR válido.
- **Sin swap**: un pico de memoria inesperado mata contenedores por OOM sin degradarse. Dado el uso actual (9 MiB backend, 47 MiB PG), habría que subir concurrencia a miles para ver presión.
- **SAN del certificado**: `tsa.bigdavi.com.conf` usa cert emitido para `ast.bigdavi.com-0001`. Confirmar que el cert cubre ambos SANs con `openssl x509 -text`.

---

# Anexos

## Archivos relevantes citados

- `backend/internal/middleware/ratelimit.go`, `ipallowlist.go`, `tsp.go`, `basicauth.go`, `adminauth.go`
- `backend/internal/repository/postgres/basicauth.go`, `upstreams.go`, `noauth.go`, `audit.go`
- `backend/internal/service/auth/service.go`, `proxy/service.go`, `basicauth/service.go`
- `backend/internal/upstream/client.go`
- `backend/internal/handler/admin/config.go`, `credentials.go`, `ipallowlist.go`, `quotas.go`
- `backend/internal/config/config.go`, `server/router.go`
- `nginx/nginx.conf`, `nginx/conf.d/tsa.bigdavi.com.conf`, `nginx/conf.d/ast.bigdavi.com.conf`
- `docker-compose.yml`, `backend/Dockerfile`, `frontend/Dockerfile`
- `frontend/package.json`, `backend/go.mod`

## Referencias OWASP

- OWASP Top 10:2021 — https://owasp.org/Top10/
- CWE list — https://cwe.mitre.org/
- CVE-2025-29927 (Next.js middleware bypass) — https://nvd.nist.gov/vuln/detail/CVE-2025-29927

---

*Informe generado automáticamente. Revisar hallazgos manualmente antes de aplicar cambios en producción.*
