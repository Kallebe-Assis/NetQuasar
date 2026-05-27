package vsolparse

import (
	"fmt"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const oidBase = "1.3.6.1.4.1.37950.1.1.6.1.1"

func TestParseSuffix_v1600PonOnu(t *testing.T) {
	cases := []struct {
		oid       string
		tbl, col  int
		pon, onu  int
	}{
		{oidBase + ".2.1.1.3.81", 2, 1, 3, 81},
		{oidBase + ".2.1.1.4.1", 2, 1, 4, 1},
		{oidBase + ".2.1.5.1.85", 2, 5, 1, 85},
		{oidBase + ".2.1.5.2.1", 2, 5, 2, 1},
		{oidBase + ".2.1.6.1.1", 2, 6, 1, 1},
		{oidBase + ".3.1.3.4.6", 3, 3, 4, 6},
		{oidBase + ".3.1.7.2.74", 3, 7, 2, 74},
		{oidBase + ".4.1.8.1.9", 4, 8, 1, 9},
		{oidBase + ".4.1.8.2.12", 4, 8, 2, 12},
	}
	for _, c := range cases {
		tbl, col, pon, onu, ok := parseSuffix(c.oid)
		if !ok || tbl != c.tbl || col != c.col || pon != c.pon || onu != c.onu {
			t.Fatalf("%s got tbl=%d col=%d pon=%d onu=%d ok=%v", c.oid, tbl, col, pon, onu, ok)
		}
	}
}

func TestFromSNMPWalk_onuOnlineFlag48(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: oidBase + ".4.1.8.1.1", Value: "1"},
		{OID: oidBase + ".4.1.8.1.9", Value: "0"},
		{OID: oidBase + ".4.1.8.2.12", Value: "0"},
		{OID: oidBase + ".4.1.8.2.13", Value: "1"},
	}
	on, off := OnlineOfflineByPon(vars)
	if on[1]+on[2] != 2 || off[1]+off[2] != 2 {
		t.Fatalf("on=%v off=%v", on, off)
	}
	_, _, rows := FromSNMPWalk(vars, false)
	by := map[string]bool{}
	for _, r := range rows {
		by[fmt.Sprintf("%v.%v", r["pon"], r["onu"])] = r["online"] == true
	}
	if by["1.9"] != false || by["2.12"] != false || by["1.1"] != true || by["2.13"] != true {
		t.Fatalf("online map %v", by)
	}
}

func TestFromSNMPWalk_v1600PhaseSerialModel(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: oidBase + ".2.1.1.3.81", Value: "3"},
		{OID: oidBase + ".4.1.8.3.81", Value: "1"},
		{OID: oidBase + ".2.1.1.4.1", Value: "6"},
		{OID: oidBase + ".2.1.5.1.85", Value: "MONU0027b3a1"},
		{OID: oidBase + ".2.1.5.2.1", Value: "HWTCb4de53ab"},
		{OID: oidBase + ".2.1.6.1.1", Value: "TX-6610"},
		{OID: oidBase + ".2.1.1.2.74", Value: "3"},
		{OID: oidBase + ".3.1.3.4.6", Value: "38.64"},
		{OID: oidBase + ".3.1.7.2.74", Value: "-27.70"},
	}
	_, _, rows := FromSNMPWalk(vars, false)
	byKey := map[string]map[string]any{}
	for _, r := range rows {
		byKey[fmt.Sprintf("%v.%v", r["pon"], r["onu"])] = r
	}
	if len(byKey) < 3 {
		t.Fatalf("rows %d keys %+v", len(rows), byKey)
	}
	r := byKey["3.81"]
	if r == nil || r["phase_sta"] != "working" || r["online"] != true {
		t.Fatalf("pon3 onu81: %+v", r)
	}
	if byKey["4.1"]["online"] != false {
		t.Fatalf("phase 6 offLine: %+v", byKey["4.1"])
	}
	if byKey["1.85"]["serial"] != "MONU0027b3a1" {
		t.Fatalf("sn: %+v", byKey["1.85"])
	}
	if byKey["1.1"]["model"] != "TX-6610" {
		t.Fatalf("model: %+v", byKey["1.1"])
	}
	if byKey["2.74"]["rx_pwr"] != "-27.70" {
		t.Fatalf("rx: %+v", byKey["2.74"])
	}
}

func TestFromSNMPWalk_opticalAloneNoPhantomOnu(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: oidBase + ".3.1.6.1.2", Value: "1.0"},
	}
	sum, _, rows := FromSNMPWalk(vars, false)
	if len(rows) != 0 {
		t.Fatalf("phantom rows %d", len(rows))
	}
	if intVal(sum["vsol_onu_online"]) != 0 || intVal(sum["vsol_onu_offline"]) != 0 {
		t.Fatalf("count %v", sum["vsol_onu_count"])
	}
}
