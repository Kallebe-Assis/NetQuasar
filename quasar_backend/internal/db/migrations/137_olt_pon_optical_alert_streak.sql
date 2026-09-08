-- +goose Up
-- Mesma proteção contra falso positivo que olt_onu_optical_streak (migração 047) já dá aos
-- alertas de ONU — só falta no nível de PON (olt_pon_tx/olt_pon_rx/olt_pon_temp,
-- internal/alertthresholds/olt_pon_optical.go), que hoje abre alerta numa ÚNICA leitura ruim.
-- Isso causou TX=0 falso-positivo em várias PONs de várias OLTs ao mesmo tempo quando o
-- servidor teve latência alta até elas (um timeout SNMP pontual, não uma falha real de óptica).
CREATE TABLE IF NOT EXISTS olt_pon_optical_streak (
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    pon_key TEXT NOT NULL,
    metric_id TEXT NOT NULL,
    streak INT NOT NULL DEFAULT 0 CHECK (streak >= 0),
    last_value DOUBLE PRECISION,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, pon_key, metric_id)
);

-- +goose Down
DROP TABLE IF EXISTS olt_pon_optical_streak;
