# =============================================================
# Backend Go — Multi-stage build
# =============================================================

# ── Build stage ──────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copiar go.mod primero, descargar dependencias (cache layer)
COPY go.mod ./
RUN go mod download

# Copiar código fuente completo
COPY . .

# Generar go.sum y verificar dependencias
RUN go mod tidy

# Build optimizado para producción
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev}" \
    -o /app/tsa-proxy \
    ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /

COPY --from=builder /app/tsa-proxy /tsa-proxy
COPY --from=builder /app/migrations /migrations

EXPOSE 8080

ENTRYPOINT ["/tsa-proxy"]
