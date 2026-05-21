package probing

import "testing"

func TestOctetStringToUTF8_macAddress(t *testing.T) {
	mac := []byte{0x64, 0xd1, 0x54, 0xdc, 0x97, 0x22}
	got := octetStringToUTF8(mac)
	want := "64:d1:54:dc:97:22"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOctetStringToUTF8_macWithLeadingZero(t *testing.T) {
	mac := []byte{0x00, 0x64, 0xd1, 0x54, 0xdc, 0x97, 0x22}
	got := octetStringToUTF8(mac)
	want := "64:d1:54:dc:97:22"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOctetStringToUTF8_printableASCII(t *testing.T) {
	got := octetStringToUTF8([]byte("RouterOS"))
	if got != "RouterOS" {
		t.Fatalf("got %q", got)
	}
}
