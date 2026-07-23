package snmpifparse

import "testing"

func TestFormatAlertIfaceLabel(t *testing.T) {
	cases := []struct {
		name, alias, custom, want string
	}{
		{"ether1", "", "", "ether1"},
		{"ether1", "Uplink", "", "ether1 (Uplink)"},
		{"ether1", "Uplink", "POP-A", "ether1 (POP-A)"},
		{"ether1", "ether1", "", "ether1"},
		{"", "Uplink", "", "Uplink"},
		{"", "", "Custom", "Custom"},
	}
	for _, c := range cases {
		got := FormatAlertIfaceLabel(c.name, c.alias, c.custom)
		if got != c.want {
			t.Fatalf("FormatAlertIfaceLabel(%q,%q,%q)=%q want %q", c.name, c.alias, c.custom, got, c.want)
		}
	}
}
