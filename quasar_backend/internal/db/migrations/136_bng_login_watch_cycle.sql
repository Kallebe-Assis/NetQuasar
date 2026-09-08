-- +goose Up
-- Ciclo "rápido" de presença online/offline dos logins PPPoE — só o walk de access_login (sem
-- GET de IPv4/IPv6/MAC/VLAN/etc. por índice, ver bngcollect.CollectAndSyncOnlineLoginsFast),
-- muito mais leve que TryStartParallelBngSessionsCycle (walk completo com detalhe, default 30
-- min) e por isso pode correr com cadência bem menor sem sobrecarregar o BNG. Alimenta só
-- bng_known_logins/bng_login_events (online/offline) — nunca bng_session_snapshots, que continua
-- a depender do ciclo completo/botão manual para IP/MAC/VLAN/etc. na tela "Sessões PPPoE".
-- Default 90s: rápido o suficiente para o mapa/tela de Logins reflectirem quedas/conexões quase
-- em tempo real, sem martelar o BNG a cada poucos segundos.

ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS bng_login_watch_seconds INT NOT NULL DEFAULT 90;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_bng_login_watch_cycle_at TIMESTAMPTZ;

-- Cruzamento client_connections.login (tela Elementos -> Logins / mapa) x bng_known_logins.login
-- (handlers_client_connections.go: loadOnlineLoginSet) — um SELECT DISTINCT lower(trim(login))
-- WHERE is_online por todos os BNGs de cada vez; este índice parcial evita varrer a tabela toda.
CREATE INDEX IF NOT EXISTS idx_bng_known_logins_login_online
    ON bng_known_logins (lower(trim(login)))
    WHERE is_online = true;

-- +goose Down
DROP INDEX IF EXISTS idx_bng_known_logins_login_online;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_bng_login_watch_cycle_at;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS bng_login_watch_seconds;
