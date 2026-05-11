package api

import "testing"

func TestNormalizeBRPhone(t *testing.T) {
	got, err := normalizeBRPhone("(11) 98765-4321")
	if err != nil || got != "11987654321" {
		t.Fatalf("11 digit: got %q err %v", got, err)
	}
	got, err = normalizeBRPhone("85 3333-4444")
	if err != nil || got != "8533334444" {
		t.Fatalf("10 digit: got %q err %v", got, err)
	}
	if _, err := normalizeBRPhone("119876543"); err == nil {
		t.Fatal("short: want error")
	}
	if _, err := normalizeBRPhone("00999999999"); err == nil {
		t.Fatal("ddd 00: want error")
	}
}
