package vsolparse

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestFromSNMPWalk_mergesTables(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.8.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.3.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.4.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1.2", Value: "3", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.3.1.2", Value: "ONU-TEST", Type: "OctetString"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.4.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6.1.2", Value: "2.5", Type: "OctetString"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.1.2", Value: "-28.3", Type: "OctetString"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.17.1.2", Value: "ABC123", Type: "OctetString"},
	}
	sum, _, rows := FromSNMPWalk(vars, false)
	if sum["vsol_onu_count"] != 1 {
		t.Fatalf("vsol_onu_count=%v", sum["vsol_onu_count"])
	}
	if sum["vsol_onu_online"] != 1 {
		t.Fatalf("vsol_onu_online=%v", sum["vsol_onu_online"])
	}
	if rows[0]["phase_sta"] != "working" {
		t.Fatalf("phase %v", rows[0]["phase_sta"])
	}
	if rows[0]["online"] != true {
		t.Fatalf("online %v", rows[0]["online"])
	}
}

func TestFromSNMPWalk_sixPartOID(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.8.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.1.3.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.1.4.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.1.5.1.2", Value: "3", Type: "Integer"},
	}
	_, _, rows := FromSNMPWalk(vars, false)
	if len(rows) != 1 || rows[0]["phase_sta"] != "working" {
		t.Fatalf("six-part parse failed: %+v", rows)
	}
}

func TestFromSNMPWalk_loggingOmccOnline(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.8.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.4.1.2", Value: "1", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1.2", Value: "0", Type: "Integer"},
	}
	sum, _, rows := FromSNMPWalk(vars, false)
	if sum["vsol_onu_online"] != 1 {
		t.Fatalf("online %v", sum["vsol_onu_online"])
	}
	if rows[0]["online"] != true {
		t.Fatalf("row online %v", rows[0]["online"])
	}
}

func TestFromSNMPWalk_unsetDetailOpNotCountedOfflineAlone(t *testing.T) {
	// Só óptica (sem gOnuStaInfo): não deve inflar total nem marcar offline por detailOp=0 por defeito.
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6.1.2", Value: "1.0", Type: "OctetString"},
	}
	sum, pons, rows := FromSNMPWalk(vars, false)
	if len(rows) != 0 {
		t.Fatalf("optical-only must not create ONU rows, got %+v", rows)
	}
	if sum["vsol_onu_online"] != 0 || sum["vsol_onu_count"] != 0 {
		t.Fatalf("optical-only sum online=%v count=%v", sum["vsol_onu_online"], sum["vsol_onu_count"])
	}
	_ = pons
}

func TestReconcileSummaryFromPons(t *testing.T) {
	sum := map[string]any{}
	pons := []map[string]any{
		{"onu_total": 10, "onu_online": 7, "onu_offline": 3},
		{"onu_total": 5, "onu_online": 4, "onu_offline": 1},
	}
	ReconcileSummaryFromPons(sum, pons)
	if sum["vsol_onu_count"] != 15 || sum["vsol_onu_online"] != 11 {
		t.Fatalf("got %v / %v", sum["vsol_onu_count"], sum["vsol_onu_online"])
	}
	if sum["vsol_pon_count"] != 2 {
		t.Fatalf("pon count %v", sum["vsol_pon_count"])
	}
}


func TestFromSNMPWalk_fourPartSuffix(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1.2", Value: "3", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.6.1.2", Value: " 1.5 ", Type: "OctetString"},
	}
	vars = append(vars,
		probing.SNMPVar{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.8.1.2", Value: "1", Type: "Integer"},
	)
	_, _, rows := FromSNMPWalk(vars, false)
	if len(rows) != 1 || rows[0]["tx_pwr"] != "1.5" {
		t.Fatalf("rows %+v", rows)
	}
}
