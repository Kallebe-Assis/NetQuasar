-- +goose Up
ALTER TABLE settings_ui
    ADD COLUMN IF NOT EXISTS map_splice_emenda_color TEXT NOT NULL DEFAULT '#d97706',
    ADD COLUMN IF NOT EXISTS map_splice_distribuicao_color TEXT NOT NULL DEFAULT '#7c3aed',
    ADD COLUMN IF NOT EXISTS map_splice_emenda_icon TEXT NOT NULL DEFAULT 'joint',
    ADD COLUMN IF NOT EXISTS map_splice_distribuicao_icon TEXT NOT NULL DEFAULT 'rocket',
    ADD COLUMN IF NOT EXISTS map_cable_funcao_colors JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Preserva a personalização já feita (cor/ícone únicos do foguete) nas 2 variantes novas, em vez
-- de perder a escolha do utilizador quando emenda/distribuição passam a ter cor/ícone próprios.
UPDATE settings_ui SET
    map_splice_emenda_color = map_splice_color,
    map_splice_distribuicao_color = map_splice_color,
    map_splice_emenda_icon = CASE WHEN map_splice_icon = 'rocket' THEN 'joint' ELSE map_splice_icon END,
    map_splice_distribuicao_icon = map_splice_icon
WHERE id = 1;

COMMENT ON COLUMN settings_ui.map_cable_funcao_colors IS 'Cor (hex) por função de cabo — chaves: backbone_link, transporte, backbone_ftth, cto, multipla, outro. Chave ausente usa a cor padrão dessa função.';

-- +goose Down
ALTER TABLE settings_ui
    DROP COLUMN IF EXISTS map_splice_emenda_color,
    DROP COLUMN IF EXISTS map_splice_distribuicao_color,
    DROP COLUMN IF EXISTS map_splice_emenda_icon,
    DROP COLUMN IF EXISTS map_splice_distribuicao_icon,
    DROP COLUMN IF EXISTS map_cable_funcao_colors;
