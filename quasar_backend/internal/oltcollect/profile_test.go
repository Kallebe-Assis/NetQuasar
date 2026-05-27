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
