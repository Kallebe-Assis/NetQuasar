-- +goose Up
-- Ciclo dedicado de snapshot de interfaces (IF-MIB) para equipamentos BNG — até aqui, o
-- pipeline periódico só varria interfaces de Mikrotik/switch/OLT (ver StepKind* em
-- pipeline_runner.go); BNG nunca entrava, então interface_snapshots só tinha dados de BNG
-- quando alguém clicava "Atualizar" manualmente na tela de equipamento. Necessário para o
-- monitoramento de tráfego dos uplinks de operadora (K2/FORTE) na aba Relatório do BNG.
-- Reaproveita a cadência já usada por Mikrotik/switch/OLT (interface_snapshot_seconds).

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_bng_interfaces_parallel_cycle_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_bng_interfaces_parallel_cycle_at;
