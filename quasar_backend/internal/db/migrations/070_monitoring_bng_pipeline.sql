-- +goose Up
ALTER TABLE monitoring_intervals
    ADD COLUMN IF NOT EXISTS bng_timeout_ms INT NOT NULL DEFAULT 120000;

ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_bng_cycle_at TIMESTAMPTZ;

UPDATE monitoring_intervals
SET pipeline_steps = pipeline_steps || '[
  {"id":"bng-subscribers","kind":"bng","enabled":true,"scope":{"target":"category","category":"bng"},"options":{"bng_mode":"totals"}}
]'::jsonb
WHERE id = 1
  AND jsonb_array_length(pipeline_steps) > 0
  AND NOT EXISTS (
    SELECT 1 FROM jsonb_array_elements(pipeline_steps) s WHERE s->>'kind' = 'bng'
  );

-- +goose Down
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_bng_cycle_at;
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS bng_timeout_ms;
