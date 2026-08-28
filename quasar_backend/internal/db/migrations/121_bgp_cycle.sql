-- +goose Up
-- Ciclo periódico de coleta BGP (peers/interfaces/tráfego) — StepKindBgp/RunBgpSweep, mesmo
-- padrão do BNG. Adiciona só o timestamp de último ciclo; a coleta em si é configurada via
-- pipeline de monitorização (Configurações → Monitorização) e o perfil SNMP padrão de
-- Configurações → BGP (bgp_snmp_profiles).
ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_bgp_cycle_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_bgp_cycle_at;
