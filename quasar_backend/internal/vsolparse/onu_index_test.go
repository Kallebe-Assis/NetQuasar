package vsolparse

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

func TestOidOnuField_matchesSnmpget(t *testing.T) {
	got := OidOnuField("2.1.6", 4, 10)
	want := "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6.4.10"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = OidOnuField("3.1.7", 2, 1)
	want = "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.2.1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOnuRefsFromIfRows(t *testing.T) {
	rows := []snmpifparse.IfRow{
		{IfName: "GPON04ONU10"},
		{IfName: "GPON02ONU1"},
		{IfName: "GPON04ONU10"},
	}
	refs := OnuRefsFromIfRows(rows)
	if len(refs) != 2 {
		t.Fatalf("len %d want 2", len(refs))
	}
	if refs[0].Pon != 2 || refs[0].Onu != 1 {
		t.Fatalf("first %+v", refs[0])
	}
	if refs[1].Pon != 4 || refs[1].Onu != 10 {
		t.Fatalf("second %+v", refs[1])
	}
}
