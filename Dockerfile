# NetQuasar — build multi-stage: Vite (React) → Go embed → imagem mínima Debian.
# Utilização: docker compose build na raiz do repositório.

# syntax=docker/dockerfile:1

FROM node:22-bookworm-slim AS frontend
WORKDIR /build
COPY quasar_frontend/package.json quasar_frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY quasar_frontend/ ./
# Altere com --build-arg CACHEBUST=$(date +%s) se a UI no contentor não atualizar (cache de build).
ARG CACHEBUST=0
RUN echo "ui cachebust=${CACHEBUST}" && npm run build

FROM golang:1.24-bookworm AS backend
WORKDIR /app
COPY quasar_backend/ ./
COPY --from=frontend /build/dist ./internal/embedui/dist
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/netquasar ./cmd/netquasar

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    traceroute \
    nmap \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/netquasar .
# CA Supabase (e outros PEMs) — o binário sozinho não inclui data/; necessário para sslrootcert em *.supabase.co
COPY --from=backend /app/data/certs /app/data/certs
EXPOSE 8080
ENV NETQUASAR_HTTP_ADDR=:8080 \
    NETQUASAR_EMBEDDED_UI=true
CMD ["./netquasar"]
