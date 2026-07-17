package monitorworker

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

func TestPeriodicCollectionSteps_doesNotPrependWalkForVsolMetrics(t *testing.T) {
	steps := periodicCollectionSteps(oltcollect.Profile{
		Steps: []oltcollect.Step{{Method: oltcollect.MethodOnuMetricsCollect}},
	}, "VSOL", "full")
	if len(steps) != 1 {
		t.Fatalf("expected 1 step (sem walk duplicado), got %d", len(steps))
	}
	if steps[0].Method != oltcollect.MethodOnuMetricsCollect {
		t.Fatalf("unexpected steps: %+v", steps)
	}
}

func TestPeriodicCollectionSteps_unchangedForDatacom(t *testing.T) {
	in := []oltcollect.Step{{Method: oltcollect.MethodSNMPWalk}, {Method: oltcollect.MethodDatacomBuildPons}}
	steps := periodicCollectionSteps(oltcollect.Profile{Steps: in}, "Datacom", "full")
	if len(steps) != 2 || steps[0].Method != oltcollect.MethodSNMPWalk {
		t.Fatalf("datacom steps changed: %+v", steps)
	}
}

func TestPeriodicCollectionSteps_skipsTelnetOnPartial(t *testing.T) {
	steps := periodicCollectionSteps(oltcollect.Profile{
		Steps:     []oltcollect.Step{{Method: oltcollect.MethodOnuMetricsCollect}},
		OnuReport: oltcollect.OnuReportConfig{Enabled: true, Command: "show"},
	}, "ZTE", "pon_status")
	for _, s := range steps {
		if s.Method == oltcollect.MethodOnuTelnetReport {
			t.Fatal("telnet step should be omitted in pon_status mode")
		}
	}
}
