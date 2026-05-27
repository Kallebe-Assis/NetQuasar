package vsolparse

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestBuildOnuTable_oneRowPerRef_partialOnline(t *testing.T) {
	refs := []OnuRef{{Pon: 1, Onu: 1}, {Pon: 1, Onu: 2}}
	vars := []probing.SNMPVar{
		{OID: oidBase + ".4.1.8.1.1", Value: "1"},
	}
	prev := []map[string]any{
		{"pon": 1, "onu": 2, "online": true, "onu_online_sta": 1, "model": "X"},
	}
	rows := BuildOnuTable(refs, vars, prev, true)
	if len(rows) != 2 {
		t.Fatalf("rows %d want 2", len(rows))
	}
	if rows[0]["online"] != true {
		t.Fatalf("onu1 online %v", rows[0]["online"])
	}
	if rows[1]["online"] == true {
		t.Fatalf("onu2 must not stay online without 4.1.8, got %v", rows[1])
	}
	if rows[1]["model"] != "X" {
		t.Fatalf("model merged %v", rows[1]["model"])
	}
}
