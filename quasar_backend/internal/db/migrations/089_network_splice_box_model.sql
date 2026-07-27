-- +goose Up
ALTER TABLE network_splice_boxes
    ADD COLUMN IF NOT EXISTS box_model TEXT NOT NULL DEFAULT 'emenda',
    ADD COLUMN IF NOT EXISTS splitter TEXT,
    ADD COLUMN IF NOT EXISTS fiber_color TEXT,
    ADD COLUMN IF NOT EXISTS splitter_ports JSONB,
    ADD COLUMN IF NOT EXISTS splice_pairs JSONB;

ALTER TABLE network_splice_boxes DROP CONSTRAINT IF EXISTS network_splice_boxes_box_model_chk;
ALTER TABLE network_splice_boxes
    ADD CONSTRAINT network_splice_boxes_box_model_chk CHECK (box_model IN ('emenda', 'distribuicao'));

COMMENT ON COLUMN network_splice_boxes.box_model IS 'emenda = fusões lado a lado; distribuicao = splitter interno';
COMMENT ON COLUMN network_splice_boxes.splice_pairs IS 'Pares de emenda: [{port, left_color, right_color, status, note, destination}]';
COMMENT ON COLUMN network_splice_boxes.splitter_ports IS 'Portas do splitter (modelo distribuicao)';

-- +goose Down
ALTER TABLE network_splice_boxes DROP CONSTRAINT IF EXISTS network_splice_boxes_box_model_chk;
ALTER TABLE network_splice_boxes DROP COLUMN IF EXISTS splice_pairs;
ALTER TABLE network_splice_boxes DROP COLUMN IF EXISTS splitter_ports;
ALTER TABLE network_splice_boxes DROP COLUMN IF EXISTS fiber_color;
ALTER TABLE network_splice_boxes DROP COLUMN IF EXISTS splitter;
ALTER TABLE network_splice_boxes DROP COLUMN IF EXISTS box_model;
