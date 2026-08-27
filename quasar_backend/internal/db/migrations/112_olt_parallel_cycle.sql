-- +goose Up
-- Ciclo OLT paralelo dedicado (mesmo padrão já usado por ping/telemetria/BNG): a coleta
-- "leve" de ONUs/PON (baseline/pon_status/onu_counts) passa a correr numa goroutine própria,
-- numa cadência curta e independente do pipeline sequencial de interfaces/OLT completo
-- (que antes só corria a cada pipeline_cycle_seconds, tipicamente 120s, e em sequência
-- depois de outros passos). Isto é o que torna a detecção de queda de ONUs mais ágil.
-- Ver DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md (secção "detecção ágil de queda de ONUs").

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS olt_baseline_parallel_seconds INT NOT NULL DEFAULT 30;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_olt_baseline_parallel_cycle_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_olt_baseline_parallel_cycle_at;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS olt_baseline_parallel_seconds;
