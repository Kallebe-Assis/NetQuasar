-- +goose Up
ALTER TABLE automation_database_backup
    ADD COLUMN IF NOT EXISTS days_of_week INT[] NOT NULL DEFAULT '{}';
ALTER TABLE automation_alerts_digest
    ADD COLUMN IF NOT EXISTS days_of_week INT[] NOT NULL DEFAULT '{}';
ALTER TABLE automation_bng_stats_report
    ADD COLUMN IF NOT EXISTS days_of_week INT[] NOT NULL DEFAULT '{}';

ALTER TABLE automation_database_backup DROP CONSTRAINT IF EXISTS automation_database_backup_frequency_check;
ALTER TABLE automation_database_backup
    ADD CONSTRAINT automation_database_backup_frequency_check
    CHECK (frequency IN ('daily', 'weekly', 'custom'));

ALTER TABLE automation_alerts_digest DROP CONSTRAINT IF EXISTS automation_alerts_digest_frequency_check;
ALTER TABLE automation_alerts_digest
    ADD CONSTRAINT automation_alerts_digest_frequency_check
    CHECK (frequency IN ('daily', 'weekly', 'custom'));

ALTER TABLE automation_bng_stats_report DROP CONSTRAINT IF EXISTS automation_bng_stats_report_frequency_check;
ALTER TABLE automation_bng_stats_report
    ADD CONSTRAINT automation_bng_stats_report_frequency_check
    CHECK (frequency IN ('daily', 'weekly', 'custom'));

UPDATE automation_database_backup
SET days_of_week = ARRAY[day_of_week]
WHERE frequency = 'weekly' AND day_of_week IS NOT NULL AND COALESCE(cardinality(days_of_week), 0) = 0;

UPDATE automation_alerts_digest
SET days_of_week = ARRAY[day_of_week]
WHERE frequency = 'weekly' AND day_of_week IS NOT NULL AND COALESCE(cardinality(days_of_week), 0) = 0;

UPDATE automation_bng_stats_report
SET days_of_week = ARRAY[day_of_week]
WHERE frequency = 'weekly' AND day_of_week IS NOT NULL AND COALESCE(cardinality(days_of_week), 0) = 0;

-- +goose Down
ALTER TABLE automation_database_backup DROP COLUMN IF EXISTS days_of_week;
ALTER TABLE automation_alerts_digest DROP COLUMN IF EXISTS days_of_week;
ALTER TABLE automation_bng_stats_report DROP COLUMN IF EXISTS days_of_week;
