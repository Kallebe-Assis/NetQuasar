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

func TestFirstEnabledPingStep_none(t *testing.T) {
	if FirstEnabledPingStep([]PipelineStep{{Kind: StepKindPing, Enabled: false}}) != nil {
		t.Fatal("expected nil")
	}
}
