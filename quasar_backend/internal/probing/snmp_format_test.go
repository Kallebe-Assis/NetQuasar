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

func TestOctetStringToUTF8_sixCharInterfaceNameNotMAC(t *testing.T) {
	// MikroTik ifName «combo1» = 6 octetos ASCII; não deve virar 63:6f:6d:62:6f:31
	got := octetStringToUTF8([]byte("combo1"))
	if got != "combo1" {
		t.Fatalf("got %q want combo1", got)
	}
}

func TestTryDecodeColonHexASCII_interfaceName(t *testing.T) {
	got, ok := TryDecodeColonHexASCII("63:6f:6d:62:6f:31")
	if !ok || got != "combo1" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestTryDecodeColonHexASCII_realMACUnchanged(t *testing.T) {
	_, ok := TryDecodeColonHexASCII("64:d1:54:dc:97:22")
	if ok {
		t.Fatal("real MAC hex should not decode as ASCII label")
	}
}
