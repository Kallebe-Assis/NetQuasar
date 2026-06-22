-- +goose Up
-- Timeout dedicado à coleta telnet de métricas ONU/PON nas OLT (sequencial por ONU/PON).

ALTER TABLE monitoring_intervals
  ADD COLUMN IF NOT EXISTS olt_onu_telnet_timeout_ms INT NOT NULL DEFAULT 600000;

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS olt_onu_telnet_timeout_ms;
