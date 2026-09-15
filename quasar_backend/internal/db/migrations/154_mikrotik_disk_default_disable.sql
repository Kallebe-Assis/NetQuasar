-- +goose Up
-- disk_total/disk_used estavam activos por omissão (ver migração 148) assumindo que o OID sem
-- índice devolvia uma linha de disco/flash real — testado ao vivo (RB3011): devolve exactamente
-- o mesmo valor de memory_total/memory_used (a linha .65536 da RAM), não disco. Desactiva quem
-- ainda tem exactamente o OID por omissão sem índice (nunca personalizado desde então) — não mexe
-- em quem já apontou para um índice de linha próprio.
UPDATE settings_mikrotik_collection
SET metrics = jsonb_set(metrics, '{disk_total,enabled}', 'false'::jsonb)
WHERE id = 1
  AND metrics -> 'disk_total' ->> 'oid' = '1.3.6.1.2.1.25.2.3.1.5'
  AND (metrics -> 'disk_total' ->> 'enabled')::boolean IS TRUE;

UPDATE settings_mikrotik_collection
SET metrics = jsonb_set(metrics, '{disk_used,enabled}', 'false'::jsonb)
WHERE id = 1
  AND metrics -> 'disk_used' ->> 'oid' = '1.3.6.1.2.1.25.2.3.1.6'
  AND (metrics -> 'disk_used' ->> 'enabled')::boolean IS TRUE;

-- +goose Down
-- Não reversível de forma segura (não dá para distinguir "estava activo por omissão" de "o
-- utilizador reactivou de propósito" depois do Up) — down é no-op.
