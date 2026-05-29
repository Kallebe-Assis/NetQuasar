package oltifderive

import "testing"

func TestFilterPonRowsByMaxSlots_zteCompact(t *testing.T) {
	pons := []map[string]any{
		{"id": "1/1/16", "pon_compact": "1/1/16"},
		{"id": "1/1/17", "pon_compact": "1/1/17"},
		{"id": "285279233", "if_index": 285279233},
	}
	out := FilterPonRowsByMaxSlots(pons, 16)
	if len(out) != 1 {
		t.Fatalf("want 1 row, got %d", len(out))
	}
	if PonPortNumberFromRow(out[0]) != 16 {
		t.Fatalf("got slot %d", PonPortNumberFromRow(out[0]))
	}
}
