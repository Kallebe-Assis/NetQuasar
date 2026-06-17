package snapshotwalk

import (
	"encoding/json"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// VarsFromJSON converte o JSON gravado em interface_snapshots para []SNMPVar.
func VarsFromJSON(raw []byte) []probing.SNMPVar {
	if len(raw) == 0 {
		return nil
	}
	var arr []struct {
		OID   string `json:"oid"`
		Value string `json:"value"`
		Type  string `json:"type"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	out := make([]probing.SNMPVar, 0, len(arr))
	for _, v := range arr {
		oid := strings.TrimSpace(v.OID)
		if strings.HasPrefix(oid, "__netquasar.") {
			continue
		}
		out = append(out, probing.SNMPVar{OID: v.OID, Value: v.Value, Type: v.Type})
	}
	return out
}

// Truncated indica se o walk SNMP foi truncado (meta __netquasar.walk).
func Truncated(raw []byte) bool {
	var arr []struct {
		OID   string `json:"oid"`
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return false
	}
	for _, v := range arr {
		if strings.TrimSpace(v.OID) == "__netquasar.walk" && strings.TrimSpace(v.Value) == "truncated" {
			return true
		}
	}
	return false
}
