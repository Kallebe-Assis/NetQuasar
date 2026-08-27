-- +goose Up
-- Capturas de tráfego guardadas pela aba Sniffer (Ferramentas). Os pacotes em si (com
-- payload completo) não ficam na base de dados — ficam num ficheiro .pcap real em disco
-- (NETQUASAR_DATA_DIR/captures/<id>.pcap, abrível directamente no Wireshark); esta tabela
-- só indexa os metadados para listar/pesquisar capturas e saber que ficheiro ler.
CREATE TABLE IF NOT EXISTS network_captures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    source TEXT NOT NULL CHECK (source IN ('local','device')),
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    interface TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    stopped_at TIMESTAMPTZ,
    packet_count INT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    file_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS network_captures_started_idx ON network_captures (started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS network_captures;
