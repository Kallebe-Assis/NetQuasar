package mikrotikcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestParseInterfaceOperStatus(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.2.1.2.2.1.8.1", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.8.2", Value: "2"},
		{OID: "1.3.6.1.2.1.2.2.1.8.5", Value: "5"},
	}
	rows := ParseInterfaceOperStatus(vars)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].IfIndex != 1 || rows[0].OperStatusLabel != "up" {
		t.Fatalf("row 0: %+v", rows[0])
	}
	if rows[1].OperStatusLabel != "down" {
		t.Fatalf("row 1 label: %s", rows[1].OperStatusLabel)
	}
	if rows[2].OperStatusLabel != "dormant" {
		t.Fatalf("row 2 label: %s", rows[2].OperStatusLabel)
	}
}

func TestParseInterfaceAdminStatus(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.2.1.2.2.1.7.3", Value: "1"},
		{OID: "1.3.6.1.2.1.2.2.1.7.4", Value: "2"},
	}
	rows := ParseInterfaceAdminStatus(vars)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].AdminStatusLabel != "up" || rows[1].AdminStatusLabel != "down" {
		t.Fatalf("labels: %+v", rows)
	}
}
