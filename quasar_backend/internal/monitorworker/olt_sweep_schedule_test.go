package monitorworker

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSweepShouldCollectDevice_workerRespectsInterval(t *testing.T) {
	last := time.Now().Add(-60 * time.Second)
	opts := SweepOpts{Source: "worker"}
	if sweepShouldCollectDevice(opts, last, 240*time.Second) {
		t.Fatal("worker should wait per-device interval")
	}
	if !sweepShouldCollectDevice(opts, time.Time{}, 240*time.Second) {
		t.Fatal("never collected should run")
	}
	if !sweepShouldCollectDevice(SweepOpts{Source: "worker", Force: true}, last, 240*time.Second) {
		t.Fatal("force should run")
	}
	if !sweepShouldCollectDevice(SweepOpts{Source: "bootstrap"}, last, 240*time.Second) {
		t.Fatal("bootstrap should run")
	}
}

func TestScheduleOltCollectCandidates_allEligible(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	def := "public"
	devices := []pingableDeviceRow{
		{id: id1, ip: "10.0.0.1", description: "A", brand: "VSOL", model: "V1600G1"},
		{id: id2, ip: "10.0.0.2", description: "B", brand: "ZTE", model: "C320"},
	}
	last := map[uuid.UUID]time.Time{id1: time.Now().Add(-10 * time.Minute)}
	opts := SweepOpts{Source: "worker", Force: true}
	out := scheduleOltCollectCandidates(devices, last, opts, 240*time.Second, &def)
	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(out))
	}
}

func TestOltCollectCandidatesFromDevices_allWithCommunity(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	def := "public"
	devices := []pingableDeviceRow{
		{id: id1, ip: "10.0.0.1", description: "Datacom", brand: "Datacom", model: "DM4610"},
		{id: id2, ip: "10.0.0.2", description: "VSOL", brand: "VSOL", model: "V1600G1"},
	}
	out := oltCollectCandidatesFromDevices(devices, &def)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}
