-- +goose Up
CREATE TABLE fleet_expenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number BIGSERIAL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    vehicle_id UUID NOT NULL REFERENCES fleet_vehicles(id) ON DELETE RESTRICT,
    expense_type TEXT NOT NULL CHECK (expense_type IN (
        'preventive_maintenance',
        'corrective_maintenance',
        'wash',
        'tire',
        'fine',
        'documentation'
    )),
    description TEXT NOT NULL,
    unit_price DOUBLE PRECISION NOT NULL CHECK (unit_price >= 0),
    quantity DOUBLE PRECISION NOT NULL CHECK (quantity > 0),
    total_amount DOUBLE PRECISION NOT NULL CHECK (total_amount >= 0),
    odometer DOUBLE PRECISION,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX fleet_expenses_vehicle_at_idx ON fleet_expenses (vehicle_id, occurred_at DESC);
CREATE INDEX fleet_expenses_occurred_at_idx ON fleet_expenses (occurred_at DESC);

ALTER TABLE fleet_odometer_readings DROP CONSTRAINT IF EXISTS fleet_odometer_readings_source_check;
ALTER TABLE fleet_odometer_readings ADD CONSTRAINT fleet_odometer_readings_source_check
    CHECK (source IN ('fueling', 'manual', 'maintenance', 'import', 'expense'));

-- +goose Down
ALTER TABLE fleet_odometer_readings DROP CONSTRAINT IF EXISTS fleet_odometer_readings_source_check;
ALTER TABLE fleet_odometer_readings ADD CONSTRAINT fleet_odometer_readings_source_check
    CHECK (source IN ('fueling', 'manual', 'maintenance', 'import'));
DROP TABLE IF EXISTS fleet_expenses;
