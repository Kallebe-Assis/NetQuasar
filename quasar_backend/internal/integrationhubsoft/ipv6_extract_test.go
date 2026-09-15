package integrationhubsoft

import "testing"

func TestExtractIPv6Prefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "real status_txt payload",
			in:   "CONECTADO HÁ 0 MES(ES), 4 DIA(S), 22 HORA(S) e 21 MINUTO(S) - 45.235.87.49 - 2804:4df:4df:5e00::/56(45.235.87.124)",
			want: "2804:4df:4df:5e00::/56",
		},
		{
			name: "no ipv6 present",
			in:   "CONECTADO HÁ 0 MES(ES), 4 DIA(S), 22 HORA(S) e 21 MINUTO(S) - 45.235.87.49",
			want: "",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "shorter prefix, no trailing nas ip",
			in:   "algo 2804:4d68:300:283e::/64 algo",
			want: "2804:4d68:300:283e::/64",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractIPv6Prefix(c.in)
			if got != c.want {
				t.Fatalf("extractIPv6Prefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
