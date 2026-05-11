# NetQuasar — Roadmap Arquitetural e Estrutura Profissional de Deploy

## Objetivo

Transformar o NetQuasar em uma plataforma:

- portátil
- escalável
- distribuível
- fácil de atualizar
- fácil de instalar
- preparada para crescimento enterprise

Foco principal:

- runtime unificado
- backend Go compilado
- frontend React embedado
- PostgreSQL externo
- Redis para realtime/cache
- distribuição via Docker Compose

---

## Arquitetura Final Desejada

```text
                        ┌────────────────────┐
                        │     Frontend       │
                        │ React + Vite Build │
                        └─────────┬──────────┘
                                  │
                                  ▼
                    ┌──────────────────────────┐
                    │      NetQuasar Core      │
                    │        (Go Binary)       │
                    │                          │
                    │ API REST                │
                    │ WebSocket               │
                    │ Scheduler               │
                    │ Pollers                 │
                    │ Alert Engine            │
                    │ Discovery Engine        │
                    │ Telegram Integrations   │
                    │ SNMP/Telnet/SSH         │
                    └─────────┬────────────────┘
                              │
               ┌──────────────┴──────────────┐
               ▼                             ▼
        ┌────────────┐               ┌────────────┐
        │ PostgreSQL │               │   Redis    │
        └────────────┘               └────────────┘
```

---

## Estrutura Física Recomendada

```text
netquasar/
│
├── docker-compose.yml
├── .env
├── README.md
│
├── backend/
│   ├── cmd/
│   │   └── netquasar/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/
│   │   ├── auth/
│   │   ├── websocket/
│   │   ├── scheduler/
│   │   ├── pollers/
│   │   ├── snmp/
│   │   ├── telnet/
│   │   ├── ssh/
│   │   ├── alerts/
│   │   ├── discovery/
│   │   ├── telemetry/
│   │   ├── integrations/
│   │   ├── repositories/
│   │   ├── services/
│   │   └── models/
│   ├── migrations/
│   ├── data/
│   ├── mibs/
│   └── Dockerfile
│
├── frontend/
│   ├── src/
│   ├── public/
│   ├── dist/
│   ├── package.json
│   └── vite.config.js
│
├── scripts/
├── backups/
└── docs/
```

---

## Backend Go — Núcleo Operacional

Responsável por:

- servir frontend
- expor APIs
- realtime
- polling
- discovery
- alertas
- integrações

### Organização por Domínio

```text
internal/
├── devices/
├── pops/
├── interfaces/
├── alerts/
├── telemetry/
├── polling/
├── maps/
├── olt/
└── auth/
```

Cada domínio:

```text
devices/
├── handler.go
├── service.go
├── repository.go
├── model.go
└── routes.go
```

---

## Frontend React

Objetivo: build estático.

```bash
npm run build
```

Gera:

```text
frontend/dist
```

### Embed no Go

```go
//go:embed all:frontend/dist
var frontend embed.FS
```

```go
func SetupFrontend() {
    fs := http.FS(frontend)
    http.Handle("/", http.FileServer(fs))
}
```

Resultado: um único runtime `netquasar` (sem Node.js/Nginx/PM2 em produção).

---

## Redis — Papel no Sistema

Redis obrigatório para:

- cache de SNMP
- realtime
- filas internas
- eventos
- websocket broadcast
- alert suppression
- debounce
- rate limit

Exemplos de chaves:

```text
traffic:{device}:{ifindex}
alerts:active
telemetry:cache
```

---

## PostgreSQL

Banco principal do sistema para:

- usuários
- dispositivos
- histórico
- eventos
- configurações
- inventário
- métricas

Recomendação: usar `pgx`/`sqlx` e evitar ORM pesado.

---

## Polling Architecture

```text
Scheduler
    ↓
Polling Queue
    ↓
Workers
    ↓
SNMP/Telnet/ICMP
    ↓
Redis Cache
    ↓
PostgreSQL
    ↓
WebSocket Broadcast
```

### Tipos de poller

- **Interface Traffic Poller** (`ifHCInOctets`, `ifHCOutOctets`) — 1s
- **Device Health Poller** (CPU/mem/temp) — 5 a 30s
- **Optical Poller** (RX/TX power) — 30s

Regra crítica SNMP:

- Nunca: `1 request HTTP => 1 SNMP GET`
- Sempre: pollers contínuos em background
- Preferir `snmpbulkwalk` e evitar `snmpget` por interface

---

## Discovery Engine

Fluxo:

```text
Discovery Job
    ↓
SNMP Walk
    ↓
Detecta interfaces
    ↓
Detecta sensores
    ↓
Detecta OLTs
    ↓
Cria entidades automaticamente
```

Estrutura:

```text
internal/discovery/
├── interfaces.go
├── optical.go
├── olt.go
├── wireless.go
└── templates.go
```

---

## Realtime Architecture

```text
Poller
    ↓
Redis
    ↓
WebSocket
    ↓
React
```

Tecnologias:

- Backend: Gorilla WebSocket ou Fiber WebSocket
- Frontend: Zustand ou Redux Toolkit

Cadência ideal de atualização:

- 500ms a 2s

---

## Sistema de Alertas

Pipeline:

```text
Alert Rule
    ↓
Threshold Engine
    ↓
Suppression Engine
    ↓
Notification Dispatcher
```

Suporte esperado:

- severidade
- cooldown
- debounce
- maintenance mode
- supressão
- agrupamento

Canais:

- Telegram
- Discord
- Webhook
- Email

---

## Dockerização

### Dockerfile Backend (base)

```dockerfile
FROM golang:1.24 AS builder

WORKDIR /app

COPY . .

RUN go build -o netquasar ./cmd/netquasar

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/netquasar .

CMD ["./netquasar"]
```

### Docker Compose (base)

```yaml
services:
  netquasar:
    image: netquasar/core:latest
    restart: always
    ports:
      - "8080:8080"
    env_file:
      - .env
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:16
    restart: always
    environment:
      POSTGRES_DB: netquasar
      POSTGRES_USER: quasar
      POSTGRES_PASSWORD: quasar
    volumes:
      - ./data/postgres:/var/lib/postgresql/data

  redis:
    image: redis:7
    restart: always
    volumes:
      - ./data/redis:/data
```

---

## Deploy Final

### Instalação

```bash
git clone ...
cd netquasar
docker compose up -d
```

### Atualização

```bash
docker compose pull
docker compose up -d
```

### Backup

- PostgreSQL via `pg_dump`
- ou cópia de volume `data/postgres`

### Logs

```text
logs/
├── api.log
├── poller.log
├── websocket.log
├── alerts.log
└── discovery.log
```

---

## Observabilidade Interna

Monitorar:

- tempo de polling
- tempo SNMP
- devices online
- queue lag
- websocket clients
- consumo de memória
- goroutines

---

## Sistema de Plugins (Futuro)

```text
plugins/
├── mikrotik/
├── olt_huawei/
├── olt_zte/
└── ubiquiti/
```

---

## Escalabilidade Futura

Modelo inicial:

- 1 servidor (~5k interfaces)

Próximo nível:

- pollers distribuídos
- core + poller nodes (estilo Zabbix Proxy / LibreNMS Dispatcher)

---

## Segurança

Recomendado:

- JWT
- RBAC
- TLS
- Audit Log
- Rate limiting
- Secrets via env

---

## Roadmap Recomendado

### Fase 1 — Estrutura Base

- embed frontend
- docker compose
- Redis
- PostgreSQL
- WebSocket

### Fase 2 — Polling Profissional

- pollers assíncronos
- bulk SNMP
- cache
- discovery

### Fase 3 — Realtime

- streaming de interfaces
- dashboards realtime
- mapas realtime

### Fase 4 — Enterprise

- HA pollers
- clustering
- plugins
- multi-tenant
- licenciamento

---

## Resultado Final Esperado

NetQuasar como:

- runtime único
- distribuível via Docker
- simples de instalar
- realtime
- modular
- escalável
- orientado a ISP/WISP

Fluxo operacional alvo:

```bash
docker compose up -d
```

Acesso:

```text
http://IP:8080
```

