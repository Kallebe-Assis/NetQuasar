package mikrotikcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestPPPoESessionsFromIfTableUsesIfName(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.2.1.2.2.1.2.10", Value: "ether1"},
		{OID: "1.3.6.1.2.1.2.2.1.8.20", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.10.20", Value: "12345"},
		{OID: "1.3.6.1.2.1.2.2.1.16.20", Value: "67890"},
		// Nome só no ifXTable (RouterOS com ifDescr fora do walk do perfil).
		{OID: "1.3.6.1.2.1.31.1.1.1.1.20", Value: "<pppoe-cliente1>"},
	}
	rows := PPPoESessionsFromIfTable(vars)
	if len(rows) != 1 {
		t.Fatalf("expected 1 pppoe row, got %d", len(rows))
	}
	if rows[0].IfIndex != 20 || rows[0].Name != "<pppoe-cliente1>" || rows[0].OperStatusLabel != "up" {
		t.Fatalf("row: %+v", rows[0])
	}
	if rows[0].InOctets != 12345 || rows[0].OutOctets != 67890 {
		t.Fatalf("octets: in=%d out=%d", rows[0].InOctets, rows[0].OutOctets)
	}
}

func TestIfTableWalkHasNames(t *testing.T) {
	statusOnly := []probing.SNMPVar{{OID: "1.3.6.1.2.1.2.2.1.8.20", Value: "1"}}
	if ifTableWalkHasNames(statusOnly) {
		t.Fatal("walk só com ifOperStatus não deve contar como tendo nomes")
	}
	withDescr := append(statusOnly, probing.SNMPVar{OID: "1.3.6.1.2.1.2.2.1.2.20", Value: "ether1"})
	if !ifTableWalkHasNames(withDescr) {
		t.Fatal("walk com ifDescr deve contar como tendo nomes")
	}
}
