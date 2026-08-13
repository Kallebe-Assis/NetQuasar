package alertverify

import "testing"

func TestPonHintsFromMeta_stripsPrefixes(t *testing.T) {
	hints := ponHintsFromMeta(map[string]any{
		"pon": "04",
		"key": "pon_down:04",
	})
	if len(hints) == 0 {
		t.Fatal("empty hints")
	}
	found := false
	for _, h := range hints {
		if h == "04" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want 04 in %v", hints)
	}
}

func TestFindPonRow_matchesPaddedAlertToBareSnapshot(t *testing.T) {
	arr := []map[string]any{
		{"id": "4", "name": "GPON0/4", "status": "pon_up", "onu_online": 8.0},
	}
	p, ok := findPonRow(arr, map[string]any{"pon": "04", "key": "pon_down:04"})
	if !ok {
		t.Fatal("PON 04 should match snapshot id 4 / GPON0/4")
	}
	if p["status"] != "pon_up" {
		t.Fatalf("row %+v", p)
	}
}
