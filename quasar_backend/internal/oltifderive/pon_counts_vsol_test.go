package oltifderive

import "testing"

func TestNormalizePonONUCounts_vsol48_noInventOffline(t *testing.T) {
	m := map[string]any{
		"onu_total": 99, "onu_online": 0, "onu_offline": 0,
		"online_source": "vsol_4.1.8",
	}
	NormalizePonONUCounts(m)
	if m["onu_offline"] != 0 {
		t.Fatalf("offline %v want 0 not tot-on", m["onu_offline"])
	}
	if m["onu_no_status"] != 99 {
		t.Fatalf("no_status %v want 99", m["onu_no_status"])
	}
}
