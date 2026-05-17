-- +goose Up
ALTER TABLE automation_onu_report ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo';
ALTER TABLE automation_onu_report ADD COLUMN IF NOT EXISTS last_run_period TEXT;
ALTER TABLE automation_onu_report ADD COLUMN IF NOT EXISTS running BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE automation_onu_report DROP COLUMN IF EXISTS running;
ALTER TABLE automation_onu_report DROP COLUMN IF EXISTS last_run_period;
ALTER TABLE automation_onu_report DROP COLUMN IF EXISTS timezone;
