package api

import "testing"

func TestBuildDatacomPonRowsFromTable_offlineCol6(t *testing.T) {
	rows := []map[string]any{
		{"suffix": "2.1", "value": "gpon-1/1/1"},
		{"suffix": "3.1", "value_int": 10},
		{"suffix": "4.1", "value_int": 0},
		{"suffix": "5.1", "value_int": 7},
		{"suffix": "6.1", "value_int": 3},
	}
	out := buildDatacomPonRowsFromTable(rows)
	if len(out) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out))
	}
	r := out[0]
	if got := r["onu_total"]; got != 10 {
		t.Errorf("onu_total: got %v want 10", got)
	}
	if got := r["onu_online"]; got != 7 {
		t.Errorf("onu_online: got %v want 7", got)
	}
	if got := r["onu_offline"]; got != 3 {
		t.Errorf("onu_offline: got %v want 3 (col. 6 ponIfDownOnus)", got)
	}
}
