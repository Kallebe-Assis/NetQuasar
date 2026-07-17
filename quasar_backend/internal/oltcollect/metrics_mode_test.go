package oltcollect

import "testing"

func TestFilterOnuMetricsByModeBaseline(t *testing.T) {
	cfg := OnuMetricsConfig{
		MetricStatus:     {Enabled: false, OID: "status"},
		MetricPonStatus:  {Enabled: false, OID: "pon-status"},
		MetricPonTxPower: {Enabled: false, OID: "pon-tx"},
		MetricRxPower:    {Enabled: true, OID: "onu-rx"},
		MetricSerial:     {Enabled: true, OID: "serial"},
	}
	got := FilterOnuMetricsByMode(cfg, "baseline")
	for _, key := range []string{MetricStatus, MetricPonStatus, MetricPonTxPower} {
		if !got[key].Enabled {
			t.Fatalf("métrica obrigatória não activada: %s", key)
		}
	}
	for _, key := range []string{MetricRxPower, MetricSerial} {
		if got[key].Enabled {
			t.Fatalf("métrica pesada não deveria estar na linha-base: %s", key)
		}
	}
}
