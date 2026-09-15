-- +goose Up
-- Ciclo dedicado de coleta MikroTik COMPLETA (disco/óptica/telnet/PPPoE), separado do ciclo
-- rápido de saúde (telemetry_seconds, CollectHealthAndStore — só CPU/memória/temperatura/
-- uptime). Até aqui, a coleta completa (mikrotikcollect.CollectAndStore) só corria pelo botão
-- manual "Coletar agora" ou, para equipamentos com bng_enabled, dentro do ciclo BNG — nada
-- automático a refrescava para um MikroTik comum. Resultado: disco/óptica/PPPoE apareciam uma
-- vez após uma coleta manual e "somiam" da leitura de telemetria mais recente assim que o
-- próximo tick de saúde gravava uma amostra mais magra (telemetry_samples é sempre INSERT, a
-- leitura de "atual" lia só a última linha — ver telemetryengine.LoadLatestMergedTelemetry para
-- o outro lado desta correção). Default espaçado (15 min) de propósito: walks ópticos/telnet são
-- pesados, não devem competir com o ciclo leve de segundos.

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS mikrotik_full_parallel_seconds INT NOT NULL DEFAULT 900;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_mikrotik_full_parallel_cycle_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_mikrotik_full_parallel_cycle_at;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS mikrotik_full_parallel_seconds;
