package bngcollect

import "testing"

func TestNormalizePPPoELogin(t *testing.T) {
	sfx := "@g2.com.br"
	if got := NormalizePPPoELogin("marciabarreto@g2.com.br", sfx); got != "marciabarreto" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizePPPoELogin("marciabarreto", sfx); got != "marciabarreto" {
		t.Fatalf("got %q", got)
	}
}

func TestMatchPPPoELogin(t *testing.T) {
	sfx := "@g2.com.br"
	if !MatchPPPoELogin("marciabarreto", "marciabarreto@g2.com.br", sfx) {
		t.Fatal("expected match")
	}
	if MatchPPPoELogin("bar", "marciabarreto", sfx) {
		t.Fatal("substring must not match")
	}
}

func TestNormalizeSNMPLoginValue_HexASCII(t *testing.T) {
	hexLogin := "6970:6570:616d:4067:322e:636f:6d2e:6272"
	got := NormalizeSNMPLoginValue(hexLogin, "@g2.com.br")
	if got != "ipepam" {
		t.Fatalf("got %q want ipepam", got)
	}
	hexLogin2 := "69:70:65:70:61:6d:40:67:32:2e:63:6f:6d:2e:62:72"
	got2 := NormalizeSNMPLoginValue(hexLogin2, "@g2.com.br")
	if got2 != "ipepam" {
		t.Fatalf("got2 %q want ipepam", got2)
	}
}
