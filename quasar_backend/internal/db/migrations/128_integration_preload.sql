-- +goose Up
-- Se marcado, o backend pré-aquece o cache dos dados desta integração (hoje só usado pela
-- HubSoft: atendimentos/O.S./financeiro recentes) assim que o servidor arranca, em vez de
-- esperar o primeiro usuário abrir a tela de Integrações — ver ensureIntegrationPreload em
-- internal/api/server.go. Falso por omissão: continua exactamente como era (carga só sob
-- demanda, na primeira visita à tela).
ALTER TABLE integrations ADD COLUMN preload_on_startup BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE integrations DROP COLUMN IF EXISTS preload_on_startup;
