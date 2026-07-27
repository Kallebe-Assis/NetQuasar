-- +goose Up
ALTER TABLE network_ctos DROP CONSTRAINT IF EXISTS network_ctos_fiber_color_chk;
ALTER TABLE network_ctos
    ADD CONSTRAINT network_ctos_fiber_color_chk CHECK (
        fiber_color IS NULL OR fiber_color IN (
            'Desconhecido',
            'Verde', 'Amarelo', 'Branco', 'Azul', 'Vermelho', 'Violeta',
            'Marrom', 'Rosa', 'Preto', 'Cinza', 'Laranja', 'Aqua (Turquesa)'
        )
    );

UPDATE network_ctos
SET fiber_color = 'Desconhecido'
WHERE fiber_color IS NULL OR TRIM(fiber_color) = '';

ALTER TABLE network_ctos
    ALTER COLUMN fiber_color SET DEFAULT 'Desconhecido';

-- +goose Down
ALTER TABLE network_ctos ALTER COLUMN fiber_color DROP DEFAULT;
UPDATE network_ctos SET fiber_color = NULL WHERE fiber_color = 'Desconhecido';
ALTER TABLE network_ctos DROP CONSTRAINT IF EXISTS network_ctos_fiber_color_chk;
ALTER TABLE network_ctos
    ADD CONSTRAINT network_ctos_fiber_color_chk CHECK (
        fiber_color IS NULL OR fiber_color IN (
            'Verde', 'Amarelo', 'Branco', 'Azul', 'Vermelho', 'Violeta',
            'Marrom', 'Rosa', 'Preto', 'Cinza', 'Laranja', 'Aqua (Turquesa)'
        )
    );
