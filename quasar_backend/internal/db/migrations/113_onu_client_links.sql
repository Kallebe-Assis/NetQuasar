-- +goose Up
-- Vincula um cliente (nome) a uma ONU pelo número de série. As ONUs em si não são
-- persistidas em tabela própria — vêm ao vivo do snapshot SNMP/telnet de cada OLT
-- (olt_snapshots.summary) — por isso este vínculo mora numa tabela separada, indexada
-- pelo serial, e é juntado por cima dos resultados de pesquisa de ONUs (ver
-- handlers_olt_onu_search.go / handlers_olt_onu_client_links.go).
CREATE TABLE IF NOT EXISTS onu_client_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    serial TEXT NOT NULL UNIQUE,
    client_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS onu_client_links;
