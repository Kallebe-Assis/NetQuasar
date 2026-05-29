package mikrotikcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestParseOpticalPorts_sfp8(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.2.8", Value: "sfp8"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.6.8", Value: "53"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.7.8", Value: "3159"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.8.8", Value: "28"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.9.8", Value: "-5025"},
		{OID: "1.3.6.1.4.1.14988.1.1.19.1.1.10.8", Value: "-7637"},
	}
	profile := DefaultMetrics()
	ports := ParseOpticalPorts(vars, profile)
	if len(ports) != 1 {
		t.Fatalf("ports=%d want 1", len(ports))
	}
	p := ports[0]
	if p.Name != "sfp8" {
		t.Fatalf("name=%q", p.Name)
	}
	if p.TemperatureC == nil || *p.TemperatureC != 53 {
		t.Fatalf("temp=%v", p.TemperatureC)
	}
	if p.SupplyVoltageV == nil || *p.SupplyVoltageV < 3.158 || *p.SupplyVoltageV > 3.16 {
		t.Fatalf("volt=%v want ~3.159", p.SupplyVoltageV)
	}
	if p.BiasCurrentMA == nil || *p.BiasCurrentMA != 28 {
		t.Fatalf("bias=%v", p.BiasCurrentMA)
	}
	if p.TxDBm == nil || *p.TxDBm > -5.02 || *p.TxDBm < -5.03 {
		t.Fatalf("tx=%v want -5.025", p.TxDBm)
	}
	if p.RxDBm == nil || *p.RxDBm > -7.63 || *p.RxDBm < -7.64 {
		t.Fatalf("rx=%v want -7.637", p.RxDBm)
	}
}

func TestApplyDivisor_voltage(t *testing.T) {
	v := applyDivisor("237", 10)
	f, ok := v.(float64)
	if !ok || f < 23.69 || f > 23.71 {
		t.Fatalf("got %v want 23.7", v)
	}
}
