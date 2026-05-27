package vsolparse

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func refKey(pon, onu int) string {
	return strings.TrimSpace(OidOnuField(FieldOnline, pon, onu))
}

// OnlineStaByRef mapa refKey → valor 4.1.8 (só respostas SNMP válidas).
func OnlineStaByRef(refs []OnuRef, vars []probing.SNMPVar) map[string]int {
	want := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		want[refKey(r.Pon, r.Onu)] = struct{}{}
	}
	out := make(map[string]int, len(refs))
	for _, v := range vars {
		if !probing.SNMPValueUsable(v.Value) {
			continue
		}
		t, col, pon, onu, ok := parseSuffix(v.OID)
		if !ok || t != 4 || col != 8 {
			continue
		}
		k := refKey(pon, onu)
		if _, need := want[k]; !need {
			continue
		}
		st := intFromVal(strings.TrimSpace(v.Value))
		if st == fieldUnset {
			continue
		}
		out[k] = st
	}
	return out
}
