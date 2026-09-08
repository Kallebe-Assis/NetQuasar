-- +goose Up
ALTER TABLE settings_ui
    ADD COLUMN IF NOT EXISTS map_equipment_categories JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS map_icon_image_urls JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN settings_ui.map_equipment_categories IS
    'Ícone/visibilidade por categoria de equipamento no mapa — chave = categoria (Concentrador, Energia, Mikrotik, Switch, OLT, Rádio, Servidor, Máquina Virtual, Outros), valor = {icon?, image_url?, hidden?}. Chave ausente usa o ícone padrão (map_equipment_icon) e fica visível.';
COMMENT ON COLUMN settings_ui.map_icon_image_urls IS
    'URL de imagem personalizada por tipo de elemento do mapa (importado em vez de escolher um ícone do catálogo) — chaves: equipment, connection, cto, splice_emenda, splice_distribuicao.';

-- +goose Down
ALTER TABLE settings_ui
    DROP COLUMN IF EXISTS map_equipment_categories,
    DROP COLUMN IF EXISTS map_icon_image_urls;
