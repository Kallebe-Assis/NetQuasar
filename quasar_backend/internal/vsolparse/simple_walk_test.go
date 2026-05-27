package vsolparse

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestSimpleWalkCount_fromFixture(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.8.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.8.2.1", Value: "0", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5.1.2", Value: "SN-A", Type: "OctetString"},
	}
	out := normalizeWalkVarsForBase(vars, DefaultVSOLOnuWalkOID)
	sum, _, rows := FromSNMPWalk(out, false)
	if len(rows) < 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if intValAny(sum["vsol_onu_online"]) != 1 {
		t.Fatalf("online %v", sum["vsol_onu_online"])
	}
	pons := PonsFromOnuRows(rows)
	if len(pons) < 1 {
		t.Fatalf("pons %d", len(pons))
	}
}

func TestPonsFromOnuRows(t *testing.T) {
	rows := []map[string]any{
		{"pon": 1, "onu": 1, "online": true},
		{"pon": 1, "onu": 2, "online": false},
		{"pon": 2, "onu": 1, "online": true},
	}
	pons := PonsFromOnuRows(rows)
	if len(pons) != 2 {
		t.Fatalf("pons %d", len(pons))
	}
}
