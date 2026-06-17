# NetQuasar

Plataforma de **monitoramento e operação de rede** para provedores ISP (NOC). Centraliza inventário, coleta automática (ping, SNMP, OLT, interfaces), alertas, notificações, integrações com sistemas externos e ferramentas de diagnóstico numa interface web única.

| Componente | Tecnologia | Função |
|------------|------------|--------|
| **Backend** | Go (`quasar_backend`) | API REST, workers de monitoramento, alertas, integrações |
| **Frontend** | React + TypeScript + Vite (`quasar_frontend`) | Painel operacional para o dia a dia do NOC |
| **Dados** | PostgreSQL | Inventário, histórico, alertas, configurações, auditoria |
| **Cache / tempo real** | Redis (opcional) | Canal WebSocket para atualizações em tempo real |
| **Deploy** | Docker Compose + binário único | API + UI embutida na porta publicada |

Documentação complementar: [Backend](quasar_backend/README-BACKEND.md) · [Frontend](quasar_frontend/README-FRONTEND.md) · [Deploy Debian](deploy/linux-debian/README.md) · [Roadmap](ROADMAP-ARQUITETURAL-DEPLOY.md)

---

## Arquitetura

```text
┌──────────────────────────────────────────────────────────────────┐
│  Browser  →  React (Vite em dev ou UI embutida no Go em prod.)   │
└────────────────────────────┬─────────────────────────────────────┘
                             │ REST /api/v1/…  (+ WebSocket /realtime/ws)
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│  netquasar (Go)                                                  │
│  · internal/api        — handlers HTTP por domínio               │
│  · internal/monitorworker — ping, SNMP, OLT, interfaces, alertas │
│  · internal/alertthresholds / alertignore / alertverify          │
│  · internal/probing / snmp* / oltcollect — sondas e parsing      │
│  · internal/alertnotify — Telegram e resolução de alertas          │
│  · internal/alertcorrelation — incidentes (POP/OLT em cascata)   │
└───────────────┬──────────────────────────────┬───────────────────┘
                ▼                              ▼
         PostgreSQL                      Redis (tempo real)
```

O **worker de monitoramento** (`monitorworker.Run`) corre em goroutine dedicada, lê o estado em `monitoring_runtime` (ligado/desligado, modo) e dispara ciclos conforme os intervalos em `monitoring_intervals`. Cada alteração relevante actualiza timestamps em `monitoring_runtime` para o frontend invalidar caches (polling + `useMonitoringLiveSync`).

---

## Motor de monitoramento

### Modos

| Modo | Comportamento |
|------|----------------|
| `off` | Worker inactivo |
| `simple_ping` | Apenas ciclo de latência/ICMP-TCP (`ping_seconds`) |
| `full` | Ping + telemetria SNMP + snapshots de interfaces + coleta OLT/PON |

Ligação/desligação: **Monitoramento** na UI ou `POST /api/v1/monitoring/start` / `stop` (admin).

### Ciclos automáticos

| Ciclo | Intervalo (config) | O que faz |
|-------|-------------------|-----------|
| **Latência / ping** | `ping_seconds` | Para cada equipamento monitorizado: ICMP e fallback TCP; grava `device_probe_cache` (`reach_ok`, `latency_ms`); abre/fecha alerta `ping_unreachable`; histórico em `ping_history` |
| **Telemetria SNMP** | `telemetry_seconds` | Walk/get conforme perfil do equipamento; amostras em `telemetry_samples` (CPU, memória, temperatura, uptime); avalia limiares globais → alertas `telemetry_threshold` e `uptime_restart_low` |
| **Interfaces (IF-MIB)** | `interface_snapshot_seconds` | Walk IF-MIB (+ IF-MIB-X); grava `interface_snapshots`; MikroTik: potências SFP e alertas ópticos; detecta transição UP→DOWN → `interface_down_transition` |
| **OLT PON / ONUs** | `olt_if_derived_pon_seconds` | Por OLT online com telemetria activa: colecta contagem de ONUs por PON, actualiza `olt_snapshots`, deriva status PON (ON se ≥1 ONU online), avalia alertas de queda/subida de ONUs e potência óptica |

Cada ciclo tem **timeout** próprio (`telemetry_timeout_ms`, `interface_snapshot_timeout_ms`, `olt_if_derived_pon_timeout_ms`) configurável em **Configurações → Monitoramento**.

Execução manual de um ciclo: `POST /api/v1/monitoring/cycles/{latency|telemetry|interfaces|olt-if-derived}` (admin, opcional `force=true`).

### Coleta OLT — como funciona

1. **OLT com derive IF-MIB** (marcas compatíveis, exclui ZTE/Datacom/VSOL): walk IF-MIB, deriva PONs/ONUs (`oltifderive`), estabiliza vs. snapshot anterior, grava `olt_snapshots`.
2. **OLT por perfil de fabricante** (VSOL, ZTE, etc.): lê `olt_vendor_models` (passos SNMP/telnet); executa `onu_metrics_collect` ou `onu_snmp_walk`; grava o mesmo `olt_snapshots`.
3. **Refresh manual** na tela OLT: `POST /olt/devices/{id}/refresh` executa o perfil completo do modelo (scope `full` ou `onu`).

A UI OLT e o Dashboard leem `olt_snapshots` e actualizam-se via polling + sinalização de `monitoring_runtime.activity_updated_at`.

### Coleta nocturna

Configurável em **Monitoramento → Coleta nocturna**: janela horária para ciclos mais pesados sem sobrecarregar o horário comercial (`PATCH /monitoring/nightly-collection`).

---

## Sistema de alertas

### Tipos de alerta

| Tipo | Origem | Condição típica |
|------|--------|-----------------|
| `ping_unreachable` | Worker ping | Equipamento sem resposta ICMP/TCP |
| `latency_high` | Worker ping | Latência acima do limiar global (`latency_ms`) |
| `uptime_restart_low` | Telemetria SNMP | Uptime abaixo do mínimo (possível reinício) |
| `telemetry_threshold` | Telemetria SNMP | CPU, memória, temperatura, etc. fora do limiar |
| `interface_down_transition` | Snapshot interfaces | Interface mudou de UP para DOWN |
| `mikrotik_sfp_tx` / `mikrotik_sfp_rx` | Snapshot interfaces MikroTik | Potência óptica SFP fora do limiar (dBm) |
| `olt_onu_drop` / `olt_onu_rise` | Coleta OLT | Queda ou subida de ONUs online por PON (contagem ou %) |
| `olt_onu_rx` / `olt_onu_tx` | Coleta OLT | Potência óptica PON/ONU abaixo do limiar |

Limiares globais: **Configurações → Monitoramento → Alertas** (regra «Limiar global de alertas» em `alert_rules`). Severidade: `info`, `warning`, `critical`.

### Ciclo de vida de um alerta

1. **Detecção** — worker ou refresh manual compara métrica com limiar.
2. **Criação** — `INSERT` em `alert_instances` se não existir alerta aberto do mesmo padrão (`device_id` + `alert_type` + `meta.key`).
3. **Actualização** — mesma condição mantém o alerta aberto e actualiza `message` / `meta` (ex.: latência 243→210 ms).
4. **Notificação** — `alertnotify.SendMonitoringTelegramAndPatchMeta` envia Telegram (config «monitoring») e regista resultado em `meta.telegram`.
5. **Resolução** — quando a condição normaliza, `closed_at` é preenchido e Telegram de resolução é enviado.

### Incidentes correlacionados

`alertcorrelation` agrupa alertas por causa provável:

- **POP offline** — vários equipamentos do mesmo POP sem ping.
- **OLT offline** — OLT offline com efeito em cascata nas ONUs.

Visíveis em **Alertas → Incidentes correlacionados**. Telegram de cascata é suprimido para evitar spam.

### Ignorar, verificar e suprimir

| Acção | Função |
|-------|--------|
| **Ignorar alerta** | Persiste em `alert_ignores` (equipamento + tipo + chave PON/interface/métrica); fecha alertas abertos do padrão; bloqueia novos alertas na UI **e** no Telegram |
| **Verificar** | Reavalia a condição (ping, latência, snapshot SFP, OLT, etc.) e actualiza valor na lista ou fecha se normalizado |
| **Verificar alertas** (global) | Recalcula pings + reverifica até 250 alertas abertos |
| **Alertas ignorados** | Modal com lista completa; opção **Reactivar** remove o silêncio |
| **Supressões** (`alert_suppressions`) | Filtro por scope POP/global na listagem (legado; ignorar por equipamento é o modelo preferido) |

### Revalidação

`POST /alerts/revalidate` — fecha `ping_unreachable` obsoletos quando o probe já está OK.

---

## Módulos da interface

### Dashboard (`/dashboard`)

**Função:** visão executiva da rede.

**Como funciona:** agrega dados de `device_probe_cache`, `alert_instances`, `olt_snapshots` e endpoints `/dashboard/analytics`, `/dashboard/olt-capacity`, `/dashboard/data-gaps`, `/overview/top-latency`. Carregamento progressivo com cache (`dashboardCache`). Mostra totais de ONUs online/offline em todas as OLTs, alertas críticos e lacunas de coleta.

---

### Monitoramento (`/monitoring`)

**Função:** painel de controlo do motor de coleta.

**Como funciona:** lê `GET /monitoring/state` (actividade actual, últimos ciclos, `is_running`). Permite iniciar/parar monitoramento, ver equipamentos activos (`/monitoring/active-equipment`), disparar ciclos manuais e configurar coleta nocturna. Indicador global no menu reflecte `current_activity` do worker.

---

### Tempo real (`/realtime`)

**Função:** latência e estado de reachability em fluxo contínuo.

**Como funciona:** WebSocket `GET /realtime/ws` (broker Redis quando configurado) ou polling `GET /realtime/ping`. Actualiza lista de equipamentos sem esperar o intervalo completo do worker.

---

### Integrações (`/integrations`)

**Função:** ligação a ERPs, CRMs e APIs externas do ISP.

**Como funciona:** cada integração tem URL base, autenticação e **pedidos** configuráveis (templates HTTP). O motor `integrationhttp` executa pedidos; `integrationconsumer` expõe acções de consulta (cliente, OS, login PPPoE). Logs em `integration_logs`. Uso típico: pesquisar cliente por CPF/login a partir da tela **Conexões**.

---

### POPs (`/pops`)

**Função:** pontos de presença (sites físicos).

**Como funciona:** CRUD em `pops`; associação em massa de equipamentos; contactos por POP; coordenadas usadas no **Mapa** e na correlação de incidentes «POP offline».

---

### Equipamentos (`/devices`)

**Função:** inventário central da rede.

**Como funciona:** cadastro com categoria (OLT, MikroTik, router, etc.), IP, SNMP community, POP, coordenadas, `max_pons` (OLT). Importação CSV. Por equipamento:

- **Relatório** — modal com ping, telemetria, interfaces, alertas (export CSV/PDF).
- **Coleta manual** — ping, telemetria SNMP, refresh interfaces.
- **SNMP walk** — descoberta de OIDs / inventário.
- **Backup de config** — texto guardado em `device_config_backups`.

Estado operacional vem de `device_probe_cache` actualizado pelo worker.

---

### Clientes (`/commercial`)

**Função:** base comercial mensal (localidades, clientes activos, churn).

**Como funciona:** `commercial_localities` e `commercial_monthly_records`; agregados e comparação mês a mês; exportação de relatórios; envio Telegram opcional. Dados introduzidos manualmente ou importados — não dependem da coleta SNMP.

---

### Conexões (`/connections`)

**Função:** cadastro de assinantes PPPoE/DHCP com localização.

**Como funciona:** tabela `client_connections` (login, cliente, plano, coordenadas, CTO/porta, médio fibra/rádio/UTP). Importação CSV em lote; pesquisa com debounce; integração com ERP (`integration-lookup` por login). Pontos aparecem no **Mapa** (switch «Logins no mapa», carga por bounding box para performance).

---

### Alertas (`/alerts`)

**Função:** fila operacional de problemas activos e histórico.

**Como funciona:** lista `GET /alerts/active` (exclui ignorados); filtros por severidade e tipo; estatísticas 24 h; incidentes correlacionados; menu ⋮ por linha (Verificar / Ignorar); botões **Verificar alertas** e **Alertas ignorados**. Histórico em `/alerts/history`. Refresh automático ~2,5 s com a página aberta.

---

### Mapa (`/mapa` → `/map`)

**Função:** visualização geográfica.

**Como funciona:** `GET /map/equipment-points` (equipamentos com lat/lng); `GET /map/connection-points` (logins, filtrado por bbox). Modos: agrupado, disperso, online/offline (pins coloridos). Equipamentos usam pin Leaflet padrão; logins usam marcador circular azul distinto. Lista paginada e painel de detalhe lateral.

---

### Ferramentas (`/tools`)

**Função:** diagnóstico ad hoc (sem persistir inventário).

**Como funciona:** executa no servidor (com auditoria em `ops_audit_log`):

| Ferramenta | Endpoint |
|------------|----------|
| DNS | `POST /tools/dns/run` |
| HTTP/HTTPS probe | `POST /tools/http-https-probe` |
| Ping ICMP | `POST /tools/icmp/ping` |
| Traceroute | `POST /tools/tracert` |
| Nmap | `POST /tools/nmap` |
| SNMP get / bulk / walk | `POST /tools/snmp/*` |
| Telnet / SSH teste | `POST /tools/telnet/test`, `/ssh/test` |
| MikroTik rápido | `POST /tools/mikrotik/*` |

---

### OLT (`/olt`)

**Função:** monitorização de PONs e ONUs.

**Como funciona:** lista OLTs com snapshot (`olt_snapshots`: `pons`, `summary`, totais computados). Detalhe por OLT: tabela PON (status ON/OFF derivado de ONUs online), ONUs VSOL/ZTE, interfaces, log de coleta SNMP. **Actualizar** dispara refresh pelo perfil do fabricante. Relatórios mensais de ONU (histórico e export). Totais globais de ONUs online/offline no dashboard e nesta tela. Coleta periódica via worker (intervalo em configurações).

---

### MikroTik (`/mikrotik`)

**Função:** interfaces, tráfego e sessões PPPoE.

**Como funciona:** lista equipamentos MikroTik; detalhe com tabela de interfaces (status, tráfego, SFP dBm), gráficos de taxa, coleta manual. Sessões PPPoE via API BNG (`/bng/sessions`). Perfil de coleta SNMP configurável em **Configurações → MikroTik**. Alertas SFP gerados automaticamente no ciclo de interfaces.

---

### Eventos (`/events`)

**Função:** linha do tempo operacional.

**Como funciona:** `GET /events` — eventos agregados (alterações, coletas, alertas) para consulta histórica rápida.

---

### Configurações (`/settings`) — admin

| Secção | Função |
|--------|--------|
| **Base de dados** | DSN, teste de ligação, logs |
| **Utilizadores** | CRUD, perfil admin/operador |
| **Monitoramento** | Intervalos, timeouts, modo, limiares de alerta |
| **OLT vendors** | Perfis SNMP/telnet por marca/modelo |
| **MikroTik collection** | OIDs e passos de coleta |
| **Telegram** | Bot token, chat monitoring e relatórios, teste de envio |
| **SMTP** | Servidor para relatórios por e-mail |
| **Automações** | Relatório ONU mensal, digest de alertas, relatório comercial |
| **Aparência** | Tema/cores da UI |
| **Auditoria** | `ops_audit_log` — quem alterou o quê |

---

## Notificações e automações

### Telegram

- **Monitoring** — cada alerta novo e resolução (quando configurado).
- **Relatórios** — ONU mensal, resumo de alertas (diário/semanal), totais comerciais.

### Relatório ONU mensal

Scheduler verifica `automation_onu_monthly_report`; percorre OLTs, colecta, gera resumo e envia Telegram (e regista execução em `automation_runs`).

### Digest de alertas

Contagens por tipo/severidade e incidentes abertos; envio agendado por Telegram e/ou SMTP.

---

## Autenticação e API

- **Login UI** — `POST /auth/login` → JWT em sessão (`NETQUASAR_SESSION_SECRET`).
- **API keys** — cabeçalho `X-API-Key` (`NETQUASAR_API_KEYS`).
- Rotas administrativas (configurações, mutações, coletas manuais) exigem perfil **admin**.

Health: `GET /health` · Métricas Prometheus: `GET /metrics`

---

## Estrutura do repositório

```text
NetQuasar/
├── quasar_backend/
│   ├── cmd/netquasar/       # Servidor HTTP + worker
│   ├── cmd/migrate/         # Migrações SQL (goose)
│   ├── internal/
│   │   ├── api/             # Handlers REST
│   │   ├── monitorworker/   # Ciclos de coleta
│   │   ├── alertthresholds/ # Limiares e avaliação
│   │   ├── alertignore/     # Ignorar alertas (persistência)
│   │   ├── alertverify/     # Verificação manual
│   │   ├── alertnotify/     # Telegram
│   │   ├── alertcorrelation/# Incidentes
│   │   ├── oltcollect/      # Perfis e coleta OLT
│   │   ├── oltifderive/     # Derivação PON/ONU via IF-MIB
│   │   └── db/migrations/   # Esquema PostgreSQL
│   └── data/mibs/           # MIBs SNMP de referência
├── quasar_frontend/         # SPA React
├── deploy/linux-debian/
├── docker-compose.yml
└── Dockerfile
```

---

## Desenvolvimento local

### Requisitos

| Ferramenta | Versão |
|------------|--------|
| Go | 1.22+ |
| Node.js | 20+ LTS |
| PostgreSQL | 16 |
| Docker (opcional) | Compose para postgres + redis |

### Arranque rápido

```powershell
# Base (Compose)
cp .env.example .env
docker compose up -d postgres redis

# Backend
cd quasar_backend
go run ./cmd/migrate/
go run ./cmd/netquasar/

# Frontend (outro terminal)
cd quasar_frontend
npm install
npm run dev
```

| Serviço | URL |
|---------|-----|
| Frontend (dev) | http://localhost:5173 |
| API | http://localhost:8080 |

Atalho Windows: `iniciar-netquasar-dev.bat`

### Comandos úteis

```powershell
cd quasar_frontend && npm run typecheck && npm run build
cd quasar_backend && go build ./... && go test ./... -short
cd quasar_backend && go run ./cmd/dbping/
```

---

## Deploy com Docker

```bash
cp .env.example .env
# Editar POSTGRES_PASSWORD, NETQUASAR_SESSION_SECRET, etc.
bash scripts/verify-compose-env.sh
docker compose up -d --build
```

UI + API na porta `NETQUASAR_PUBLISH_PORT` (padrão `8080`), com `NETQUASAR_EMBEDDED_UI=true`.

Guia completo: [deploy/linux-debian/README.md](deploy/linux-debian/README.md)

---

## Configuração inicial

1. Aceder à UI → login ou `/config-setup` se a base estiver vazia.
2. **Configurações** (admin) — intervalos, Telegram, perfis OLT, utilizadores.
3. Cadastrar **POPs** e **Equipamentos** (com IP e SNMP).
4. **Monitoramento** → iniciar modo **Full**.
5. Ajustar **limiares de alerta** em Configurações.
6. (Opcional) Importar **Conexões** CSV e activar logins no mapa.

### Variáveis principais (`.env`)

| Variável | Descrição |
|----------|-----------|
| `NETQUASAR_DATABASE_URL` | DSN PostgreSQL |
| `NETQUASAR_SESSION_SECRET` | Segredo JWT (produção) |
| `NETQUASAR_API_KEYS` | Chaves API |
| `NETQUASAR_REDIS_URL` | Redis para WebSocket tempo real |
| `NETQUASAR_PUBLISH_PORT` | Porta HTTP no Compose |
| `NETQUASAR_LOG_LEVEL` | `debug`, `info`, `warn`, `error` |

---

## Mapa de rotas (SPA)

| Rota | Módulo |
|------|--------|
| `/dashboard` | Dashboard |
| `/monitoring` | Monitoramento |
| `/realtime` | Tempo real |
| `/integrations` | Integrações |
| `/pops` | POPs |
| `/devices` | Equipamentos |
| `/commercial` | Clientes |
| `/connections` | Conexões PPPoE/DHCP |
| `/alerts` | Alertas |
| `/map` | Mapa |
| `/tools` | Ferramentas |
| `/olt` | OLT |
| `/mikrotik` | MikroTik |
| `/events` | Eventos |
| `/settings` | Configurações |

Redireccionamentos legados: `/alertas` → `/alerts`, `/comercial` → `/commercial`, `/bng` → `/mikrotik`, etc. (ver `routes.ts`).

---

## Segurança

- Não commitar `.env`, certificados (`*.crt`) nem credenciais.
- Palavras-passe fortes em Postgres e `NETQUASAR_SESSION_SECRET`.
- Em produção: firewall + reverse proxy com TLS.
- Codificar caracteres especiais no DSN (`%2C`, `%24`, …).

---

## Resolução de problemas

| Sintoma | Verificar |
|---------|-----------|
| Frontend sem API | Proxy Vite / mesma origem em produção |
| Autenticação Postgres falha | DSN, password URL-encoded, credenciais Supabase |
| Docker + Supabase | Usar **Session pooler** (IPv4) — ver README-BACKEND |
| Interfaces MikroTik incompletas | Aumentar `interface_snapshot_timeout_ms` |
| OLT sem actualizar PONs | Monitoramento **Full** ligado; intervalo `olt_if_derived_pon_seconds`; equipamento online |
| Alertas não no Telegram | Configurações → Telegram monitoring; bot/chat correctos |
| Migrações em falta | `go run ./cmd/migrate/` |

---

## Licença

Consulte os ficheiros de licença no repositório. Evolução de longo prazo: [ROADMAP-ARQUITETURAL-DEPLOY.md](ROADMAP-ARQUITETURAL-DEPLOY.md).
