-- +goose Up
-- Cadências de coleta OLT em 3 níveis: status PON, contagens ONU, coleta completa.

ALTER TABLE monitoring_intervals
  ADD COLUMN IF NOT EXISTS olt_pon_status_seconds INT NOT NULL DEFAULT 60,
  ADD COLUMN IF NOT EXISTS olt_onu_counts_seconds INT NOT NULL DEFAULT 180,
  ADD COLUMN IF NOT EXISTS olt_full_collect_seconds INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS olt_full_collect_schedule TEXT NOT NULL DEFAULT '';

ALTER TABLE monitoring_runtime
  ADD COLUMN IF NOT EXISTS last_olt_pon_status_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_olt_onu_counts_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_olt_full_collect_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime
  DROP COLUMN IF EXISTS last_olt_full_collect_at,
  DROP COLUMN IF EXISTS last_olt_onu_counts_at,
  DROP COLUMN IF EXISTS last_olt_pon_status_at;

ALTER TABLE monitoring_intervals
  DROP COLUMN IF EXISTS olt_full_collect_schedule,
  DROP COLUMN IF EXISTS olt_full_collect_seconds,
  DROP COLUMN IF EXISTS olt_onu_counts_seconds,
  DROP COLUMN IF EXISTS olt_pon_status_seconds;
