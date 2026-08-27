-- +goose Up
-- Ciclo dedicado de coleta periódica das sessões PPPoE detalhadas (login/IP/MAC/uptime por
-- assinante) — até aqui, a única forma de popular bng_session_snapshots era o botão manual
-- "Coletar sessões" na tela de BNG; nada automático corria (RunBngSweep/TryStartParallelBngCycle
-- só recolhe TOTAIS — pppoe_online, ipv4_online etc. — nunca a lista de logins). Mesmo padrão já
-- usado por ping/telemetria/BNG-totais/OLT: cadência própria, independente do pipeline
-- sequencial, com lock dedicado para não sobrepor ciclos.
-- Default alto (30 min) de propósito: um walk completo de sessões pode levar minutos num BNG
-- com milhares de assinantes — não deve competir com os ciclos leves de segundos.

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS bng_sessions_parallel_seconds INT NOT NULL DEFAULT 1800;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_bng_sessions_parallel_cycle_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_bng_sessions_parallel_cycle_at;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS bng_sessions_parallel_seconds;
