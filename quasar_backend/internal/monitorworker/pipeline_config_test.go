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
