package oltifderive

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

func TestDeriveFromIfRows_countsONUByPon(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfIndex: 17, IfName: "GPON0/1", Descr: "", AdminStatus: 1, OperStatus: 1},
		{IfIndex: 26, IfName: "GPON01ONU1", Descr: "", AdminStatus: 1, OperStatus: 1},
		{IfIndex: 27, IfName: "GPON01ONU2", Descr: "x", AdminStatus: 1, OperStatus: 2},
		{IfIndex: 1, IfName: "GE0/1", Descr: "", AdminStatus: 1, OperStatus: 2},
	}
	pons, sum := DeriveFromIfRows(rows, map[int]snmpmikrotik.OpticalPower{})
	if len(pons) != 1 {
		t.Fatalf("pons %d", len(pons))
	}
	if sum["onu_total_if_mib"] != 2 {
		t.Fatalf("total %v", sum["onu_total_if_mib"])
	}
	if sum["onu_online_if_mib"] != 1 || sum["onu_offline_if_mib"] != 1 {
		t.Fatalf("on/off %v %v", sum["onu_online_if_mib"], sum["onu_offline_if_mib"])
	}
}

func TestDeriveFromIfRows_countsONUByPonZTE(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfIndex: 285278465, IfName: "PON-1/1/1", Descr: "", AdminStatus: 1, OperStatus: 1},
		{IfIndex: 101, IfName: "ONU-1/1/1:1", Descr: "", AdminStatus: 1, OperStatus: 1},
		{IfIndex: 102, IfName: "gpon-onu_1/1/1:2", Descr: "", AdminStatus: 1, OperStatus: 2},
	}
	pons, sum := DeriveFromIfRows(rows, map[int]snmpmikrotik.OpticalPower{})
	if len(pons) != 1 {
		t.Fatalf("pons %d", len(pons))
	}
	if pons[0]["id"] != "1/1/1" {
		t.Fatalf("pon id %v", pons[0]["id"])
	}
	if sum["onu_total_if_mib"] != 2 {
		t.Fatalf("total %v", sum["onu_total_if_mib"])
	}
	if sum["onu_online_if_mib"] != 1 || sum["onu_offline_if_mib"] != 1 {
		t.Fatalf("on/off %v %v", sum["onu_online_if_mib"], sum["onu_offline_if_mib"])
	}
}

func TestDeriveFromIfRows_ordersPonNaturally(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfIndex: 1, IfName: "PON-1/1/10", OperStatus: 1},
		{IfIndex: 2, IfName: "PON-1/1/2", OperStatus: 1},
		{IfIndex: 3, IfName: "PON-1/1/1", OperStatus: 1},
	}
	pons, _ := DeriveFromIfRows(rows, map[int]snmpmikrotik.OpticalPower{})
	if len(pons) != 3 {
		t.Fatalf("pons %d", len(pons))
	}
	got := []string{
		pons[0]["id"].(string),
		pons[1]["id"].(string),
		pons[2]["id"].(string),
	}
	want := []string{"1/1/1", "1/1/2", "1/1/10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordem pons inválida: got=%v want=%v", got, want)
		}
	}
}
