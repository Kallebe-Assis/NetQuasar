package oltcollect

import "testing"

func TestParseStepsArray(t *testing.T) {
	raw := `[{"method":"snmp_walk","store_as":"zte_onu_online_table","oid_field":"onu_online_oid"}]`
	steps := ParseSteps([]byte(raw))
	if len(steps) != 1 || steps[0].Method != MethodSNMPWalk {
		t.Fatalf("steps: %+v", steps)
	}
}

func TestParseStepsWrapped(t *testing.T) {
	raw := `{"steps":[{"method":"telnet","command":"show version"}]}`
	steps := ParseSteps([]byte(raw))
	if len(steps) != 1 || steps[0].Method != MethodTelnet {
		t.Fatalf("steps: %+v", steps)
	}
}

func TestEnabledSteps(t *testing.T) {
	f := false
	steps := EnabledSteps([]Step{{Method: MethodSNMPWalk}, {Method: MethodTelnet, Enabled: &f}})
	if len(steps) != 1 {
		t.Fatalf("expected 1 enabled, got %d", len(steps))
	}
}

func TestEffectiveCollectionSteps(t *testing.T) {
	t.Run("uses enabled steps", func(t *testing.T) {
		steps := EffectiveCollectionSteps(Profile{
			Steps: []Step{{Method: MethodSNMPWalk}},
		})
		if len(steps) != 1 || steps[0].Method != MethodSNMPWalk {
			t.Fatalf("steps: %+v", steps)
		}
	})
	t.Run("falls back to metrics", func(t *testing.T) {
		steps := EffectiveCollectionSteps(Profile{
			OnuMetrics: OnuMetricsConfig{
				MetricSerial: {Enabled: true, OID: "1.3.6.1.2.1"},
			},
		})
		if len(steps) != 1 || steps[0].Method != MethodOnuMetricsCollect {
			t.Fatalf("steps: %+v", steps)
		}
	})
}

func TestEffectivePeriodicSteps_prefersMetrics(t *testing.T) {
	steps := EffectivePeriodicSteps(Profile{
		Steps: []Step{{Method: MethodOnuSNMPWalk}},
		OnuMetrics: OnuMetricsConfig{
			MetricSerial: {Enabled: true, OID: "1.3.6.1.2.1"},
		},
	})
	if len(steps) != 1 || steps[0].Method != MethodOnuSNMPWalk {
		t.Fatalf("periodic should match manual profile steps, got %+v", steps)
	}
}
