# NetQuasar — README Backend

Backend principal do NetQuasar, responsável por:

- API REST
- coleta de monitoramento (ping/SNMP/OLT/interfaces)
- persistência e histórico
- avaliação de alertas
- notificações Telegram
- integrações e ferramentas operacionais

## Módulos centrais

- `internal/api` — handlers HTTP por domínio
- `internal/monitorworker` — workers de monitoramento e alertas
- `internal/alertthresholds` — avaliação de limiares e severidade
- `internal/probing` — sondas de rede (SNMP, Telnet, etc.)
- `internal/snmp*` — parsing e mapeamentos SNMP
- `internal/db` — migrações e acesso a banco

## Domínios de API

- Monitoramento (estado/start/stop/intervalos)
- Ping
- Telemetria
- Interfaces
- OLT
- Alertas e regras
- Configurações
- Ferramentas (DNS/SNMP/Telnet/MikroTik)

## Responsabilidades de negócio

- manter o estado operacional de equipamentos/OLTs
- gerar e fechar alertas automaticamente
- emitir mensagens para Telegram quando necessário
- entregar dados estruturados para o frontend

## Execução local (resumo)

1. Configurar variáveis de ambiente e banco
2. Executar migrações (se aplicável)
3. Iniciar servidor Go

### Variável `NETQUASAR_EMBEDDED_UI`

- `false` (omissão recomendada para dev com Vite): só API (e ficheiros não servidos pelo Go).
- `true`: serve o build estático incluído em `internal/embedui/dist` (placeholder no repositório; a imagem Docker gera o build Vite real no build multi-stage).

### Supabase / PostgreSQL na nuvem (SSL)

- Host: `db.<ref>.supabase.co` (não use o URL `https://` do painel — isso é API HTTP, não Postgres).
- Em `NETQUASAR_DATABASE_URL` use `sslmode=require` (ou `verify-full` se souber o que implica).
- **CA raiz Supabase:** o repositório inclui `data/certs/supabase-root-ca-2021.pem`. Para o host **direto** `db.<ref>.supabase.co`, o backend **acrescenta automaticamente** `sslrootcert` (ou `NETQUASAR_DB_SSLROOTCERT` se definir). No **pooler** `*.pooler.supabase.com` o TLS usa o certificado do endpoint AWS (não se força o PEM da instância Postgres da Supabase).
- Caracteres especiais na palavra-passe devem ir **codificados na URL** (ex. `,` → `%2C`, `$` → `%24`). Se `go run` falhar com `password authentication failed`, atualize a palavra-passe em **Project Settings → Database** no Supabase e alinhe a URL no `.env`.

### Supabase + Docker (IPv6)

O hostname **direto** `db.<ref>.supabase.co` muitas vezes **só tem registo DNS AAAA (IPv6)**. No Windows o IPv6 costuma funcionar; no **Docker** (p.ex. Docker Desktop) a VM Linux por vezes **não tem rota IPv6** até à Internet, o que produz erros como `dial tcp [2600:…]:5432: connect: network is unreachable`. Isto **não** é falha de SSL nem de palavra-passe.

- **Recomendado em contentores:** usar a cadeia **Session pooler** do painel (Connect → Session): host `aws-0-<região>.pooler.supabase.com` ou `aws-1-<região>.pooler.supabase.com` (a Supabase indica qual no teu projeto), porta **5432**, utilizador `postgres.<project_ref>`, base `postgres`. Modo transação no host `db.*` usa porta **6543** (só IPv6 no DNS do host direto).
- **Alternativa:** ativar IPv6 no Docker / rede do servidor, ou a opção **IPv4** para Postgres na Supabase quando existir no plano.
- O utilitário `go run ./cmd/dbping` na pasta `quasar_backend` replica a mesma DSN; em falha, pode imprimir uma linha extra com esta explicação.

## Documentação relacionada

- Geral: `../README.md`
- Deploy Docker (Linux/Debian/VM): `../deploy/linux-debian/README.md`, `../Dockerfile`, `../docker-compose.yml` na raiz do projeto
- Frontend: `../quasar_frontend/README-FRONTEND.md`

