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
	if f, ok := parseOpticalDbm("-23.5", 0); !ok || f != -23.5 {
		t.Fatalf("float got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm("INTEGER: -2350", 0); !ok || f != -23.5 {
		t.Fatalf("centi got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm("INTEGER: 509", 100); !ok || f != 5.09 {
		t.Fatalf("divisor got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm("INTEGER: -23449", 1); !ok || f < -23.45 || f > -23.44 {
		t.Fatalf("zte milli divisor=1 got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm(`STRING: "-15.60"`, 0); !ok || f != -15.6 {
		t.Fatalf("vsol string got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm("2d:31:35:2e:36:30", 0); !ok || f != -15.6 {
		t.Fatalf("vsol hex got %v ok=%v", f, ok)
	}
	if f, ok := parseOpticalDbm("34.80", 0); !ok || isPlausibleOnuRxDbm(f) {
		t.Fatalf("temp 34.80 must not be plausible RX, got %v ok=%v", f, ok)
	}
}

func TestParseOnuRxDbm_vsol(t *testing.T) {
	dbm, display, ok := parseOnuRxDbm(`STRING: "-15.60"`, 0)
	if !ok || dbm != -15.6 || display != "-15.60" {
		t.Fatalf("got dbm=%v display=%q ok=%v", dbm, display, ok)
	}
	_, display, ok = parseOnuRxDbm("32.00", 0)
	if ok {
		t.Fatalf("expected reject temp-like value, display=%q", display)
	}
}

func TestStatusFromRxPower_zteScaled(t *testing.T) {
	raw := -23449
	dbm, ok := parseOpticalDbm("-23449", 1)
	if !ok {
		t.Fatal("parse failed")
	}
	if !StatusFromRxPower(raw, dbm, true, -50) {
		t.Fatalf("expected online at %.2f dBm vs threshold -50", dbm)
	}
}

func TestDecodeColonHexASCII_vsolRx(t *testing.T) {
	got, ok := decodeColonHexASCII("2d:31:39:2e:35:31")
	if !ok || got != "-19.51" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestDecodeColonHexASCII_macUnchanged(t *testing.T) {
	_, ok := decodeColonHexASCII("aa:bb:cc:dd:ee:ff")
	if ok {
		t.Fatal("expected MAC-like hex to stay undecoded")
	}
}

func TestNormalizeSnmpDisplayValue(t *testing.T) {
	if normalizeSnmpDisplayValue("2d:31:39:2e:35:31") != "-19.51" {
		t.Fatalf("normalize failed")
	}
	if normalizeSnmpDisplayValue("-23.5") != "-23.5" {
		t.Fatalf("plain float changed")
	}
}

func TestParseScaledFloat(t *testing.T) {
	if f, ok := parseScaledFloat("INTEGER: 512", 0); !ok || f != 512 {
		t.Fatalf("raw got %v ok=%v", f, ok)
	}
	if f, ok := parseScaledFloat("INTEGER: 512", 100); !ok || f != 5.12 {
		t.Fatalf("scaled got %v ok=%v", f, ok)
	}
}

func TestMergePonOpticalIntoPons(t *testing.T) {
	pons := []map[string]any{{"id": "01", "name": "GPON0/01", "onu_total": 2}}
	optical := map[int]map[string]any{1: {"rx_dbm": -24.1, "tx_dbm": 3.2, "voltage": 3.3, "current": 0.42, "temperature": 38.5}}
	out := mergePonOpticalIntoPons(pons, optical, nil)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	if out[0]["rx_dbm"].(float64) != -24.1 || out[0]["tx_dbm"].(float64) != 3.2 {
		t.Fatalf("optical merge %+v", out[0])
	}
	if out[0]["voltage"].(float64) != 3.3 || out[0]["current"].(float64) != 0.42 || out[0]["temperature"].(float64) != 38.5 {
		t.Fatalf("electrical merge %+v", out[0])
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

func TestParsePonOnuSuffix_vsolPhasePonSlot(t *testing.T) {
	base := "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1"
	full := base + ".5"
	pon, onu, ok := ParsePonOnuSuffix(base, full)
	if !ok || pon != 1 || onu != 5 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestParsePonOnuSuffixMapped_vsolOnuPonIndex(t *testing.T) {
	base := "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.2"
	full := base + ".5"
	idx := map[int]int{5: 1}
	pon, onu, _, ok := ParsePonOnuSuffixMapped(base, full, nil, idx)
	if !ok || pon != 1 || onu != 5 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestParsePonFromSuffix_datacomLaneIndex(t *testing.T) {
	base := "1.3.6.1.4.1.3709.3.6.8.2.1.1.3"
	full := base + ".101744641.1"
	pon, ok := ParsePonFromSuffix(base, full, nil)
	if !ok || pon != 1 {
		t.Fatalf("expected pon 1, got %d (ok=%v)", pon, ok)
	}
}

func TestParsePonOnuSuffixMapped_zteIfIndex(t *testing.T) {
	m := map[int]ponIfRef{
		285278465: {PonPort: 1, Compact: "1/1/1", Name: "PON-1/1/1", IfIndex: 285278465},
	}
	base := "1.3.6.1.4.1.3902.1082.500.1.2.4.2.1.2"
	full := base + ".285278465.12"
	pon, onu, ifIdx, ok := ParsePonOnuSuffixMapped(base, full, m, nil)
	if !ok || pon != 1 || onu != 12 || ifIdx != 285278465 {
		t.Fatalf("got pon=%d onu=%d if=%d ok=%v", pon, onu, ifIdx, ok)
	}
}

func TestStatusFromRxPower(t *testing.T) {
	th := -70.0
	if !StatusFromRxPower(0, -25.5, true, th) {
		t.Fatal("expected online above threshold")
	}
	if !StatusFromRxPower(0, -70, true, th) {
		t.Fatal("expected online at threshold (>=)")
	}
	if StatusFromRxPower(0, -80, true, th) {
		t.Fatal("expected offline at -80")
	}
	if StatusFromRxPower(0, -70.1, true, th) {
		t.Fatal("expected offline below threshold")
	}
	if StatusFromRxPower(-80000, -80, true, th) {
		t.Fatal("expected offline for ZTE raw -80000")
	}
	if StatusFromRxPower(65535000, 0, false, th) {
		t.Fatal("expected offline for invalid raw")
	}
	if StatusFromRxPower(0, 0, true, th) {
		t.Fatal("expected offline for 0.00 dBm (no signal)")
	}
}

func TestShouldApplyStatusFromRx(t *testing.T) {
	phase := OnuMetricsConfig{
		MetricStatus: {Enabled: true, StatusMode: StatusModePonOnuSuffix, OnlineValues: []int{3}},
		MetricRxPower: {Enabled: true, OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7"},
	}
	if shouldApplyStatusFromRx(phase) {
		t.Fatal("phase mode must not apply RX override")
	}
	rxMode := OnuMetricsConfig{
		MetricStatus: {Enabled: true, StatusMode: StatusModeRxPowerThreshold, OfflineRxDbm: -70},
		MetricRxPower: {Enabled: true, OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7"},
	}
	if !shouldApplyStatusFromRx(rxMode) {
		t.Fatal("rx threshold mode should apply RX override")
	}
}

func TestParsePonOnuSuffix_vsolPhaseAllPons(t *testing.T) {
	base := "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5"
	full := base + ".2.5"
	pon, onu, ok := ParsePonOnuSuffix(base, full)
	if !ok || pon != 2 || onu != 5 {
		t.Fatalf("got %d.%d ok=%v", pon, onu, ok)
	}
}

func TestStatusIsOnlineIfOper(t *testing.T) {
	def := OnuMetricDef{OnlineValues: []int{1}}
	if !StatusIsOnlineIfOper(1, def) || StatusIsOnlineIfOper(2, def) {
		t.Fatal("ifOper mapping failed")
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

func TestParsePonPortFromIfLabels(t *testing.T) {
	pon, compact, ok := parsePonPortFromIfLabels("STRING: gpon_olt-1/1/4", "PON-1/1/4")
	if !ok || pon != 4 || compact != "1/1/4" {
		t.Fatalf("got %d %q ok=%v", pon, compact, ok)
	}
	_, _, ok = parsePonPortFromIfLabels("xgei-1/4/1", "xgei-1/4/1")
	if ok {
		t.Fatal("xgei should not be classified as PON")
	}
}

func TestParsePonFromSuffix_unknownIfIndexWithMap(t *testing.T) {
	m := map[int]ponIfRef{285278465: {PonPort: 1, Compact: "1/1/1"}}
	_, ok := ParsePonFromSuffix("1.3.6.1.2.1.2.2.1.8", "1.3.6.1.2.1.2.2.1.8.285279233", m)
	if ok {
		t.Fatal("unknown uplink ifIndex should not map to a PON")
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
	_, _, _, err := CollectOnuMetrics(nil, "1.1.1.1", "public", nil, 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}
