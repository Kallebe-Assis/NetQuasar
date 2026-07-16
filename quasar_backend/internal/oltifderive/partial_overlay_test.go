package oltifderive

import "testing"

func TestOverlayPartialPonSnapshot_preservesOptical(t *testing.T) {
	prev := []map[string]any{
		{"id": "01", "pon": 1, "onu_online": 10, "onu_offline": 2, "rx_dbm": -18.5, "oper_status": 1},
	}
	cur := []map[string]any{
		{"id": "01", "pon": 1, "oper_status": 2, "status": "down"},
	}
	out := OverlayPartialPonSnapshot(prev, cur, "pon_status")
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0]["oper_status"] != 2 {
		t.Fatalf("oper_status=%v", out[0]["oper_status"])
	}
	if out[0]["rx_dbm"] != -18.5 {
		t.Fatalf("rx_dbm lost: %v", out[0]["rx_dbm"])
	}
	if out[0]["onu_online"] != 10 {
		t.Fatalf("onu_online lost: %v", out[0]["onu_online"])
	}
}

func TestOverlayPartialPonSnapshot_updatesCounts(t *testing.T) {
	prev := []map[string]any{
		{"id": "01", "pon": 1, "onu_online": 10, "onu_offline": 2, "rx_dbm": -18.5},
	}
	cur := []map[string]any{
		{"id": "01", "pon": 1, "onu_online": 8, "onu_offline": 4, "onu_total": 12},
	}
	out := OverlayPartialPonSnapshot(prev, cur, "onu_counts")
	if out[0]["onu_online"] != 8 {
		t.Fatalf("onu_online=%v", out[0]["onu_online"])
	}
	if out[0]["rx_dbm"] != -18.5 {
		t.Fatalf("rx_dbm lost: %v", out[0]["rx_dbm"])
	}
}
