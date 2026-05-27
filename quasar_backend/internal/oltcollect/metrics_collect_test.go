package oltcollect

import "testing"

func TestParsePonOnuSuffix_modelTable(t *testing.T) {
	base := "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6"
	full := base + ".3.10"
	pon, onu, ok := ParsePonOnuSuffix(base, full)
	if !ok || pon != 3 || onu != 10 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestParseOpticalDbm(t *testing.T) {
	if f, ok := parseOpticalDbm("-23.5"); !ok || f != -23.5 {
		t.Fatalf("float got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm("INTEGER: -2350"); !ok || f != -23.5 {
		t.Fatalf("centi got %v ok=%v", f, ok)
	}
}

func TestMergePonOpticalIntoPons(t *testing.T) {
	pons := []map[string]any{{"id": "01", "name": "GPON0/01", "onu_total": 2}}
	optical := map[int]map[string]any{1: {"rx_dbm": -24.1, "tx_dbm": 3.2}}
	out := mergePonOpticalIntoPons(pons, optical)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	if out[0]["rx_dbm"].(float64) != -24.1 || out[0]["tx_dbm"].(float64) != 3.2 {
		t.Fatalf("optical merge %+v", out[0])
	}
}

func TestParsePonOnuSuffix_statusTable(t *testing.T) {
	base := "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.2"
	full := base + ".2.1"
	pon, onu, ok := ParsePonOnuSuffix(base, full)
	if !ok || pon != 2 || onu != 1 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestStatusIsOnline_userValues(t *testing.T) {
	def := OnuMetricDef{OnlineValues: []int{3}, OfflineValues: []int{4}}
	if !StatusIsOnline(3, def) || StatusIsOnline(4, def) {
		t.Fatalf("status mapping failed")
	}
}

func TestStatusIsOnline_offlineEmptyFallback(t *testing.T) {
	def := OnuMetricDef{OnlineValues: []int{1}, OfflineValues: nil}
	if !StatusIsOnline(1, def) {
		t.Fatal("status 1 should be online")
	}
	if StatusIsOnline(2, def) {
		t.Fatal("status 2 should be offline when offline_values is empty")
	}
	if StatusIsOnline(6, def) {
		t.Fatal("status 6 should be offline when offline_values is empty")
	}
}

func TestParsePonOnuFromIfDescr(t *testing.T) {
	pon, onu, ok := parsePonOnuFromIfDescr("GPON01ONU10")
	if !ok || pon != 1 || onu != 10 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestParsePonOnuFromIfDescr_withStringPrefix(t *testing.T) {
	pon, onu, ok := parsePonOnuFromIfDescr("STRING: GPON03ONU7")
	if !ok || pon != 3 || onu != 7 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestParsePonOnuFromIfDescr_fullSnmpLine(t *testing.T) {
	pon, onu, ok := parsePonOnuFromIfDescr("IF-MIB::ifDescr.26 = STRING: GPON01ONU2")
	if !ok || pon != 1 || onu != 2 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestParseStatusInt_variants(t *testing.T) {
	cases := []struct {
		in  string
		out int
	}{
		{"1", 1},
		{"INTEGER: 2", 2},
		{"up(1)", 1},
		{"down(2)", 2},
		{"testing(3)", 3},
		{"notPresent(6)", 6},
		{"IF-MIB::ifOperStatus.26 = INTEGER: down(2)", 2},
	}
	for _, tc := range cases {
		got, err := parseStatusInt(tc.in)
		if err != nil {
			t.Fatalf("input %q unexpected err: %v", tc.in, err)
		}
		if got != tc.out {
			t.Fatalf("input %q got=%d want=%d", tc.in, got, tc.out)
		}
	}
}

func TestCollectOnuMetrics_noConfig(t *testing.T) {
	_, _, _, err := CollectOnuMetrics(nil, "1.1.1.1", "public", nil, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}
