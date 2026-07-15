package oltcollect

import "testing"

func TestMaxOnuIDFromEntries(t *testing.T) {
	entries := []SerialSearchOnuEntry{
		{Pon: 1, Onu: 3},
		{Pon: 1, Onu: 12},
		{Pon: 2, Onu: 40},
	}
	if got := MaxOnuIDFromEntries(entries, 1); got != 12 {
		t.Fatalf("got %d", got)
	}
	if got := MaxOnuIDFromEntries(entries, 2); got != 40 {
		t.Fatalf("got %d", got)
	}
}

func TestMaxOnuIDFromSnapshotSummary(t *testing.T) {
	sum := []byte(`{"vsol_onu_rows":[{"pon":4,"onu":7},{"pon":4,"onu":2},{"pon":1,"onu":99}]}`)
	if got := MaxOnuIDFromSnapshotSummary(sum, 4); got != 7 {
		t.Fatalf("got %d", got)
	}
}

func TestFirstAvailableOnuID_fillsGap(t *testing.T) {
	used := map[int]struct{}{
		1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}, 8: {}, 9: {}, 10: {},
	}
	if got := FirstAvailableOnuID(used); got != 7 {
		t.Fatalf("got %d want 7", got)
	}
}

func TestFirstAvailableOnuID_empty(t *testing.T) {
	if got := FirstAvailableOnuID(nil); got != 1 {
		t.Fatalf("got %d", got)
	}
	if got := FirstAvailableOnuID(map[int]struct{}{}); got != 1 {
		t.Fatalf("got %d", got)
	}
}

func TestFirstAvailableOnuID_contiguous(t *testing.T) {
	used := map[int]struct{}{1: {}, 2: {}, 3: {}}
	if got := FirstAvailableOnuID(used); got != 4 {
		t.Fatalf("got %d want 4", got)
	}
}

func TestCollectOnuIDsFromEntries(t *testing.T) {
	entries := []SerialSearchOnuEntry{
		{Pon: 9, Onu: 1},
		{Pon: 9, Onu: 3},
		{Pon: 1, Onu: 2},
	}
	got := CollectOnuIDsFromEntries(entries, 9)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if _, ok := got[1]; !ok {
		t.Fatal("missing 1")
	}
	if _, ok := got[3]; !ok {
		t.Fatal("missing 3")
	}
}

func TestPonOnuListCommandVSOL(t *testing.T) {
	if got := PonOnuListCommand("VSOL", OnuReportConfig{}); got != "show onu info {pon}" {
		t.Fatalf("got %q", got)
	}
}
