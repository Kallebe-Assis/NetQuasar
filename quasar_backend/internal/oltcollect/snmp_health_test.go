package oltcollect

import "testing"

func TestDeriveSnmpHealthFromSummary_failedCollect(t *testing.T) {
	st, reason := DeriveSnmpHealthFromSummary(map[string]any{
		"last_collect_ok":   false,
		"last_collect_error": "timeout SNMP",
	})
	if st != "failed" || reason != "timeout SNMP" {
		t.Fatalf("got %q / %q", st, reason)
	}
}

func TestDeriveSnmpHealthFromSummary_partial(t *testing.T) {
	st, _ := DeriveSnmpHealthFromSummary(map[string]any{
		"last_collect_ok":      true,
		"onu_metrics_incomplete": true,
	})
	if st != "partial" {
		t.Fatalf("got %q want partial", st)
	}
}

func TestDeriveSnmpHealthFromSummary_ok(t *testing.T) {
	st, reason := DeriveSnmpHealthFromSummary(map[string]any{
		"last_collect_ok": true,
	})
	if st != "ok" || reason != "" {
		t.Fatalf("got %q / %q", st, reason)
	}
}
