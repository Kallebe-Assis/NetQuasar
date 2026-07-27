package monitorworker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestFirstEnabledTelemetryStep(t *testing.T) {
	steps := []PipelineStep{
		{ID: "p", Kind: StepKindPing, Enabled: true},
		{ID: "t", Kind: StepKindTelemetry, Enabled: true},
	}
	got := FirstEnabledTelemetryStep(steps)
	if got == nil || got.ID != "t" {
		t.Fatalf("expected telemetry step, got %#v", got)
	}
}

func TestForEachLimitedRespectsConcurrency(t *testing.T) {
	var peak, current atomic.Int32
	n := 20
	limit := 4
	forEachLimited(context.Background(), n, limit, func(i int) {
		c := current.Add(1)
		for {
			p := peak.Load()
			if c <= p || peak.CompareAndSwap(p, c) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		current.Add(-1)
	})
	if peak.Load() > int32(limit) {
		t.Fatalf("peak concurrency %d > limit %d", peak.Load(), limit)
	}
	if peak.Load() < 1 {
		t.Fatal("expected some concurrency")
	}
}
