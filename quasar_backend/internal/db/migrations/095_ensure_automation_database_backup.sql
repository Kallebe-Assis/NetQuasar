-- +goose Up
-- Recuperação: garante tabelas do backup automático mesmo se a 092
-- tiver ficado marcada no goose sem criar as relações (ou restore incompleto).

ALTER TABLE settings_database_meta
  ADD COLUMN IF NOT EXISTS provider TEXT;

UPDATE settings_database_meta
SET provider = 'local'
WHERE provider IS NULL OR trim(provider) = '';

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'settings_database_meta_provider_check'
      AND conrelid = 'settings_database_meta'::regclass
  ) THEN
    ALTER TABLE settings_database_meta
      ADD CONSTRAINT settings_database_meta_provider_check
      CHECK (provider IN ('local', 'external'));
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE settings_database_meta
  ALTER COLUMN provider SET DEFAULT 'local',
  ALTER COLUMN provider SET NOT NULL;

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
SELECT 1;
