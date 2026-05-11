package snmpmikrotik

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

// Amostra real (snmpwalk) — índice mtxr ≠ ifIndex; col.2 nome, 9 TX, 10 RX.
func snmpVarsFromUserWalk() []probing.SNMPVar {
	return []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.2.1", Value: "sfpplus1"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.2.6", Value: "sfp5"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.2.11", Value: "sfp10"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.9.1", Value: "919"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.10.1", Value: "-823"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.9.11", Value: "-6027"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.10.11", Value: "-14045"},
	}
}

func TestOpticalPowerByIfIndex_UserSnmpwalk(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfIndex: 1, IfName: "sfp-sfpplus1", DisplayName: "sfp-sfpplus1", Descr: "sfp-sfpplus1"},
		{IfIndex: 2, IfName: "combo1", DisplayName: "combo1", Descr: "combo1"},
		{IfIndex: 6, IfName: "ether4", DisplayName: "ether4", Descr: "ether4"},
		{IfIndex: 11, IfName: "sfp-sfpplus10", DisplayName: "sfp-sfpplus10", Descr: "sfp-sfpplus10"},
	}
	m := OpticalPowerByIfIndex(rows, snmpVarsFromUserWalk())
	p1 := m[1]
	if p1.TxDBm == nil || *p1.TxDBm != 0.919 {
		t.Fatalf("if1 tx want 0.919 (919/1000) got %v", p1.TxDBm)
	}
	if p1.RxDBm == nil || *p1.RxDBm != -0.823 {
		t.Fatalf("if1 rx want -0.823 got %v", p1.RxDBm)
	}
	p11 := m[11]
	if p11.TxDBm == nil || *p11.TxDBm != -6.027 {
		t.Fatalf("if11 tx want -6.027 got %v", p11.TxDBm)
	}
	if p11.RxDBm == nil || *p11.RxDBm != -14.045 {
		t.Fatalf("if11 rx want -14.045 got %v", p11.RxDBm)
	}
	if _, exists := m[6]; exists {
		t.Fatalf("mtx row 6 (sfp5) must not map blindly to ifIndex 6 (ether4), got %+v", m[6])
	}
}

func TestOpticalPowerByIfIndex_Sfp5NotEther4(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.2.6", Value: "sfp5"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.9.6", Value: "1000"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.10.6", Value: "-2000"},
	}
	rows := []snmpifparse.IfRow{
		{IfIndex: 6, IfName: "ether4", DisplayName: "ether4", Descr: "ether4"},
		{IfIndex: 7, IfName: "sfp-sfpplus5", DisplayName: "sfp-sfpplus5", Descr: "sfp-sfpplus5"},
	}
	m := OpticalPowerByIfIndex(rows, vars)
	if _, ok := m[6]; ok {
		t.Fatal("must not attach SFP power to ether4 at ifIndex 6")
	}
	p7 := m[7]
	if p7.TxDBm == nil || *p7.TxDBm != 1.0 || p7.RxDBm == nil || *p7.RxDBm != -2.0 {
		t.Fatalf("expected tx=1 rx=-2 on if7, got %+v", p7)
	}
}

func TestParseMikrotikMilliDbm(t *testing.T) {
	v, ok := parseMikrotikMilliDbm("-495")
	if !ok || v != -0.495 {
		t.Fatalf("want -0.495 got %v ok=%v", v, ok)
	}
}

// mtxrInterfaceStatsName (…14.1.1.2.<n>) alinhado com mtxrOptical (…19.1.1.*.<n>) — mesmo n.
func TestOpticalPowerByIfIndex_InterfaceStatsName(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.14988.1.1.14.1.1.2.1", Value: "sfp-sfpplus1"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.9.1", Value: "919"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.10.1", Value: "-823"},
	}
	rows := []snmpifparse.IfRow{
		{IfIndex: 1, IfName: "sfp-sfpplus1", DisplayName: "sfp-sfpplus1", Descr: "sfp-sfpplus1"},
	}
	m := OpticalPowerByIfIndex(rows, vars)
	p := m[1]
	if p.TxDBm == nil || *p.TxDBm != 0.919 || p.RxDBm == nil || *p.RxDBm != -0.823 {
		t.Fatalf("expected tx/rx on if1 via stats name, got %+v", p)
	}
}

func TestOpticalPowerByIfIndex_ObjectIndexHint(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.1.3", Value: "7"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.2.3", Value: "weird"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.9.3", Value: "1000"},
	}
	rows := []snmpifparse.IfRow{
		{IfIndex: 7, IfName: "sfp-sfpplus7", DisplayName: "sfp-sfpplus7", Descr: "sfp-sfpplus7"},
	}
	m := OpticalPowerByIfIndex(rows, vars)
	if p, ok := m[7]; !ok || p.TxDBm == nil || *p.TxDBm != 1.0 {
		t.Fatalf("expected col1 mtxrOpticalIndex=7 to map to ifIndex 7, got %+v", m)
	}
}
