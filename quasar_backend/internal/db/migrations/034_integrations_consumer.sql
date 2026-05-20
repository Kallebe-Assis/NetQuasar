-- +goose Up
ALTER TABLE integrations
    ADD COLUMN consumer_config JSONB NOT NULL DEFAULT '{"client_search":{"enabled":false}}'::jsonb;

-- +goose Down
ALTER TABLE integrations DROP COLUMN IF EXISTS consumer_config;
