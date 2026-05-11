package vsolparse

import (
	"fmt"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestFromSNMPWalk_mergesTables(t *testing.T) {
	// Sufixo após 1.3.6.1.4.1.37950.1.1.6.1.1. → T.1.col.pon.onu
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.3.1.2", Value: "1", Type: "Integer"}, // admin enable
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.4.1.2", Value: "1", Type: "Integer"}, // omcc enable
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1.2", Value: "3", Type: "Integer"}, // phase working
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.3.1.2", Value: "ONU-TEST", Type: "OctetString"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.4.1.2", Value: "1", Type: "Integer"}, // auth sn
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6.1.2", Value: "2.5", Type: "OctetString"}, // tx
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.1.2", Value: "-28.3", Type: "OctetString"}, // rx
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.17.1.2", Value: "ABC123", Type: "OctetString"}, // model
	}
	sum, pons, rows := FromSNMPWalk(vars)
	if sum["vsol_onu_count"] != 1 {
		t.Fatalf("vsol_onu_count=%v", sum["vsol_onu_count"])
	}
	if len(pons) != 1 {
		t.Fatalf("pons len %d", len(pons))
	}
	if len(rows) != 1 {
		t.Fatalf("rows len %d", len(rows))
	}
	if rows[0]["phase_sta"] != "working" {
		t.Fatalf("phase %v", rows[0]["phase_sta"])
	}
	if rows[0]["profile_name"] != "ONU-TEST" {
		t.Fatalf("profile %v", rows[0]["profile_name"])
	}
	if rows[0]["rx_pwr"] != "-28.3" {
		t.Fatalf("rx %v", rows[0]["rx_pwr"])
	}
	if rows[0]["model"] != "ABC123" {
		t.Fatalf("model %v", rows[0]["model"])
	}
	if len(pons) == 1 {
		if pons[0]["tx_dbm"] == nil {
			t.Fatal("expected tx_dbm on PON from ONU optical avg")
		}
		if g := fmt.Sprint(pons[0]["id"]); g != "01" {
			t.Fatalf("pon id %q want 01", g)
		}
		if g := fmt.Sprint(pons[0]["name"]); g != "GPON0/1" {
			t.Fatalf("pon name %q want GPON0/1", g)
		}
	}
}

func TestFromSNMPWalk_legacyOpticalOID(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1.2", Value: "3", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.5.12.2.1.8.1.6.1.2", Value: "2.1", Type: "OctetString"},
		{OID: "1.3.6.1.4.1.37950.1.1.5.12.2.1.8.1.7.1.2", Value: "-27,5 dBm", Type: "OctetString"},
	}
	_, pons, rows := FromSNMPWalk(vars)
	if len(rows) != 1 || rows[0]["tx_pwr"] != "2.1" {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0]["rx_pwr"] != "-27,5 dBm" {
		t.Fatalf("rx raw %+v", rows[0]["rx_pwr"])
	}
	if len(pons) != 1 || pons[0]["tx_dbm"] == nil {
		t.Fatalf("pons %+v", pons)
	}
}

func TestFromSNMPWalk_fourPartSuffix(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.1.2", Value: "3", Type: "Integer"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.6.1.2", Value: " 1.5 ", Type: "OctetString"},
	}
	_, pons, rows := FromSNMPWalk(vars)
	if len(rows) != 1 || rows[0]["tx_pwr"] != "1.5" {
		t.Fatalf("rows %+v", rows)
	}
	if len(pons) != 1 || pons[0]["tx_dbm"] == nil {
		t.Fatalf("pons %+v", pons)
	}
}
