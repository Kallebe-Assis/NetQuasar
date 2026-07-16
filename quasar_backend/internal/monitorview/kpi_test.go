package monitorview

import (
	"testing"
	"time"
)

func TestKPIsFromProbeDetail_roundTrip(t *testing.T) {
	kpis := DeviceKPIs{
		CPUPercent:    ptrFloat(42),
		MemoryPercent: ptrFloat(71),
		TemperatureC:  ptrFloat(38),
		Uptime:        "2d 03h",
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	patch := KPIsDetailPatch(kpis)
	got, ok := KPIsFromProbeDetail(patch)
	if !ok {
		t.Fatal("expected cached KPIs")
	}
	if got.CPUPercent == nil || *got.CPUPercent != 42 {
		t.Fatalf("cpu %v", got.CPUPercent)
	}
	if got.Uptime != "2d 03h" {
		t.Fatalf("uptime %q", got.Uptime)
	}
}
