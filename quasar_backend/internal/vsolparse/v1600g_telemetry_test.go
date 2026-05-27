package vsolparse

import (
	"fmt"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestFromSNMPWalk_telemetryWithoutOnlineFirst(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.2.1", Value: "-23.46"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6.4.10", Value: "125GV1.0"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.4.2.1", Value: "3.3"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5.2.1", Value: "SN123"},
	}
	_, _, rows := FromSNMPWalk(vars, false)
	byKey := map[string]map[string]any{}
	for _, r := range rows {
		byKey[fmt.Sprintf("%v.%v", r["pon"], r["onu"])] = r
	}
	if len(byKey) != 2 {
		t.Fatalf("rows %d", len(byKey))
	}
	r := byKey["2.1"]
	if r["rx_pwr"] != "-23.46" || r["voltage"] != "3.3" || r["serial"] != "SN123" {
		t.Fatalf("pon2 %+v", r)
	}
	r = byKey["4.10"]
	if r["model"] != "125GV1.0" {
		t.Fatalf("pon4 model %v", r["model"])
	}
}
