package alertnotify

import (
	"strings"
	"testing"
)

func TestTelegramMonitoringBlocksUptimeRestart(t *testing.T) {
	text := telegramMonitoringBlocksWithContext(
		"WARNING",
		"Possível reinício do equipamento",
		"Equipamento com uptime baixo (12 min, limite 60 min) — possível reinício.",
		"OLT Pirapetinga",
		"10.22.25.6",
		"uptime_restart_low",
		map[string]any{
			"observed_uptime_minutes": 12.0,
			"threshold_minutes":       60,
		},
	)
	for _, want := range []string{
		"Possível reinício",
		"Uptime = 12 min",
		"limite 60 min",
		"OLT Pirapetinga",
		"10.22.25.6",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestTelegramMonitoringBlocksOltOnuDrop(t *testing.T) {
	text := telegramMonitoringBlocksWithContext(
		"WARNING",
		"Queda percentual de ONUs online — PON",
		"PON 08 — queda de 5 ONUs online (50% de 10) — OLT Pirapetinga (10.22.25.6).",
		"OLT Pirapetinga",
		"10.22.25.6",
		"olt_onu_drop",
		map[string]any{
			"pon":               "08",
			"drop_online_count": 5.0,
			"drop_online_pct":   50.0,
			"prev_online":       10.0,
			"curr_online":       5.0,
		},
	)
	for _, want := range []string{
		"PON 08",
		"Queda de 5 ONUs",
		"50%",
		"Online: 10 → 5",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestTelegramMonitoringBlocksBngPppoeDrop(t *testing.T) {
	text := telegramMonitoringBlocksWithContext(
		"WARNING",
		"Queda de logins BNG — PPPoE online",
		"BNG BNG-01 (192.168.1.1) — queda de 52 PPPoE online (1200 → 1148) entre coletas SNMP.",
		"BNG-01",
		"192.168.1.1",
		"bng_subscriber_drop",
		map[string]any{
			"subscriber_field": "pppoe_online",
			"metric_id":        "bng_pppoe_drop_count",
			"drop_count":       52.0,
			"prev_online":      1200,
			"curr_online":      1148,
		},
	)
	for _, want := range []string{
		"BNG-01",
		"192.168.1.1",
		"Queda de logins BNG — PPPoE",
		"Queda de 52 PPPoEs",
		"Online: 1200 → 1148",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestTelegramMonitoringBlocksTelemetryUptime(t *testing.T) {
	text := telegramMonitoringBlocksWithContext(
		"WARNING",
		"Telemetria — limiar global",
		"OLT Pirapetinga (10.22.25.6): Uptime está em 12.00 — estado Atenção segundo os seus limiares de alerta.",
		"OLT Pirapetinga",
		"10.22.25.6",
		"telemetry_threshold",
		map[string]any{
			"metric_id":  "uptime_minutes",
			"value":      12.0,
			"value_text": "12 min",
		},
	)
	for _, want := range []string{
		"Uptime abaixo do limiar",
		"Uptime = 12 min",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}