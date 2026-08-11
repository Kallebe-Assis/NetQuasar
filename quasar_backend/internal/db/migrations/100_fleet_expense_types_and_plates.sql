-- +goose Up
CREATE TABLE fleet_expense_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description TEXT NOT NULL,
    code TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX fleet_expense_types_description_uq ON fleet_expense_types (lower(trim(description)));
CREATE UNIQUE INDEX fleet_expense_types_code_uq ON fleet_expense_types (lower(trim(code)))
    WHERE code IS NOT NULL AND trim(code) <> '';

INSERT INTO fleet_expense_types (description, code) VALUES
    ('Manutenção preventiva', 'preventive_maintenance'),
    ('Manutenção corretiva', 'corrective_maintenance'),
    ('Lavagem', 'wash'),
    ('Pneu', 'tire'),
    ('Multa', 'fine'),
    ('Documentação', 'documentation');

ALTER TABLE fleet_expenses ADD COLUMN expense_type_id UUID REFERENCES fleet_expense_types(id) ON DELETE RESTRICT;

UPDATE fleet_expenses e
SET expense_type_id = t.id
FROM fleet_expense_types t
WHERE e.expense_type_id IS NULL AND t.code = e.expense_type;

INSERT INTO fleet_expense_types (description, code)
SELECT DISTINCT initcap(replace(e.expense_type, '_', ' ')), e.expense_type
FROM fleet_expenses e
WHERE e.expense_type_id IS NULL
  AND e.expense_type IS NOT NULL
  AND trim(e.expense_type) <> ''
  AND NOT EXISTS (
      SELECT 1 FROM fleet_expense_types t WHERE lower(trim(t.code)) = lower(trim(e.expense_type))
  );

UPDATE fleet_expenses e
SET expense_type_id = t.id
FROM fleet_expense_types t
WHERE e.expense_type_id IS NULL AND t.code = e.expense_type;

ALTER TABLE fleet_expenses ALTER COLUMN expense_type_id SET NOT NULL;
ALTER TABLE fleet_expenses DROP COLUMN expense_type;

UPDATE fleet_vehicles
SET plate = regexp_replace(upper(regexp_replace(plate, '[^A-Za-z0-9]', '', 'g')), '^([A-Z]{3})(.{4})$', '\1-\2')
WHERE length(regexp_replace(plate, '[^A-Za-z0-9]', '', 'g')) = 7;

-- +goose Down
ALTER TABLE fleet_expenses ADD COLUMN expense_type TEXT;
UPDATE fleet_expenses e SET expense_type = COALESCE(t.code, t.description)
FROM fleet_expense_types t WHERE t.id = e.expense_type_id;
ALTER TABLE fleet_expenses DROP COLUMN expense_type_id;
DROP TABLE IF EXISTS fleet_expense_types;
