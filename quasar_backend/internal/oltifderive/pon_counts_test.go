package oltifderive

import "testing"

func TestNormalizePonONUCounts_OnlineOfflineExceedTotal(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"onu_total": 340, "onu_online": 392, "onu_offline": 307,
	}
	NormalizePonONUCounts(m)
	if m["onu_total"].(int) != 340 {
		t.Fatalf("total %v", m["onu_total"])
	}
	if m["onu_online"].(int) != 340 {
		t.Fatalf("online capped %v", m["onu_online"])
	}
	if m["onu_offline"].(int) != 0 {
		t.Fatalf("offline %v", m["onu_offline"])
	}
}

func TestMergePonRowPair_NormalizesAfterMax(t *testing.T) {
	t.Parallel()
	out := mergePonRowPair(
		map[string]any{"onu_total": 40, "onu_online": 200, "onu_offline": 5},
		map[string]any{"onu_total": 340, "onu_online": 50, "onu_offline": 10},
	)
	if out["onu_total"].(int) != 340 {
		t.Fatalf("total %v", out["onu_total"])
	}
	on := out["onu_online"].(int)
	off := out["onu_offline"].(int)
	if on+off > 340 {
		t.Fatalf("incoherent on=%d off=%d", on, off)
	}
}
