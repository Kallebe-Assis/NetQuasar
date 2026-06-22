package oltcollect

import "testing"

func TestSelectRotatingPonBatch_wraps(t *testing.T) {
	pons := []map[string]any{
		{"pon": 1}, {"pon": 2}, {"pon": 3}, {"pon": 4},
	}
	batch, next := selectRotatingPonBatch(pons, 2, 3)
	if len(batch) != 2 {
		t.Fatalf("batch len=%d", len(batch))
	}
	if ponIndexFromRowMap(batch[0]) != 4 || ponIndexFromRowMap(batch[1]) != 1 {
		t.Fatalf("batch=%v", batch)
	}
	if next != 1 {
		t.Fatalf("next=%d", next)
	}
}

func TestSelectRotatingPonBatch_allWhenBelowMax(t *testing.T) {
	pons := []map[string]any{{"pon": 1}, {"pon": 2}}
	batch, next := selectRotatingPonBatch(pons, 16, 5)
	if len(batch) != 2 || next != 0 {
		t.Fatalf("batch=%d next=%d", len(batch), next)
	}
}

func TestCarryForwardPonTelnetFromPrev(t *testing.T) {
	prev := []map[string]any{{
		"pon": 2, "pon_telnet_source": true,
		"temperature": "45.0", "tx_dbm": "-2.1", "pon_telnet_at": "2026-01-01T00:00:00Z",
	}}
	pons := []map[string]any{{"pon": 1}, {"pon": 2}, {"pon": 3}}
	refreshKeys := map[string]bool{"pon:1": true}
	carryForwardPonTelnetFromPrev(pons, prev, refreshKeys)
	if pons[1]["temperature"] != "45.0" {
		t.Fatalf("pon2 should carry forward, got %v", pons[1])
	}
	if _, ok := pons[0]["temperature"]; ok {
		t.Fatal("pon1 was refreshed, should not carry")
	}
}
