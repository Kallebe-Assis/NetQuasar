-- +goose Up
ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS pipeline_cycle_seconds INT NOT NULL DEFAULT 120,
    ADD COLUMN IF NOT EXISTS mikrotik_timeout_ms INT NOT NULL DEFAULT 120000,
    ADD COLUMN IF NOT EXISTS pipeline_steps JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_pipeline_cycle_at TIMESTAMPTZ;

UPDATE monitoring_intervals SET pipeline_steps = '[
  {"id":"ping-all","kind":"ping","enabled":true,"scope":{"target":"all"},"options":{}},
  {"id":"telemetry-all","kind":"telemetry","enabled":true,"scope":{"target":"all"},"options":{"telemetry_fields":[]}},
  {"id":"mikrotik-if","kind":"mikrotik","enabled":true,"scope":{"target":"category","category":"mikrotik"},"options":{"mikrotik_mode":"full"}},
  {"id":"olt-if","kind":"interfaces_olt","enabled":true,"scope":{"target":"category","category":"olt"},"options":{}},
  {"id":"olt-onu","kind":"olt_onu","enabled":true,"scope":{"target":"category","category":"olt"},"options":{"olt_onu_mode":"full"}}
]'::jsonb
WHERE id = 1 AND (pipeline_steps IS NULL OR pipeline_steps = '[]'::jsonb);

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_pipeline_cycle_at;
ALTER TABLE monitoring_intervals
    DROP COLUMN IF EXISTS pipeline_steps,
    DROP COLUMN IF EXISTS mikrotik_timeout_ms,
    DROP COLUMN IF EXISTS pipeline_cycle_seconds;
