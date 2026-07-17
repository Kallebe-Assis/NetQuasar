package probing

import "testing"

func TestLooksLikeCLIPrompt(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"switch#", true},
		{"Nexus#", true},
		{"switch>", true},
		{"switch(config)#", true},
		{"switch(config-if)#", true},
		{"[admin@MikroTik] >", true},
		{"[admin@MikroTik] > ", true},

		// Eco do comando — NÃO cortar aqui (era o bug do RX/TX vazio).
		{"switch# show interface transceiver details", false},
		{"Nexus# show interface status", false},
		{"$ show interface transceiver details", false},

		// Conteúdo DOM / tabelas.
		{"Rx Power        0.48 dBm  +    0.99 dBm  -18.23 dBm", false},
		{"Note: ++  high-alarm; +  high-warning; --  low-alarm; -  low-warning", false},
		{"    transceiver is present", false},
		{"Ethernet1/12", false},
		{"", false},

		// Buffer multi-linha: só a última linha conta.
		{"switch# show interface transceiver details\nEthernet1/12\n    transceiver is present", false},
		{"Ethernet1/12\n    transceiver is present\nswitch#", true},
		{"Ethernet1/1\n    transceiver is not present\nNexus#", true},
	}
	for _, tc := range cases {
		if got := looksLikeCLIPrompt(tc.in); got != tc.want {
			t.Fatalf("looksLikeCLIPrompt(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}
