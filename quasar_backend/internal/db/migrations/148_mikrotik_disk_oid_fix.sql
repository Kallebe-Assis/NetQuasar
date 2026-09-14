-- +goose Up
-- O índice de linha .131072 chutado para hrStorage (disco) na primeira versão não bateu no
-- equipamento real do utilizador — snmpget confirmou que o agente responde directo na raiz da
-- coluna, sem índice (ver mikrotikcollect/metrics.go, disk_total/disk_used). Corrige só quem
-- ainda tem exactamente o OID antigo (nunca editado à mão desde então) — não mexe em quem já
-- personalizou o OID.
UPDATE settings_mikrotik_collection
SET metrics = jsonb_set(
    metrics,
    '{disk_total,oid}',
    '"1.3.6.1.2.1.25.2.3.1.5"'::jsonb
)
WHERE id = 1
  AND metrics -> 'disk_total' ->> 'oid' = '1.3.6.1.2.1.25.2.3.1.5.131072';

UPDATE settings_mikrotik_collection
SET metrics = jsonb_set(
    metrics,
    '{disk_used,oid}',
    '"1.3.6.1.2.1.25.2.3.1.6"'::jsonb
)
WHERE id = 1
  AND metrics -> 'disk_used' ->> 'oid' = '1.3.6.1.2.1.25.2.3.1.6.131072';

-- +goose Down
-- Não reversível de forma segura (não dá para distinguir "estava no antigo chute" de
-- "o utilizador voltou a pôr esse valor à mão" depois do Up) — down é no-op.
