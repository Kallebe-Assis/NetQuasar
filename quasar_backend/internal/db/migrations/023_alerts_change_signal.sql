-- +goose Up
ALTER TABLE monitoring_runtime
    ADD COLUMN IF NOT EXISTS last_alerts_change_at TIMESTAMPTZ;

UPDATE monitoring_runtime SET last_alerts_change_at = COALESCE(last_alerts_change_at, updated_at, now()) WHERE id = 1;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_monitoring_alerts_change_at() RETURNS trigger AS $$
BEGIN
    UPDATE monitoring_runtime
    SET last_alerts_change_at = now(), updated_at = now()
    WHERE id = 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_alert_instances_bump_alerts_change ON alert_instances;
CREATE TRIGGER trg_alert_instances_bump_alerts_change
    AFTER INSERT OR UPDATE OF closed_at, message, meta, severity ON alert_instances
    FOR EACH ROW
    EXECUTE FUNCTION bump_monitoring_alerts_change_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_alert_instances_bump_alerts_change ON alert_instances;
DROP FUNCTION IF EXISTS bump_monitoring_alerts_change_at();
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS last_alerts_change_at;
