-- +goose Up
-- Backup de configuração (script/texto) por equipamento.

CREATE TABLE device_config_backups (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    content TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS device_config_backups CASCADE;
