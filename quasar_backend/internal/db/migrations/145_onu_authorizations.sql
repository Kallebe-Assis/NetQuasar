-- +goose Up
-- Histórico de autorizações de ONU (tela OLT → ONUs não autorizadas). Guarda o essencial de
-- cada autorização bem-sucedida — serial, modelo, PON, ONU, VLAN, OLT, quem autorizou e quando,
-- e o cliente vinculado no momento — para o painel "últimas autorizações".
CREATE TABLE IF NOT EXISTS onu_authorizations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    olt_device_id  UUID REFERENCES devices(id) ON DELETE SET NULL,
    olt_description TEXT NOT NULL DEFAULT '',
    serial         TEXT NOT NULL DEFAULT '',
    model          TEXT NOT NULL DEFAULT '',
    pon            INT,
    onu            INT,
    vlan           TEXT NOT NULL DEFAULT '',
    client_name    TEXT NOT NULL DEFAULT '',
    authorized_by  TEXT NOT NULL DEFAULT '',
    authorized_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_onu_authorizations_at ON onu_authorizations (authorized_at DESC);
CREATE INDEX IF NOT EXISTS idx_onu_authorizations_serial ON onu_authorizations (serial);

-- +goose Down
DROP TABLE IF EXISTS onu_authorizations;
