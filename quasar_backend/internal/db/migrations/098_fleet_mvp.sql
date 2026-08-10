-- +goose Up
-- Módulo Frota (MVP): cadastros, abastecimentos, odómetro, alertas e settings.

CREATE TABLE fleet_cost_centers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL,
    description TEXT NOT NULL,
    parent_id UUID REFERENCES fleet_cost_centers(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fleet_cost_centers_code_uq UNIQUE (code)
);

CREATE TABLE fleet_fuels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description TEXT NOT NULL,
    code TEXT,
    fuel_type TEXT,
    unit TEXT NOT NULL DEFAULT 'L',
    active BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX fleet_fuels_code_uq ON fleet_fuels (lower(trim(code))) WHERE code IS NOT NULL AND trim(code) <> '';

CREATE TABLE fleet_stations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description TEXT NOT NULL,
    legal_name TEXT,
    trade_name TEXT,
    cnpj TEXT,
    phone TEXT,
    email TEXT,
    zip TEXT,
    address TEXT,
    number TEXT,
    complement TEXT,
    neighborhood TEXT,
    city TEXT,
    uf TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    station_kind TEXT NOT NULL DEFAULT 'other' CHECK (station_kind IN ('conveniado', 'proprio', 'fornecedor', 'other')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE fleet_station_fuels (
    station_id UUID NOT NULL REFERENCES fleet_stations(id) ON DELETE CASCADE,
    fuel_id UUID NOT NULL REFERENCES fleet_fuels(id) ON DELETE CASCADE,
    PRIMARY KEY (station_id, fuel_id)
);

CREATE TABLE fleet_vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description TEXT NOT NULL,
    plate TEXT NOT NULL,
    year INT,
    model TEXT,
    color TEXT,
    city TEXT,
    uf TEXT,
    vehicle_type TEXT,
    category TEXT,
    primary_fuel_id UUID REFERENCES fleet_fuels(id) ON DELETE SET NULL,
    tank_capacity_liters DOUBLE PRECISION,
    expected_km_per_liter DOUBLE PRECISION,
    min_km_per_liter DOUBLE PRECISION,
    max_km_per_liter DOUBLE PRECISION,
    odometer_current DOUBLE PRECISION NOT NULL DEFAULT 0,
    hourmeter_current DOUBLE PRECISION,
    cost_center_id UUID REFERENCES fleet_cost_centers(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'maintenance', 'sold', 'written_off', 'stopped', 'rented')),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX fleet_vehicles_plate_uq ON fleet_vehicles (upper(replace(replace(trim(plate), '-', ''), ' ', '')));

CREATE TABLE fleet_drivers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    cpf TEXT,
    rg TEXT,
    phone TEXT,
    email TEXT,
    license_number TEXT,
    license_category TEXT,
    license_expires_on DATE,
    city TEXT,
    uf TEXT,
    user_id UUID UNIQUE REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'blocked')),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX fleet_drivers_cpf_uq ON fleet_drivers (regexp_replace(cpf, '[^0-9]', '', 'g'))
  WHERE cpf IS NOT NULL AND trim(cpf) <> '';

CREATE TABLE fleet_driver_vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id UUID NOT NULL REFERENCES fleet_drivers(id) ON DELETE CASCADE,
    vehicle_id UUID NOT NULL REFERENCES fleet_vehicles(id) ON DELETE CASCADE,
    starts_on DATE,
    ends_on DATE,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fleet_driver_vehicles_pair_uq UNIQUE (driver_id, vehicle_id, starts_on)
);

CREATE TABLE fleet_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    consumption_tolerance_pct DOUBLE PRECISION NOT NULL DEFAULT 20,
    price_tolerance_pct DOUBLE PRECISION NOT NULL DEFAULT 15,
    min_minutes_between_fuelings INT NOT NULL DEFAULT 60,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO fleet_settings (id) VALUES (1);

CREATE TABLE fleet_fuelings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number BIGSERIAL,
    fueled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    vehicle_id UUID NOT NULL REFERENCES fleet_vehicles(id) ON DELETE RESTRICT,
    driver_id UUID REFERENCES fleet_drivers(id) ON DELETE SET NULL,
    station_id UUID REFERENCES fleet_stations(id) ON DELETE SET NULL,
    fuel_id UUID NOT NULL REFERENCES fleet_fuels(id) ON DELETE RESTRICT,
    cost_center_id UUID REFERENCES fleet_cost_centers(id) ON DELETE SET NULL,
    liters DOUBLE PRECISION NOT NULL CHECK (liters > 0),
    price_per_liter DOUBLE PRECISION NOT NULL CHECK (price_per_liter >= 0),
    total_amount DOUBLE PRECISION NOT NULL CHECK (total_amount >= 0),
    odometer_previous DOUBLE PRECISION,
    odometer_current DOUBLE PRECISION,
    km_driven DOUBLE PRECISION,
    hourmeter_previous DOUBLE PRECISION,
    hourmeter_current DOUBLE PRECISION,
    hours_worked DOUBLE PRECISION,
    km_per_liter DOUBLE PRECISION,
    cost_per_km DOUBLE PRECISION,
    liters_per_100km DOUBLE PRECISION,
    payment_method TEXT,
    document_number TEXT,
    invoice_number TEXT,
    receipt_path TEXT,
    odometer_photo_path TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX fleet_fuelings_vehicle_at_idx ON fleet_fuelings (vehicle_id, fueled_at DESC);
CREATE INDEX fleet_fuelings_fueled_at_idx ON fleet_fuelings (fueled_at DESC);

CREATE TABLE fleet_odometer_readings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id UUID NOT NULL REFERENCES fleet_vehicles(id) ON DELETE CASCADE,
    reading_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    odometer DOUBLE PRECISION NOT NULL,
    hourmeter DOUBLE PRECISION,
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('fueling', 'manual', 'maintenance', 'import')),
    fueling_id UUID REFERENCES fleet_fuelings(id) ON DELETE SET NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX fleet_odometer_vehicle_at_idx ON fleet_odometer_readings (vehicle_id, reading_at DESC);

CREATE TABLE fleet_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    severity TEXT NOT NULL DEFAULT 'attention' CHECK (severity IN ('critical', 'attention', 'info')),
    alert_type TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    vehicle_id UUID REFERENCES fleet_vehicles(id) ON DELETE SET NULL,
    fueling_id UUID REFERENCES fleet_fuelings(id) ON DELETE SET NULL,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX fleet_alerts_open_idx ON fleet_alerts (created_at DESC) WHERE acknowledged_at IS NULL;

-- Seeds úteis
INSERT INTO fleet_fuels (description, code, fuel_type, unit) VALUES
  ('Gasolina comum', 'GAS', 'gasolina', 'L'),
  ('Gasolina aditivada', 'GAS-AD', 'gasolina', 'L'),
  ('Etanol', 'ETA', 'etanol', 'L'),
  ('Diesel S10', 'DSL-S10', 'diesel', 'L'),
  ('Diesel S500', 'DSL-S500', 'diesel', 'L'),
  ('Arla 32', 'ARLA', 'arla', 'L');

INSERT INTO fleet_cost_centers (code, description) VALUES
  ('001', 'Administrativo'),
  ('002', 'Comercial'),
  ('003', 'Manutenção'),
  ('004', 'Instalação'),
  ('005', 'Suporte'),
  ('006', 'Logística'),
  ('007', 'NOC'),
  ('008', 'Operações');

-- +goose Down
DROP TABLE IF EXISTS fleet_alerts;
DROP TABLE IF EXISTS fleet_odometer_readings;
DROP TABLE IF EXISTS fleet_fuelings;
DROP TABLE IF EXISTS fleet_settings;
DROP TABLE IF EXISTS fleet_driver_vehicles;
DROP TABLE IF EXISTS fleet_drivers;
DROP TABLE IF EXISTS fleet_vehicles;
DROP TABLE IF EXISTS fleet_station_fuels;
DROP TABLE IF EXISTS fleet_stations;
DROP TABLE IF EXISTS fleet_fuels;
DROP TABLE IF EXISTS fleet_cost_centers;
