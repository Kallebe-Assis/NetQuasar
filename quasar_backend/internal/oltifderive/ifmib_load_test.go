package oltifderive

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

func TestMergeIfRowSets_unionsPons(t *testing.T) {
	live := []snmpifparse.IfRow{
		{IfIndex: 10, IfName: "GPON01ONU1", OperStatus: 1},
		{IfIndex: 11, IfName: "GPON02ONU1", OperStatus: 1},
	}
	snap := []snmpifparse.IfRow{
		{IfIndex: 10, IfName: "GPON01ONU1", OperStatus: 0},
		{IfIndex: 20, IfName: "GPON05ONU1", OperStatus: 1},
		{IfIndex: 21, IfName: "GPON06ONU1", OperStatus: 1},
	}
	merged := MergeIfRowSets(live, snap)
	if len(merged) != 4 {
		t.Fatalf("ifaces %d want 4", len(merged))
	}
	if CountPonWithOnuFromRows(merged) != 4 {
		t.Fatalf("pons %d", CountPonWithOnuFromRows(merged))
	}
	byIdx := map[int]int{}
	for _, r := range merged {
		byIdx[r.IfIndex] = r.OperStatus
	}
	if byIdx[10] != 1 {
		t.Fatalf("oper 10=%d want 1 from live", byIdx[10])
	}
}

func TestCountPonWithOnuFromRows(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfName: "GPON01ONU1"},
		{IfName: "GPON01ONU2"},
		{IfName: "GPON08ONU3"},
	}
	if n := CountPonWithOnuFromRows(rows); n != 2 {
		t.Fatalf("got %d", n)
	}
}
