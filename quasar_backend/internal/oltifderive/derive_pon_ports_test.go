package oltifderive

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

func TestListPonPhysicalPortsFromIfRows(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfIndex: 1, IfName: "GPON0/1", OperStatus: 1},
		{IfIndex: 2, IfName: "GPON0/2", OperStatus: 1},
		{IfIndex: 3, IfName: "GPON0/8", OperStatus: 1},
		{IfIndex: 10, IfName: "GPON01ONU3", OperStatus: 1},
	}
	out := ListPonPhysicalPortsFromIfRows(rows, nil)
	if len(out) != 3 {
		t.Fatalf("want 3 pon ports got %d %+v", len(out), out)
	}
}

func TestCountOnuPerPonFromIfRows(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfName: "GPON01ONU1", OperStatus: 1},
		{IfName: "GPON01ONU2", OperStatus: 1},
		{IfName: "GPON01ONU3", OperStatus: 2},
		{IfName: "GPON02ONU1", OperStatus: 1},
	}
	out := CountOnuPerPonFromIfRows(rows)
	if len(out) != 2 {
		t.Fatalf("pons %d", len(out))
	}
	m := map[string]int{}
	for _, r := range out {
		m[r["id"].(string)] = r["onu_total"].(int)
	}
	if m["01"] != 3 || m["02"] != 1 {
		t.Fatalf("%v", m)
	}
}

func TestMergeVsolKeepsCountsAddsPon(t *testing.T) {
	vsol := []map[string]any{
		{"id": "01", "name": "GPON0/1", "onu_total": 40, "onu_online": 35, "onu_offline": 5, "status": "vsol_snmp"},
	}
	ifPon := []map[string]any{
		{"id": "01", "name": "GPON0/1", "onu_total": 2, "onu_online": 1, "onu_offline": 1, "status": "if_mib_pon_port"},
		{"id": "08", "name": "GPON0/8", "onu_total": 0, "onu_online": 0, "onu_offline": 0, "status": "if_mib_pon_port"},
	}
	merged := MergePonRowsForIfaceRefresh(vsol, ifPon)
	if len(merged) != 2 {
		t.Fatalf("len %d", len(merged))
	}
	for _, row := range merged {
		if row["id"] == "01" {
			if rowPickInt(row, "onu_online") != 35 {
				t.Fatalf("vsol counts overwritten: %+v", row)
			}
		}
	}
}
