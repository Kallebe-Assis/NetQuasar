# NetQuasar

Plataforma de monitoramento e operação de rede para provedores ISP, composta por:

- `quasar_backend` — API, coleta, alertas, integração com OLT, SNMP, Telnet e rotinas de monitoramento
- `quasar_frontend` — interface web operacional em React

## Estrutura do projeto

- `quasar_backend/` — backend em Go
- `quasar_frontend/` — frontend em React + Vite
- `docker-compose.yml` — stack principal para deploy com Docker
- `.env.example` — modelo de variáveis para a raiz do projeto
- `scripts/` — utilitários auxiliares
- `deploy/linux-debian/README.md` — guia de deploy em Debian/Linux
- `ROADMAP-ARQUITETURAL-DEPLOY.md` — visão arquitetural e roadmap técnico

## Principais recursos

- monitoramento contínuo de equipamentos
- ping, SNMP, telemetria e interfaces
- coleta e visualização de OLT/PON/ONU
- alertas operacionais com severidade
- notificações Telegram
- ferramentas de diagnóstico
- mapa operacional e base comercial

## Desenvolvimento local

### Início rápido

Backend:

```powershell
cd quasar_backend
go run .\cmd\netquasar\
```

Frontend:

```powershell
cd quasar_frontend
npm install
npm run dev
```

Também existe o iniciador:

- `iniciar-netquasar-dev.bat` — abre backend e frontend em janelas separadas no Windows

### Endereços locais

- frontend: `http://localhost:5173`
- backend/UI embutida em produção: `http://localhost:8080`

## Deploy com Docker

1. Copie `.env.example` para `.env`
2. Ajuste as variáveis obrigatórias, principalmente `POSTGRES_PASSWORD`
3. Suba o stack:

```bash
docker compose up -d --build
```

Guia detalhado:

- `deploy/linux-debian/README.md`

## Documentação

- Backend: `quasar_backend/README-BACKEND.md`
- Frontend: `quasar_frontend/README-FRONTEND.md`
- Deploy Linux/Debian: `deploy/linux-debian/README.md`
- Roadmap arquitetural: `ROADMAP-ARQUITETURAL-DEPLOY.md`

## Observação sobre a documentação da raiz

O `README.md` passa a ser a entrada principal do projeto.

Foi mantido na raiz apenas o que faz sentido como documentação de topo:

- `README.md` — visão geral e entrada do projeto
- `ROADMAP-ARQUITETURAL-DEPLOY.md` — documento estratégico/técnico, não redundante
