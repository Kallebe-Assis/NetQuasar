package oltcollect

import "testing"

const zteOnuStateSN = `ONU         SN report      ONU ID Auth configuration State              times
-------------------------------------------------------------------------------
1/1/8:1     SN(000062F88DD 45     SN(000062F88DD5)   working            40
            5)
1/1/8:2     SN(XGTC0710019 16     SN(XGTC07100192)   working            7
            2)
1/1/8:10    SN(ZTEGC4AB6EC 3      SN(ZTEGC4AB6EC6)   DyingGasp          10
            6)

1/1/8:25    SN(ZTEG206101C N/A    SN(ZTEG206101C5)   OffLine            0
            5)
`

func TestParsePonRefreshAuthConfigSN(t *testing.T) {
	got := ParsePonRefreshOutput(zteOnuStateSN)
	want := map[int]struct {
		sn     string
		online bool
	}{1: {"000062F88DD5", true}, 2: {"XGTC07100192", true}, 10: {"ZTEGC4AB6EC6", false}, 25: {"ZTEG206101C5", false}}
	if len(got) != len(want) {
		t.Fatalf("entradas: %d %+v", len(got), got)
	}
	for _, e := range got {
		w, ok := want[e.Onu]
		if !ok || e.Pon != 8 || e.Serial != w.sn || e.Online == nil || *e.Online != w.online {
			t.Errorf("ONU %d: %+v (esperava %+v)", e.Onu, e, w)
		}
	}
}
