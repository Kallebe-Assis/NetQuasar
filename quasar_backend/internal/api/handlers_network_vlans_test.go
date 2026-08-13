package api

import "testing"

func TestNormalizeVLANKind(t *testing.T) {
	if normalizeVLANKind("PPPoE") != "pppoe" {
		t.Fatal("pppoe")
	}
	if normalizeVLANKind("Gerência") != "gerencia" {
		t.Fatal("gerencia")
	}
	if normalizeVLANKind("transporte") != "transporte" {
		t.Fatal("transporte")
	}
}

func TestNormalizeCatalogVLANID(t *testing.T) {
	v, msg := normalizeCatalogVLANID("vlan100")
	if v != "100" || msg != "" {
		t.Fatalf("%q %q", v, msg)
	}
	if _, msg := normalizeCatalogVLANID("0"); msg == "" {
		t.Fatal("vlan 0")
	}
	if _, msg := normalizeCatalogVLANID("5000"); msg == "" {
		t.Fatal("out of range")
	}
}
