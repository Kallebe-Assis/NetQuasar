-- +goose Up
-- VSOL: IF-MIB via snapshot (rápido) + walk gOnuAuthList — evita if_mib_refresh duplicado no refresh diário.
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"if_snap","method":"if_mib_snapshot","enabled":true},
  {"id":"vsol","method":"vsol_onu_collect","enabled":true,"params":{"include_if_mib":false}}
]'::jsonb
WHERE brand = 'VSOL';

-- +goose Down
UPDATE olt_vendor_models SET collection_steps = '[
  {"id":"if_refresh","method":"if_mib_refresh","enabled":true},
  {"id":"vsol","method":"vsol_onu_collect","enabled":true,"params":{"include_if_mib":false}}
]'::jsonb
WHERE brand = 'VSOL';
