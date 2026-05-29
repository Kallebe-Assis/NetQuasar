package mikrotikcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestParsePPPoESessionsFromIFMib(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.2.1.2.2.1.2.10", Value: "ether1"},
		{OID: "1.3.6.1.2.1.2.2.1.2.20", Value: "<pppoe-cliente1>"},
		{OID: "1.3.6.1.2.1.2.2.1.8.20", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.10.20", Value: "12345"},
		{OID: "1.3.6.1.2.1.2.2.1.16.20", Value: "67890"},
	}
	rows := ParsePPPoESessionsFromIFMib(vars)
	if len(rows) != 1 {
		t.Fatalf("expected 1 pppoe row, got %d", len(rows))
	}
	if rows[0].Name != "<pppoe-cliente1>" || rows[0].OperStatusLabel != "up" {
		t.Fatalf("row: %+v", rows[0])
	}
	if rows[0].InOctets != 12345 || rows[0].OutOctets != 67890 {
		t.Fatalf("octets: in=%d out=%d", rows[0].InOctets, rows[0].OutOctets)
	}
}
