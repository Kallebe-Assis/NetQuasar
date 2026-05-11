-- +goose Up
-- Sessão de monitoramento + opção de refresh de inventário ao iniciar; marca por equipamento/sessão.

ALTER TABLE monitoring_runtime
	ADD COLUMN IF NOT EXISTS mon_session_id UUID NOT NULL DEFAULT gen_random_uuid(),
	ADD COLUMN IF NOT EXISTS offer_snmp_inventory_refresh BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS monitoring_snmp_inv_refresh_done (
	device_id UUID NOT NULL,
	session_id UUID NOT NULL,
	done_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (device_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_mon_snmp_inv_refresh_session ON monitoring_snmp_inv_refresh_done (session_id);

-- 0 = desligado; >0 = minutos de uptime abaixo dos quais se emite alerta de reinício.
ALTER TABLE monitoring_intervals
	ADD COLUMN IF NOT EXISTS uptime_restart_alert_minutes INT NOT NULL DEFAULT 0
		CHECK (uptime_restart_alert_minutes >= 0 AND uptime_restart_alert_minutes <= 10080);

-- +goose Down
ALTER TABLE monitoring_intervals DROP COLUMN IF EXISTS uptime_restart_alert_minutes;
DROP INDEX IF EXISTS idx_mon_snmp_inv_refresh_session;
DROP TABLE IF EXISTS monitoring_snmp_inv_refresh_done;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS offer_snmp_inventory_refresh;
ALTER TABLE monitoring_runtime DROP COLUMN IF EXISTS mon_session_id;
