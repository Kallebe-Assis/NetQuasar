package oltcollect

import "testing"

func TestIsOltSnapshotIncomplete_flags(t *testing.T) {
	if !IsOltSnapshotIncomplete(map[string]any{"onu_walk_truncated": true}) {
		t.Fatal("expected incomplete on onu_walk_truncated")
	}
	if !IsOltSnapshotIncomplete(map[string]any{"onu_metrics_incomplete": true}) {
		t.Fatal("expected incomplete on onu_metrics_incomplete flag")
	}
	if IsOltSnapshotIncomplete(map[string]any{"onu_metrics_walks": []any{
		map[string]any{"metric": MetricRxPower, "truncated": true, "matched_rows": 100},
	}}) {
		t.Fatal("rx-only truncation should not mark incomplete alone")
	}
	if !IsOltSnapshotIncomplete(map[string]any{"onu_metrics_walks": []any{
		map[string]any{"metric": MetricStatus, "truncated": true, "matched_rows": 50},
		map[string]any{"metric": MetricSerial, "truncated": false, "matched_rows": 400},
	}}) {
		t.Fatal("expected incomplete when status truncated ([]any)")
	}
	if !IsOltSnapshotIncomplete(map[string]any{"onu_metrics_walks": []map[string]any{
		{"metric": MetricStatus, "truncated": true, "matched_rows": 200, "note": "context deadline exceeded"},
		{"metric": MetricSerial, "truncated": true, "matched_rows": 117},
		{"metric": MetricRxPower, "matched_rows": 378},
	}}) {
		t.Fatal("expected incomplete when status truncated ([]map[string]any) — regressão em memória")
	}
	if !IsOltSnapshotIncomplete(map[string]any{"onu_metrics_walks": []map[string]any{
		{"metric": MetricStatus, "matched_rows": 200},
		{"metric": MetricRxPower, "matched_rows": 378},
	}}) {
		t.Fatal("expected incomplete when status rows << rx rows")
	}
}
