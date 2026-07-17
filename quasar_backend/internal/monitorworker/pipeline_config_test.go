package monitorworker

import "testing"

func TestFirstEnabledPingStep(t *testing.T) {
	steps := []PipelineStep{
		{Kind: StepKindTelemetry, Enabled: true},
		{ID: "p1", Kind: StepKindPing, Enabled: false},
		{ID: "p2", Kind: StepKindPing, Enabled: true, Scope: StepScope{Target: "category", Category: "olt"}},
	}
	got := FirstEnabledPingStep(steps)
	if got == nil || got.ID != "p2" {
		t.Fatalf("expected p2, got %+v", got)
	}
}

func TestEnsureBngPipelineStep(t *testing.T) {
	steps := []PipelineStep{
		{Kind: StepKindPing, Enabled: true},
		{Kind: StepKindTelemetry, Enabled: true},
		{Kind: StepKindMikrotik, Enabled: true},
	}
	out := ensureBngPipelineStep(steps)
	if len(out) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(out))
	}
	if out[2].Kind != StepKindBng {
		t.Fatalf("expected bng at index 2, got %s", out[2].Kind)
	}
	if ensureBngPipelineStep(append(out, PipelineStep{Kind: StepKindBng})) == nil {
		t.Fatal("unexpected nil")
	}
}

func TestEnsureMonitoringBaselineSteps(t *testing.T) {
	in := []PipelineStep{
		{ID: "ping-off", Kind: StepKindPing, Enabled: false},
		{ID: "bng-old", Kind: StepKindBng, Enabled: false, Options: StepOptions{BngMode: "totals"}},
		{ID: "pon-old", Kind: StepKindOltOnu, Enabled: true, Options: StepOptions{OltOnuMode: "pon_status"}},
		{ID: "counts-old", Kind: StepKindOltOnu, Enabled: true, Options: StepOptions{OltOnuMode: "onu_counts"}},
	}
	out := ensureMonitoringBaselineSteps(in)

	required := map[string]bool{}
	baseline := 0
	for _, step := range out {
		if step.Enabled {
			required[step.Kind] = true
		}
		if step.Kind == StepKindBng && step.Options.BngMode != "monitoring" {
			t.Fatalf("BNG deve usar monitoring: %+v", step)
		}
		if step.Kind == StepKindOltOnu && step.Enabled && step.Options.OltOnuMode == "baseline" {
			baseline++
		}
		if step.Kind == StepKindOltOnu && step.Enabled &&
			(step.Options.OltOnuMode == "pon_status" || step.Options.OltOnuMode == "onu_counts") {
			t.Fatalf("tier leve duplicado permaneceu activo: %+v", step)
		}
	}
	for _, kind := range []string{StepKindPing, StepKindTelemetry, StepKindBng, StepKindMikrotik, StepKindSwitch} {
		if !required[kind] {
			t.Fatalf("passo obrigatório ausente: %s", kind)
		}
	}
	if baseline != 1 {
		t.Fatalf("esperava uma linha-base OLT, obteve %d", baseline)
	}
}
