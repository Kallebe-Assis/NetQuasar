package oltcollect

import (
	"encoding/json"
	"testing"
)

func TestCarryForwardOnuIdentity_fillsMissingSerialFromPrev(t *testing.T) {
	prev := []map[string]any{
		{"pon": 3, "onu": 7, "serial": "ITBSCF8F197A", "model": "F601"},
	}
	// Coleta atual: mesma ONU, agora offline e sem serial (a OLT parou de devolver por SNMP).
	summary := map[string]any{
		"vsol_onu_rows": []any{
			map[string]any{"pon": 3, "onu": 7, "online": false},
		},
	}
	n := CarryForwardOnuIdentity(summary, prev)
	if n != 1 {
		t.Fatalf("filled = %d, want 1", n)
	}
	row := OnuRowsFromSummary(summary)[0]
	if row["serial"] != "ITBSCF8F197A" {
		t.Fatalf("serial = %v, want carried ITBSCF8F197A", row["serial"])
	}
	if row["model"] != "F601" {
		t.Fatalf("model = %v, want carried F601", row["model"])
	}
	if row["serial_source"] != "carried_prev" {
		t.Fatalf("serial_source = %v, want carried_prev", row["serial_source"])
	}
}

func TestCarryForwardOnuIdentity_freshSerialWins(t *testing.T) {
	prev := []map[string]any{{"pon": 1, "onu": 1, "serial": "OLDSERIAL1234"}}
	summary := map[string]any{
		"vsol_onu_rows": []any{
			map[string]any{"pon": 1, "onu": 1, "serial": "NEWSERIAL5678", "online": true},
		},
	}
	if n := CarryForwardOnuIdentity(summary, prev); n != 0 {
		t.Fatalf("filled = %d, want 0 (fresh serial present)", n)
	}
	if OnuRowsFromSummary(summary)[0]["serial"] != "NEWSERIAL5678" {
		t.Fatalf("fresh serial was overwritten")
	}
}

func TestCarryForwardOnuIdentity_ignoresImplausiblePrevSerial(t *testing.T) {
	prev := []map[string]any{{"pon": 2, "onu": 2, "serial": "3"}} // código de fase, não serial
	summary := map[string]any{
		"vsol_onu_rows": []any{map[string]any{"pon": 2, "onu": 2, "online": false}},
	}
	if n := CarryForwardOnuIdentity(summary, prev); n != 0 {
		t.Fatalf("filled = %d, want 0 (prev serial implausible)", n)
	}
	if _, ok := OnuRowsFromSummary(summary)[0]["serial"]; ok {
		t.Fatalf("implausible serial should not be carried")
	}
}

func TestCarryForwardOnuIdentity_noPrevMatch(t *testing.T) {
	prev := []map[string]any{{"pon": 1, "onu": 1, "serial": "ITBSCF8F197A"}}
	summary := map[string]any{
		"vsol_onu_rows": []any{map[string]any{"pon": 5, "onu": 9, "online": false}},
	}
	if n := CarryForwardOnuIdentity(summary, prev); n != 0 {
		t.Fatalf("filled = %d, want 0 (no matching pon/onu)", n)
	}
}

func TestCarryForwardOnuIdentityJSON_roundTrip(t *testing.T) {
	prev := []byte(`{"vsol_onu_rows":[{"pon":3,"onu":7,"serial":"ITBSCF8F197A"}]}`)
	fresh := []byte(`{"vsol_onu_rows":[{"pon":3,"onu":7,"online":false}]}`)
	out := CarryForwardOnuIdentityJSON(fresh, prev)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if OnuRowsFromSummary(m)[0]["serial"] != "ITBSCF8F197A" {
		t.Fatalf("serial not carried through JSON path: %s", out)
	}
}
