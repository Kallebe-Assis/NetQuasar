-- +goose Up
ALTER TABLE network_cables
    ADD COLUMN IF NOT EXISTS funcao TEXT NOT NULL DEFAULT 'outro';

ALTER TABLE network_cables DROP CONSTRAINT IF EXISTS network_cables_funcao_chk;
ALTER TABLE network_cables
    ADD CONSTRAINT network_cables_funcao_chk CHECK (
        funcao IN ('backbone_link', 'transporte', 'backbone_ftth', 'cto', 'multipla', 'outro')
    );

COMMENT ON COLUMN network_cables.funcao IS 'Função do cabo no mapa: backbone_link, transporte, backbone_ftth, cto, multipla (Função Múltipla) ou outro';

-- +goose Down
ALTER TABLE network_cables DROP CONSTRAINT IF EXISTS network_cables_funcao_chk;
ALTER TABLE network_cables DROP COLUMN IF EXISTS funcao;
