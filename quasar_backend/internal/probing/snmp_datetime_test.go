package probing

import "testing"

func TestFormatSNMPDateAndTime_EightBytes(t *testing.T) {
	b := []byte{0x07, 0xea, 0x06, 0x1c, 0x17, 0x38, 0x28, 0x00}
	got, ok := formatSNMPDateAndTime(b)
	if !ok {
		t.Fatal("expected ok")
	}
	if got != "2026-06-28 23:56:40" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatSNMPDateAndTime_FourBytesNotIPv4(t *testing.T) {
	b := []byte{0x07, 0xea, 0x06, 0x1d}
	got := octetStringToUTF8(b)
	if got == "7.234.6.29" {
		t.Fatalf("should not be misread as IPv4, got %q", got)
	}
	if got != "2026-06-29 00:00:00" {
		t.Fatalf("got %q", got)
	}
}
