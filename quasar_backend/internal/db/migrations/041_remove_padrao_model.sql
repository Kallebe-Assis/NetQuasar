-- +goose Up
DELETE FROM olt_vendor_models WHERE model = 'Padrão';

-- +goose Down
-- modelo «Padrão» removido de propósito; não recriar automaticamente
