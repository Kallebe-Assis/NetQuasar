package oltifderive

import (
	"fmt"
	"testing"
)

func TestCanonicalPonRowKey_vsol_matches_if_mib(t *testing.T) {
	vsol := map[string]any{"id": "1", "name": "PON 1", "status": "vsol_snmp"}
	if got := CanonicalPonRowKey(vsol); got != "01" {
		t.Fatalf("vsol row key %q want 01", got)
	}
	ifMib := map[string]any{"id": "01", "name": "GPON0/1", "status": "pon_up"}
	if got := CanonicalPonRowKey(ifMib); got != "01" {
		t.Fatalf("if row key %q want 01", got)
	}
}

func TestVsolMibPonCompactID(t *testing.T) {
	if VsolMibPonCompactID(1) != "01" {
		t.Fatalf("1 -> %q", VsolMibPonCompactID(1))
	}
	if VsolMibPonCompactID(10) != "010" {
		t.Fatalf("10 -> %q", VsolMibPonCompactID(10))
	}
}

func TestMergePonRowsForIfaceRefresh_dedupes_vsol_and_if(t *testing.T) {
	existing := []map[string]any{
		{"id": "1", "name": "PON 1", "onu_total": 4, "onu_online": 3, "onu_offline": 1, "status": "vsol_snmp", "tx_dbm": -2.5},
	}
	fromIf := []map[string]any{
		{"id": "01", "name": "GPON0/1", "onu_total": 2, "onu_online": 1, "onu_offline": 1, "status": "pon_up", "source_slice": "if_mib_onu"},
	}
	out := MergePonRowsForIfaceRefresh(existing, fromIf)
	if len(out) != 1 {
		t.Fatalf("len %d want 1: %+v", len(out), out)
	}
	if fmt.Sprint(out[0]["id"]) != "01" {
		t.Fatalf("id %v want 01", out[0]["id"])
	}
	if rowPickInt(out[0], "onu_total") != 2 {
		t.Fatalf("onu_total %v", out[0]["onu_total"])
	}
	if out[0]["tx_dbm"] == nil {
		t.Fatal("lost tx_dbm from VSOL")
	}
}

func TestDedupePonMaps_snapshot_style_dup(t *testing.T) {
	rows := []map[string]any{
		{"id": "1", "name": "PON 1", "onu_total": 5, "onu_online": 4, "onu_offline": 1, "status": "vsol_snmp"},
		{"id": "01", "name": "GPON0/1", "onu_total": 5, "onu_online": 4, "onu_offline": 1, "status": "pon_up"},
	}
	out := DedupePonMaps(rows)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	if fmt.Sprint(out[0]["id"]) != "01" {
		t.Fatalf("id %v", out[0]["id"])
	}
	if rowPickInt(out[0], "onu_total") != 5 {
		t.Fatalf("total %v", out[0]["onu_total"])
	}
}
