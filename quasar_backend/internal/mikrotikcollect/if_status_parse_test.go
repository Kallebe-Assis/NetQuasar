package mikrotikcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestParseIFMibInterfaces(t *testing.T) {
	t.Parallel()
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.2.1.2.2.1.2.1", Value: "ether1"},
		{OID: "1.3.6.1.2.1.2.2.1.7.1", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.8.1", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.2.2", Value: "wlan1"},
		{OID: "1.3.6.1.2.1.2.2.1.7.2", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.8.2", Value: "2"},
	}
	rows := ParseIFMibInterfaces(vars)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].Name != "ether1" || rows[0].OperStatusLabel != "up" || rows[0].AdminStatusLabel != "up" {
		t.Fatalf("row0: %+v", rows[0])
	}
	if rows[1].Name != "wlan1" || rows[1].OperStatusLabel != "down" {
		t.Fatalf("row1: %+v", rows[1])
	}
}
