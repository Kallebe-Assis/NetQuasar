package oltifderive

import "testing"

func TestPonIdentityNorm(t *testing.T) {
	cases := []struct{ in, want string }{
		{"04", "4"},
		{"4", "4"},
		{"GPON0/4", "4"},
		{"gpon0/4", "4"},
		{"PON 4", "4"},
		{"pon_down:04", "pon_down:04"},
		{"1/1/04", "1/1/4"},
		{"010", "10"},
		{"GPON0/10", "10"},
	}
	for _, c := range cases {
		if got := PonIdentityNorm(c.in); got != c.want {
			t.Fatalf("PonIdentityNorm(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestPonKeysEqual(t *testing.T) {
	if !PonKeysEqual("04", "4") {
		t.Fatal("04 vs 4")
	}
	if !PonKeysEqual("04", "GPON0/4") {
		t.Fatal("04 vs GPON0/4")
	}
	if !PonKeysEqual("PON 4", "04") {
		t.Fatal("PON 4 vs 04")
	}
	if !PonKeysEqual("0/4", "04") {
		t.Fatal("0/4 vs 04")
	}
	if PonKeysEqual("04", "05") {
		t.Fatal("04 vs 05 should differ")
	}
	if PonKeysEqual("1/1/4", "1/2/4") {
		t.Fatal("1/1/4 vs 1/2/4 should differ")
	}
}

func TestFindPonRowByHints_paddedVsBare(t *testing.T) {
	arr := []map[string]any{
		{"id": "4", "name": "GPON0/4", "status": "pon_up", "onu_online": 12.0},
		{"id": "5", "name": "GPON0/5", "status": "pon_down"},
	}
	p, ok := FindPonRowByHints(arr, []string{"04", "pon_down:04"})
	if !ok {
		t.Fatal("expected match for 04")
	}
	if PonIdentityNorm(StablePonRowKey(p)) != "4" {
		t.Fatalf("got %+v", p)
	}
}

func TestFindPonRowByHints_uniqueLastSegment(t *testing.T) {
	arr := []map[string]any{
		{"id": "1/1/4", "name": "gpon-1/1/4", "status": "pon_up"},
	}
	p, ok := FindPonRowByHints(arr, []string{"04"})
	if !ok || p == nil {
		t.Fatal("expected unique last-segment match")
	}
}

func TestFindPonRowByHints_ambiguousLastSegment(t *testing.T) {
	arr := []map[string]any{
		{"id": "1/1/4", "name": "gpon-1/1/4"},
		{"id": "1/2/4", "name": "gpon-1/2/4"},
	}
	if _, ok := FindPonRowByHints(arr, []string{"04"}); ok {
		t.Fatal("ambiguous last segment must not match")
	}
}
