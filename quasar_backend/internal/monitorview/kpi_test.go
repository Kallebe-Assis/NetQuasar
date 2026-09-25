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

func TestMemoryPercentFromVars(t *testing.T) {
	// hrStorage: RAM identificada só pelo tipo (MikroTik: "main memory").
	vars := map[string]string{
		"1.3.6.1.2.1.25.2.3.1.3.65536": "main memory",
		"1.3.6.1.2.1.25.2.3.1.2.65536": "1.3.6.1.2.1.25.2.1.2",
		"1.3.6.1.2.1.25.2.3.1.5.65536": "1000",
		"1.3.6.1.2.1.25.2.3.1.6.65536": "250",
	}
	if got := memoryPercentFromVars(vars, metricsProfile{}); got == nil || *got != 25 {
		t.Fatalf("hrStorage: %v", got)
	}
	// UCD: total e disponível.
	ucd := map[string]string{"1.3.6.1.4.1.2021.4.5.0": "2000", "1.3.6.1.4.1.2021.4.6.0": "500"}
	if got := memoryPercentFromVars(ucd, metricsProfile{}); got == nil || *got != 75 {
		t.Fatalf("ucd: %v", got)
	}
	// Perfil com memAvailReal como "usado": o valor é o que sobra.
	prof := metricsProfile{MemoryUsedOID: ".1.3.6.1.4.1.2021.4.6.0", MemorySizeOID: "1.3.6.1.4.1.2021.4.5.0"}
	if got := memoryPercentFromVars(ucd, prof); got == nil || *got != 75 {
		t.Fatalf("profile avail: %v", got)
	}
	if got := memoryPercentFromVars(map[string]string{}, metricsProfile{}); got != nil {
		t.Fatalf("empty: %v", got)
	}
}

func TestMergeMikrotikKPIs_memoryInBytes(t *testing.T) {
	metrics := []byte(`{"mikrotik_collection":{"fields":{"memory_used":{"ok":true,"value":268435456},"memory_total":{"ok":true,"value":1073741824}}}}`)
	var cpu, mem, temp *float64
	up := ""
	mergeMikrotikKPIs(metrics, &cpu, &mem, &temp, &up)
	if mem == nil || *mem != 25 {
		t.Fatalf("mikrotik memória em bytes: %v", mem)
	}
}
