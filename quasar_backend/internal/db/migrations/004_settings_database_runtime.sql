-- +goose Up
ALTER TABLE settings_database_meta ADD COLUMN IF NOT EXISTS ssl_mode TEXT NOT NULL DEFAULT 'disable';
ALTER TABLE settings_database_meta ADD COLUMN IF NOT EXISTS db_password TEXT;

CREATE TABLE IF NOT EXISTS settings_connection_audit (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ok BOOLEAN NOT NULL,
    phase TEXT NOT NULL,
    message TEXT NOT NULL,
    target_host TEXT,
    target_port INT,
    target_db TEXT
);

CREATE INDEX IF NOT EXISTS idx_settings_connection_audit_created ON settings_connection_audit (created_at DESC);

INSERT INTO settings_database_meta (id, ssl_mode) VALUES (1, 'disable')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS idx_settings_connection_audit_created;
DROP TABLE IF EXISTS settings_connection_audit;
ALTER TABLE settings_database_meta DROP COLUMN IF EXISTS db_password;
ALTER TABLE settings_database_meta DROP COLUMN IF EXISTS ssl_mode;
