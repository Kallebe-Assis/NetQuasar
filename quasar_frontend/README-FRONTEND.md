# NetQuasar — README Frontend

Frontend web do NetQuasar (React), focado em operação NOC com telas de monitoramento, alertas e configuração.

## Stack

- React + TypeScript
- Vite
- TanStack Query
- React Router

## Áreas principais da interface

- Monitoramento (Overview, Equipamentos, OLT, MikroTik/Interfaces, tempo real)
- Alertas
- Configurações
- Mapa
- Relatórios/Base comercial
- Ferramentas operacionais

## Responsabilidades do frontend

- consumir APIs modulares do backend
- apresentar estado operacional com feedback claro (loading/erro/sucesso)
- permitir ações rápidas de operação (refresh, tempo real, filtros, diagnóstico)
- manter UX consistente para fluxo diário de NOC

## Execução local

1. Entrar em `quasar_frontend`
2. Instalar dependências: `npm install`
3. Desenvolvimento: `npm run dev`
4. Build produção: `npm run build`

## Documentação relacionada

- Geral: `../README.md`
- Backend: `../quasar_backend/README-BACKEND.md`

