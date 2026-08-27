-- +goose Up
-- Concorrência do worker (equipamentos sondados em paralelo por ciclo: ping/telemetria/BNG/OLT).
-- Antes fixa em 6 no código, só ajustável via variável de ambiente NETQUASAR_SWEEP_CONCURRENCY.
-- 0 = usar o default do código (12, ver internal/monitorworker/sweep_concurrency.go).
-- Ver DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md (achado "concorrência do worker").

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS sweep_concurrency INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS sweep_concurrency;
