package switchcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestParsePortVlanMap_accessAndTrunk(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.9.9.68.1.2.2.1.1.436297728", Value: "1"},
		{OID: "1.3.6.1.4.1.9.9.68.1.2.2.1.2.436297728", Value: "740"},
		{OID: "1.3.6.1.4.1.9.9.68.1.2.2.1.1.436207616", Value: "3"},
		{OID: "1.3.6.1.4.1.9.9.68.1.2.2.1.4.436207616", Value: "00 00 08 00 00 00 00 00 00 00 00 00 00 00 00 00"},
	}
	m := ParsePortVlanMap(vars)
	acc, ok := m[436297728]
	if !ok || acc.Mode != "access" || len(acc.VlanIDs) != 1 || acc.VlanIDs[0] != 740 {
		t.Fatalf("access port: %+v ok=%v", acc, ok)
	}
	tr, ok := m[436207616]
	if !ok || tr.Mode != "trunk" || len(tr.VlanIDs) == 0 {
		t.Fatalf("trunk port: %+v ok=%v", tr, ok)
	}
}

func TestFormatVlanLabel_withName(t *testing.T) {
	lbl := FormatVlanLabel(PortVlanInfo{Mode: "access", VlanIDs: []int{740}}, map[int]string{740: "VLAN-740-EXAMPLE"})
	if lbl == "" || lbl == "740" {
		t.Fatalf("expected name in label, got %q", lbl)
	}
}
