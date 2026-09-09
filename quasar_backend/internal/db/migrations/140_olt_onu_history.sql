-- +goose Up
-- Histórico por ONU (pon+onu) — últimas colectas para o botão "Histórico" na aba de ONUs (OLT).
-- Uma linha por ONU a cada colecta (ver internal/oltsamples/oltsamples.go, RecordOnuHistory,
-- chamado nos mesmos pontos que RecordSample/olt_onu_samples) — "row" guarda o mesmo objecto
-- já usado pela tabela ao vivo (status, rx/tx, temperatura, voltagem, modelo, serial…), assim o
-- histórico mostra exactamente os mesmos campos, sem duplicar o parsing. Podado para as últimas
-- 10 linhas por (device_id, pon, onu) a cada gravação — não é uma série temporal sem fim.
CREATE TABLE olt_onu_history (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    pon INT NOT NULL,
    onu INT NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    row JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_olt_onu_history_lookup ON olt_onu_history (device_id, pon, onu, collected_at DESC);

-- +goose Down
DROP TABLE IF EXISTS olt_onu_history;
