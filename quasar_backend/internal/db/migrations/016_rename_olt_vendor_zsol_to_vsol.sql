-- +goose Up
UPDATE olt_vendor_profiles SET brand = 'VSOL' WHERE brand = 'ZSOL';
UPDATE devices SET brand = 'VSOL' WHERE lower(trim(brand)) = 'zsol';

-- +goose Down
UPDATE olt_vendor_profiles SET brand = 'ZSOL' WHERE brand = 'VSOL';
UPDATE devices SET brand = 'ZSOL' WHERE lower(trim(brand)) = 'vsol';
