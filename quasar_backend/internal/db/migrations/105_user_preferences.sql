-- +goose Up
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS preferences JSONB NOT NULL DEFAULT '{
        "theme": "dark",
        "alert_toast_everywhere": true,
        "alert_sound_enabled": true,
        "alert_sound_id": "builtin:alert"
    }'::jsonb;

UPDATE users u
SET preferences = COALESCE(u.preferences, '{}'::jsonb) || jsonb_build_object(
    'theme', COALESCE((SELECT theme FROM settings_ui WHERE id = 1), 'dark'),
    'alert_toast_everywhere', true,
    'alert_sound_enabled', true,
    'alert_sound_id', 'builtin:alert'
)
WHERE NOT (COALESCE(u.preferences, '{}'::jsonb) ? 'alert_sound_enabled');

ALTER TABLE users
    ALTER COLUMN preferences SET DEFAULT '{
        "theme": "dark",
        "alert_toast_everywhere": true,
        "alert_sound_enabled": true,
        "alert_sound_id": "builtin:alert"
    }'::jsonb;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS preferences;
