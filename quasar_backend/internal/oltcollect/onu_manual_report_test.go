package oltcollect

import (
	"testing"
	"time"
)

func TestApplyOnuReportResultsReplacesOldValues(t *testing.T) {
	summary := map[string]any{"vsol_onu_rows": []any{
		map[string]any{"pon": float64(1), "onu": float64(2), "online": true, "rx_pwr": "-30.0 dBm", "rx_dbm": -30.0,
			"telnet_fields": map[string]any{"RX": "-30.0 dBm", "Distancia": "1 m"}},
		map[string]any{"pon": float64(1), "onu": float64(3), "online": false, "serial": "AAAA00000001"},
	}}
	n := ApplyOnuReportResults(summary, []OnuReportResult{
		{Pon: 1, Onu: 2, Fields: map[string]string{"RX": "-21.5 dBm", "Temperatura": "40"}},
		{Pon: 1, Onu: 3, Fields: map[string]string{"RX": "-19.0 dBm"}},
	}, time.Now())
	if n != 2 {
		t.Fatalf("n=%d", n)
	}
	rows := OnuRowsFromSummary(summary)
	if rows[0]["rx_pwr"] != "-21.5 dBm" || rows[0]["rx_dbm"] != -21.5 || rows[0]["temp"] != "40" {
		t.Fatalf("online row not replaced: %#v", rows[0])
	}
	if tf := rows[0]["telnet_fields"].(map[string]any); tf["RX"] != "-21.5 dBm" || tf["Distancia"] != nil {
		t.Fatalf("telnet_fields not replaced: %#v", tf)
	}
	if _, has := rows[1]["rx_pwr"]; has {
		t.Fatalf("offline row got RX: %#v", rows[1])
	}
}

func TestApplyOnuReportResultsEstadoDefinesOnline(t *testing.T) {
	summary := map[string]any{"vsol_onu_rows": []any{
		map[string]any{"pon": float64(1), "onu": float64(2), "online": false},
		map[string]any{"pon": float64(1), "onu": float64(3), "online": true, "rx_pwr": "-20 dBm"},
	}}
	ApplyOnuReportResults(summary, []OnuReportResult{
		{Pon: 1, Onu: 2, Fields: map[string]string{"Estado": "working", "RX": "-22.0 dBm"}},
		{Pon: 1, Onu: 3, Fields: map[string]string{"Estado": "LOS", "RX": "-20 dBm"}},
	}, time.Now())
	rows := OnuRowsFromSummary(summary)
	if rows[0]["online"] != true || rows[0]["rx_pwr"] != "-22.0 dBm" {
		t.Fatalf("working should be online with RX: %#v", rows[0])
	}
	if rows[1]["online"] != false {
		t.Fatalf("LOS should be offline: %#v", rows[1])
	}
	if _, has := rows[1]["rx_pwr"]; has {
		t.Fatalf("offline RX kept: %#v", rows[1])
	}
}
