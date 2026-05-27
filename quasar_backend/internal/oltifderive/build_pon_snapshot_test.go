package oltifderive

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

func TestBuildPonSnapshotFromIfMIB_phyDoesNotZeroCounts(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfIndex: 1, IfName: "GPON0/1", OperStatus: 2},
		{IfIndex: 2, IfName: "GPON0/5", OperStatus: 2},
		{IfName: "GPON01ONU1", OperStatus: 1},
		{IfName: "GPON01ONU2", OperStatus: 1},
		{IfName: "GPON05ONU1", OperStatus: 1},
		{IfName: "GPON05ONU2", OperStatus: 1},
		{IfName: "GPON05ONU3", OperStatus: 2},
	}
	out := BuildPonSnapshotFromIfMIB(rows, nil)
	byID := map[string]map[string]any{}
	for _, r := range out {
		byID[r["id"].(string)] = r
	}
	p1 := byID["01"]
	if p1 == nil || rowPickInt(p1, "onu_total") != 2 {
		t.Fatalf("pon 01: %+v", p1)
	}
	if p1["status"] != "if_mib_onu" {
		t.Fatalf("pon 01 status %v", p1["status"])
	}
	p5 := byID["05"]
	if p5 == nil || rowPickInt(p5, "onu_total") != 3 {
		t.Fatalf("pon 05: %+v", p5)
	}
	if p5["status"] != "if_mib_onu" {
		t.Fatalf("pon 05 status %v want if_mib_onu", p5["status"])
	}
}

func TestMergePonRows_ifMibOnuCountNotZeroedByPonPort(t *testing.T) {
	counts := []map[string]any{
		{"id": "05", "name": "GPON0/5", "onu_total": 57, "onu_online": 50, "onu_offline": 7, "status": "if_mib_onu", "source_slice": "if_mib_onu_count"},
	}
	phy := []map[string]any{
		{"id": "05", "name": "GPON0/5", "onu_total": 0, "onu_online": 0, "onu_offline": 0, "status": "pon_down", "source_slice": "if_mib_pon_port"},
	}
	out := MergePonRowsForIfaceRefresh(counts, phy)
	if len(out) != 1 {
		t.Fatalf("len %d", len(out))
	}
	if rowPickInt(out[0], "onu_total") != 57 {
		t.Fatalf("total %v", out[0]["onu_total"])
	}
	if out[0]["status"] != "if_mib_onu" {
		t.Fatalf("status %v", out[0]["status"])
	}
}
