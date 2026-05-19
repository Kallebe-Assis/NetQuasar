# NetQuasar

Plataforma de **monitoramento e operação de rede** voltada a provedores ISP (NOC). Centraliza inventário, coleta automática (ping, SNMP, OLT, interfaces), alertas, notificações e ferramentas de diagnóstico numa interface web única.

| Componente | Tecnologia | Função |
|------------|------------|--------|
| **Backend** | Go (`quasar_backend`) | API REST, workers de monitoramento, alertas, integrações |
| **Frontend** | React + TypeScript + Vite (`quasar_frontend`) | Painel operacional para o dia a dia do NOC |
| **Dados** | PostgreSQL | Inventário, histórico, alertas, configurações |
| **Cache** | Redis (Compose) | Reservado para cache/realtime futuro |
| **Deploy** | Docker Compose + binário único | API + UI embutida na porta publicada |

---

## O que o sistema faz

### Monitoramento contínuo

- **Ping** — latência e disponibilidade por equipamento.
- **Telemetria SNMP** — KPIs por perfil de equipamento (CPU, memória, temperatura, etc.).
- **Interfaces** — snapshot periódico de status, tráfego e contadores; suporte a equipamentos com centenas de interfaces (ex.: MikroTik).
- **OLT** — snapshot de PONs/ONUs, potências, totais online/offline; atualização manual ou em massa com timeouts configuráveis.
- **Workers** — ciclos de coleta respeitam intervalos e **timeouts definidos em Configurações** (telemetria, snapshot de interfaces, fase OLT).

### Alertas e notificações

- Regras e limiares com severidade operacional.
- Fechamento automático quando a condição normaliza.
- Integração **Telegram** (teste de envio e templates nas configurações).

### Operação e relatórios

- **Dashboard** — visão consolidada com carregamento progressivo.
- **Equipamentos** — cadastro, filtros, relatório completo (CSV/PDF), histórico de ping/telemetria/interfaces.
- **OLT** — detalhe por OLT, PONs, relatórios mensais de ONU, refresh snapshot/interfaces.
- **MikroTik / interfaces** — listagem ampla, gráficos de tráfego, tempo real.
- **Mapa** — visão geográfica de POPs e equipamentos.
- **Base comercial** — localidades e registos mensais (clientes, churn, etc.).
- **Ferramentas** — DNS, SNMP walk/get, Telnet, matriz de latência HTTP, utilitários MikroTik.
- **Eventos e métricas** — histórico operacional e painéis auxiliares.

### Governança

- **Autenticação** — login na UI; opcionalmente chaves de API (`NETQUASAR_API_KEYS`).
- **Perfis** — rotas administrativas (configurações) restritas a administradores.
- **Auditoria** — registo de alterações relevantes (equipamentos, utilizadores, Telegram, intervalos, base comercial, etc.) consultável em Configurações.

---

## Estrutura do repositório

```text
NetQuasar/
├── quasar_backend/          # API Go, workers, migrações, MIBs
│   ├── cmd/netquasar/       # Servidor principal
│   ├── cmd/migrate/         # Migrações de base de dados
│   ├── cmd/dbping/          # Teste de ligação PostgreSQL
│   └── internal/            # api, monitorworker, probing, snmp, db, …
├── quasar_frontend/         # SPA React (Vite)
├── deploy/linux-debian/     # Guia de deploy em Debian/VM
├── scripts/                 # Utilitários (ex.: verify-compose-env.sh)
├── docker-compose.yml       # Stack: netquasar + postgres + redis
├── Dockerfile               # Build multi-stage (UI embutida)
├── .env.example             # Modelo de variáveis na raiz
└── ROADMAP-ARQUITETURAL-DEPLOY.md
```

Documentação complementar:

- [Backend](quasar_backend/README-BACKEND.md) — módulos, API, Supabase/SSL, variáveis.
- [Frontend](quasar_frontend/README-FRONTEND.md) — stack e áreas da UI.
- [Deploy Linux/Debian](deploy/linux-debian/README.md) — produção em VM/Docker.
- [Roadmap arquitetural](ROADMAP-ARQUITETURAL-DEPLOY.md) — visão de evolução da plataforma.

---

## Requisitos

| Ambiente | Versão sugerida |
|----------|-----------------|
| Go | 1.22+ (ver `go.mod`) |
| Node.js | 20+ LTS |
| PostgreSQL | 16 (local via Compose ou Supabase) |
| Docker + Compose | Para deploy containerizado |

---

## Desenvolvimento local

### 1. Base de dados

**Opção A — Docker Compose (recomendado para dev completo)**

```bash
cp .env.example .env
# Edite POSTGRES_PASSWORD e outras variáveis
docker compose up -d postgres redis
```

**Opção B — Supabase / Postgres externo**

Configure `NETQUASAR_DATABASE_URL` no `.env` da raiz ou em `quasar_backend/.env`. Para Supabase no Docker, prefira o **Session pooler** (IPv4); detalhes em [README-BACKEND](quasar_backend/README-BACKEND.md).

### 2. Backend

```powershell
cd quasar_backend
# Copie/ajuste .env com NETQUASAR_DATABASE_URL ou variáveis DB_*
go run ./cmd/migrate/    # primeira vez ou após pull com novas migrações
go run ./cmd/netquasar/
```

Com `NETQUASAR_EMBEDDED_UI=false` (omissão em dev), o Go serve **apenas a API**. A UI vem do Vite.

### 3. Frontend

```powershell
cd quasar_frontend
npm install
npm run dev
```

Atalho no Windows: `iniciar-netquasar-dev.bat` (abre backend e frontend em janelas separadas).

### Endereços locais

| Serviço | URL |
|---------|-----|
| Frontend (dev) | http://localhost:5173 |
| API / health | http://localhost:8080 |
| UI embutida (produção/Docker) | http://localhost:8080 |

### Comandos úteis

```powershell
# Frontend
cd quasar_frontend
npm run typecheck
npm run build

# Backend
cd quasar_backend
go build ./...
go run ./cmd/dbping/     # testar ligação à base
```

---

## Deploy com Docker (produção)

1. Copie `.env.example` para `.env` na raiz.
2. Defina `POSTGRES_PASSWORD` (e opcionalmente `NETQUASAR_SESSION_SECRET`, `NETQUASAR_API_KEYS`).
3. Verifique o ficheiro: `bash scripts/verify-compose-env.sh`
4. Suba o stack:

```bash
docker compose up -d --build
```

O serviço `netquasar` publica **API + interface** na porta `NETQUASAR_PUBLISH_PORT` (padrão `8080`), com `NETQUASAR_EMBEDDED_UI=true` e PostgreSQL/Redis na mesma rede Compose.

Guia passo a passo para Debian/Proxmox: [deploy/linux-debian/README.md](deploy/linux-debian/README.md).

---

## Configuração inicial (primeira utilização)

1. Aceda à UI (`/login` ou setup se a base estiver vazia).
2. **Configuração da base** — `/config-setup` ou assistente de cliente (`/client-setup`) conforme o estado da instalação.
3. **Configurações** (admin) — intervalos de monitoramento, timeouts de coleta, Telegram, perfis OLT, utilizadores.
4. **Monitoramento** — inicie o motor de coleta e cadastre equipamentos/POPs.
5. Ajuste **alertas** e regras conforme a operação do ISP.

Principais variáveis (ver `.env.example`):

| Variável | Descrição |
|----------|-----------|
| `NETQUASAR_DATABASE_URL` | DSN PostgreSQL (opcional se usar host `postgres` no Compose) |
| `NETQUASAR_SESSION_SECRET` | JWT de sessão após login (recomendado em produção) |
| `NETQUASAR_API_KEYS` | Chaves API (cabeçalho `X-API-Key`) |
| `NETQUASAR_PUBLISH_PORT` | Porta HTTP no hospedeiro (Compose) |
| `NETQUASAR_LOG_LEVEL` | `trace` … `error` |

---

## Mapa da interface (rotas)

| Rota | Descrição |
|------|-----------|
| `/dashboard` | Painel principal |
| `/monitoring` | Estado do monitoramento e ações globais |
| `/devices` | Inventário e relatórios por equipamento |
| `/olt` | OLTs, PONs, relatórios |
| `/mikrotik` | Interfaces e tráfego (ex-MikroTik/BNG) |
| `/pops` | Pontos de presença |
| `/alerts` | Alertas ativos e histórico |
| `/commercial` | Base comercial |
| `/map` | Mapa operacional |
| `/tools` | Ferramentas de rede |
| `/events` | Eventos |
| `/metrics` | Métricas |
| `/realtime` | Visualização em tempo real |
| `/settings` | Configurações e auditoria (admin) |

---

## Arquitetura (resumo)

```text
┌─────────────────────────────────────────────────────────┐
│  Browser  →  React (Vite dev ou UI embutida no Go)      │
└───────────────────────────┬─────────────────────────────┘
                            │ REST /api/v1/…
                            ▼
┌─────────────────────────────────────────────────────────┐
│  netquasar (Go)                                         │
│  · Handlers HTTP (internal/api)                         │
│  · monitorworker — ping, SNMP, OLT, interfaces, alertas │
│  · probing / snmp — sondas e parsing                    │
└───────────────┬─────────────────────┬───────────────────┘
                ▼                     ▼
         PostgreSQL              Redis (futuro)
```

Workers de interface e OLT leem `monitoring_intervals` na base (incluindo `interface_snapshot_timeout_ms`), alinhando coleta automática e refresh manual.

---

## Segurança e boas práticas

- Não commite `.env`, credenciais nem certificados privados (ex.: ficheiros `*.crt` locais na raiz).
- Use palavras-passe fortes em `POSTGRES_PASSWORD` e `NETQUASAR_SESSION_SECRET`.
- Em produção, restrinja acesso à porta publicada (firewall/reverse proxy com TLS).
- Codifique caracteres especiais na palavra-passe do DSN (`%2C`, `%24`, etc.).

---

## Licença e contribuição

Consulte os ficheiros de licença no repositório (se existirem). Para evolução técnica de longo prazo, veja [ROADMAP-ARQUITETURAL-DEPLOY.md](ROADMAP-ARQUITETURAL-DEPLOY.md).

---

## Resolução rápida de problemas

| Sintoma | O que verificar |
|---------|-----------------|
| Frontend não fala com API | Proxy Vite / `VITE_API` ou mesma origem em produção |
| `password authentication failed` | DSN, palavra-passe codificada, credenciais Supabase |
| Docker não liga ao Supabase `db.*.supabase.co` | Usar **Session pooler** (IPv4) — ver README-BACKEND |
| Interfaces incompletas no MikroTik | Timeout em Configurações; aviso de walk truncado na UI |
| Migrações em falta | `go run ./cmd/migrate/` no `quasar_backend` |

Para diagnóstico de base: `go run ./cmd/dbping/` dentro de `quasar_backend`.
