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
    curl \
    gnupg \
    tzdata \
    traceroute \
    nmap \
  && install -d /usr/share/postgresql-common/pgdg \
  && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
  && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" \
    > /etc/apt/sources.list.d/pgdg.list \
  && apt-get update \
  && apt-get install -y --no-install-recommends postgresql-client-18 \
  && ln -sf /usr/lib/postgresql/18/bin/pg_dump /usr/local/bin/pg_dump \
  && ln -sf /usr/lib/postgresql/18/bin/pg_restore /usr/local/bin/pg_restore \
  && ln -sf /usr/lib/postgresql/18/bin/psql /usr/local/bin/psql \
  && apt-get purge -y --auto-remove curl gnupg \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/netquasar .
# CA Supabase (e outros PEMs) — o binário sozinho não inclui data/; necessário para sslrootcert em *.supabase.co
COPY --from=backend /app/data/certs /app/data/certs
EXPOSE 8080
ENV NETQUASAR_HTTP_ADDR=:8080 \
    NETQUASAR_EMBEDDED_UI=true \
    TZ=America/Sao_Paulo \
    PATH="/usr/lib/postgresql/18/bin:/usr/local/bin:/usr/bin:/bin"
CMD ["./netquasar"]
