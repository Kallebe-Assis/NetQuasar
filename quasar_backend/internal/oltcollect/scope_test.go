package oltcollect

import "testing"

func TestStepsForScope_onuSnmpWalkOnly(t *testing.T) {
	en := true
	in := []Step{
		{Method: MethodOnuSNMPWalk, Enabled: &en},
		{Method: MethodIfMibRefresh, Enabled: &en},
	}
	out := StepsForScope(in, ScopeOnu)
	if len(out) != 1 || out[0].Method != MethodOnuSNMPWalk {
		t.Fatalf("got %+v", out)
	}
}

func TestStepsForScope_onuSkipsIfRefresh(t *testing.T) {
	en := true
	in := []Step{
		{Method: MethodIfMibRefresh, Enabled: &en},
		{Method: MethodVsolOnuCollect, Enabled: &en},
	}
	out := StepsForScope(in, ScopeOnu)
	if len(out) != 2 {
		t.Fatalf("got %d steps", len(out))
	}
	if out[0].Method != MethodIfMibSnapshot || out[1].Method != MethodVsolOnuCollect {
		t.Fatalf("steps: %+v", out)
	}
}

func TestStepsForScope_fullUnchanged(t *testing.T) {
	en := true
	in := []Step{
		{Method: MethodIfMibRefresh, Enabled: &en},
		{Method: MethodVsolOnuCollect, Enabled: &en},
	}
	out := StepsForScope(in, ScopeFull)
	if len(out) != 2 {
		t.Fatalf("got %d", len(out))
	}
}
