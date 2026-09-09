package vsolparse

import "testing"

func TestMergeOnuRowsTelemetry_keepsRx(t *testing.T) {
	prev := []map[string]any{{"pon": 2, "onu": 1, "rx_pwr": "-23.46", "model": "X"}}
	fresh := []map[string]any{{"pon": 2, "onu": 1, "online": true, "onu_online_sta": 1}}
	out := MergeOnuRowsTelemetry(prev, fresh)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	if out[0]["rx_pwr"] != "-23.46" {
		t.Fatalf("rx %v", out[0]["rx_pwr"])
	}
	if out[0]["online"] != true {
		t.Fatalf("online %v", out[0]["online"])
	}
}

// Reportado pelo utilizador: uma ONU que cai continuava a mostrar a última potência/voltagem/
// temperatura lidas quando ainda estava online, como se fossem valores actuais — devem sumir
// (ficar "-" na UI, ver formatSnmpMetricCell/EM_DASH no frontend) assim que a ONU fica offline,
// mesmo com model/serial (provisionamento) continuando a aparecer normalmente.
func TestMergeOnuRowsTelemetry_clearsLiveTelemetryWhenOffline(t *testing.T) {
	prev := []map[string]any{{
		"pon": 3, "onu": 5, "rx_pwr": "-24.1", "tx_pwr": "2.5", "voltage": "3.3", "temp": "45",
		"bias": "10", "model": "ONU-X", "serial": "ABC123",
	}}
	fresh := []map[string]any{{"pon": 3, "onu": 5, "online": false, "onu_online_sta": 4}}
	out := MergeOnuRowsTelemetry(prev, fresh)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	row := out[0]
	for _, k := range []string{"rx_pwr", "tx_pwr", "voltage", "temp", "bias"} {
		if v, ok := row[k]; ok {
			t.Fatalf("campo de telemetria %q deveria ter sido limpo com ONU offline, veio %v", k, v)
		}
	}
	if row["model"] != "ONU-X" || row["serial"] != "ABC123" {
		t.Fatalf("model/serial (provisionamento) não deviam ser apagados: %+v", row)
	}
	if row["online"] != false {
		t.Fatalf("online %v", row["online"])
	}
}
