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
