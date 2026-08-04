-- +goose Up
-- Provider de ligação (local|external) + backup periódico Backblaze B2.

ALTER TABLE settings_database_meta
  ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'local'
  CHECK (provider IN ('local', 'external'));

CREATE TABLE IF NOT EXISTS settings_b2_backup (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    key_id TEXT,
    application_key TEXT,
    bucket TEXT,
    bucket_id TEXT,
    endpoint TEXT,
    region TEXT NOT NULL DEFAULT 'us-east-005',
    prefix TEXT NOT NULL DEFAULT 'netquasar/postgres',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings_b2_backup (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS automation_database_backup (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT false,
    frequency TEXT NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly')),
    day_of_week INT CHECK (day_of_week IS NULL OR (day_of_week >= 0 AND day_of_week <= 6)),
    time_hhmm TEXT NOT NULL DEFAULT '03:00' CHECK (time_hhmm ~ '^\d{2}:\d{2}$'),
    timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    keep_last INT NOT NULL DEFAULT 14 CHECK (keep_last >= 1 AND keep_last <= 365),
    last_run_at TIMESTAMPTZ,
    last_run_key TEXT,
    last_status TEXT,
    last_error TEXT,
    last_object_key TEXT,
    last_size_bytes BIGINT,
    running BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO automation_database_backup (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS automation_database_backup;
DROP TABLE IF EXISTS settings_b2_backup;
ALTER TABLE settings_database_meta DROP COLUMN IF EXISTS provider;
