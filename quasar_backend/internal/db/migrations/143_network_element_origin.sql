-- +goose Up
-- Origem do sinal de uma CTO ou foguete (caixa de emenda/distribuição): de onde vem a fibra
-- que alimenta o elemento. É polimórfico (pode apontar para um POP, outro foguete, outra CTO,
-- uma OLT/switch/mikrotik/rádio da tabela devices) por isso não há FK — guarda-se o tipo, o id
-- de referência e um rótulo legível (que sobrevive mesmo que o elemento de origem seja apagado).
ALTER TABLE network_ctos
    ADD COLUMN IF NOT EXISTS origin_kind   TEXT,
    ADD COLUMN IF NOT EXISTS origin_ref_id UUID,
    ADD COLUMN IF NOT EXISTS origin_label  TEXT;

ALTER TABLE network_splice_boxes
    ADD COLUMN IF NOT EXISTS origin_kind   TEXT,
    ADD COLUMN IF NOT EXISTS origin_ref_id UUID,
    ADD COLUMN IF NOT EXISTS origin_label  TEXT;

-- +goose Down
ALTER TABLE network_ctos
    DROP COLUMN IF EXISTS origin_kind,
    DROP COLUMN IF EXISTS origin_ref_id,
    DROP COLUMN IF EXISTS origin_label;

ALTER TABLE network_splice_boxes
    DROP COLUMN IF EXISTS origin_kind,
    DROP COLUMN IF EXISTS origin_ref_id,
    DROP COLUMN IF EXISTS origin_label;
