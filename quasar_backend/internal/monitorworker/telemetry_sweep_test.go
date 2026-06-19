package monitorworker

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCycleDue(t *testing.T) {
	now := time.Now()
	past := now.Add(-5 * time.Minute)
	if !cycleDue(&past, 120) {
		t.Fatal("expected due after interval")
	}
	recent := now.Add(-30 * time.Second)
	if cycleDue(&recent, 120) {
		t.Fatal("expected not due yet")
	}
	if !cycleDue(nil, 120) {
		t.Fatal("nil last should be due")
	}
}

func TestTelemetryCycleOutcomeSkipped(t *testing.T) {
	// smoke: funções de registo não panicam com pool nil
	recordTelemetryCycleOutcome(nil, nil, uuid.Nil, "worker", telemetryCycleOutcome{
		Skipped: true, Reason: "test",
	})
}
