-- +goose Up
-- Retenção automática de histórico. Antes disto, ping_history / telemetry_samples /
-- interface_snapshots só eram limpos manualmente (Configurações -> Base de dados),
-- crescendo indefinidamente. history_retention_days = 0 desliga a purga automática
-- (mantém o comportamento anterior). Ver DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md.

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS history_retention_days INT NOT NULL DEFAULT 90;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_retention_run_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_retention_run_at;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS history_retention_days;
