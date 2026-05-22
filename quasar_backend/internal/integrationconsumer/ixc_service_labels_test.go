package integrationconsumer

import "testing"

func TestFormatIXCOnline(t *testing.T) {
	tests := map[string]string{
		"S":  "Online",
		"N":  "Offline",
		"SS": "Sem status",
		"":   "",
		"x":  "x",
	}
	for in, want := range tests {
		if got := FormatIXCOnline(in); got != want {
			t.Errorf("FormatIXCOnline(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatIXCStatusInternet(t *testing.T) {
	tests := map[string]string{
		"A":  "Ativo",
		"D":  "Desativado",
		"CM": "Bloqueio Manual",
		"FA": "Financeiro em atraso",
	}
	for in, want := range tests {
		if got := FormatIXCStatusInternet(in); got != want {
			t.Errorf("FormatIXCStatusInternet(%q) = %q, want %q", in, got, want)
		}
	}
}
