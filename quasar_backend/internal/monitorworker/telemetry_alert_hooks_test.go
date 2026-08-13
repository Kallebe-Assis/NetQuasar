package monitorworker

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestScalarFromCollectionBag_CollectOutput(t *testing.T) {
	out := mikrotikcollect.CollectOutput{
		Fields: map[string]mikrotikcollect.FieldResult{
			"board_temperature": {Key: "board_temperature", OK: true, Value: 72.5},
			"cpu_load":          {Key: "cpu_load", OK: true, Value: 81.0},
		},
	}
	temp := scalarFromCollectionBag(out, "board_temperature")
	if temp == nil || *temp != 72.5 {
		t.Fatalf("board_temperature: got %v", temp)
	}
	cpu := scalarFromCollectionBag(&out, "cpu_load")
	if cpu == nil || *cpu != 81.0 {
		t.Fatalf("cpu_load: got %v", cpu)
	}
	if scalarFromCollectionBag(out, "missing") != nil {
		t.Fatal("missing field should be nil")
	}
}

func TestScalarFromCollectionBag_MapJSON(t *testing.T) {
	raw := map[string]any{
		"fields": map[string]any{
			"temperature": map[string]any{"ok": true, "value": 68.0},
			"dead":        map[string]any{"ok": false, "value": 99.0},
		},
	}
	got := scalarFromCollectionBag(raw, "temperature")
	if got == nil || *got != 68.0 {
		t.Fatalf("temperature map: got %v", got)
	}
	if scalarFromCollectionBag(raw, "dead") != nil {
		t.Fatal("ok=false should be nil")
	}
}

func TestParseTempCFromTelemetry_PrefersBoardTemperature(t *testing.T) {
	metrics := map[string]any{
		"mikrotik_collection": mikrotikcollect.CollectOutput{
			Fields: map[string]mikrotikcollect.FieldResult{
				"board_temperature": {OK: true, Value: 65.0},
				"cpu_temperature":   {OK: true, Value: 90.0},
			},
		},
	}
	got := parseTempCFromTelemetry(metrics, nil)
	if got == nil || *got != 65.0 {
		t.Fatalf("want board 65, got %v", got)
	}
}

func TestParseMikrotikCPUTempC(t *testing.T) {
	metrics := map[string]any{
		"mikrotik_collection": mikrotikcollect.CollectOutput{
			Fields: map[string]mikrotikcollect.FieldResult{
				"board_temperature": {OK: true, Value: 65.0},
				"cpu_temperature":   {OK: true, Value: 82.0},
			},
		},
	}
	got := parseMikrotikCPUTempC(metrics)
	if got == nil || *got != 82.0 {
		t.Fatalf("want cpu 82, got %v", got)
	}
}

func TestParseCPUFromTelemetry_CollectOutput(t *testing.T) {
	metrics := map[string]any{
		"mikrotik_collection": mikrotikcollect.CollectOutput{
			Fields: map[string]mikrotikcollect.FieldResult{
				"cpu_load": {OK: true, Value: 42.0},
			},
		},
	}
	got := parseCPUFromTelemetry(metrics, []probing.SNMPVar{})
	if got == nil || *got != 42.0 {
		t.Fatalf("cpu: got %v", got)
	}
}

func TestParseMemoryFromTelemetry_Percent(t *testing.T) {
	metrics := map[string]any{
		"mikrotik_collection": mikrotikcollect.CollectOutput{
			Fields: map[string]mikrotikcollect.FieldResult{
				"memory_used":  {OK: true, Value: 250.0},
				"memory_total": {OK: true, Value: 500.0},
			},
		},
	}
	got := parseMemoryFromTelemetry(metrics, nil)
	if got == nil || *got != 50.0 {
		t.Fatalf("memory pct: got %v", got)
	}
}
