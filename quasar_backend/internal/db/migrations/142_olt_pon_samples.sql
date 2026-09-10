-- +goose Up
-- Histórico por PORTA PON de cada OLT — uma linha por PON a cada colecta (manual ou
-- periódica). Alimenta o gráfico "histórico por PON" na aba Relatório da OLT (só quando uma
-- OLT específica está seleccionada). Distinto de olt_onu_samples (que é por OLT inteira) e de
-- olt_onu_history (que é por ONU individual).
CREATE TABLE olt_pon_samples (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    pon INT NOT NULL,
    pon_name TEXT,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    onu_total INT NOT NULL DEFAULT 0,
    onu_online INT NOT NULL DEFAULT 0,
    onu_offline INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_olt_pon_samples_lookup ON olt_pon_samples (device_id, pon, recorded_at DESC);
CREATE INDEX idx_olt_pon_samples_time ON olt_pon_samples (recorded_at);

-- +goose Down
DROP TABLE IF EXISTS olt_pon_samples;
