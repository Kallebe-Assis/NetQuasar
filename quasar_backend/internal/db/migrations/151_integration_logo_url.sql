-- +goose Up
-- Logo por integração (link http(s) de imagem) — antes só existia no localStorage do navegador
-- (lib/hubsoftLogo.ts / lib/integrationLogo.ts), por isso "sumia" ao trocar de navegador/limpar
-- dados e nunca era realmente "salvo" no sentido de persistente/partilhado entre a equipa.
ALTER TABLE integrations
    ADD COLUMN IF NOT EXISTS logo_url TEXT;

-- +goose Down
ALTER TABLE integrations DROP COLUMN IF EXISTS logo_url;
