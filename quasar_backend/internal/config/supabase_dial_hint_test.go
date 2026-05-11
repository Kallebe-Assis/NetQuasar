package config

import "testing"

func TestIsSupabaseDirectDBHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"db.abc.supabase.co", true},
		{"DB.XYZ.SUPABASE.CO", true},
		{"aws-0-us-east-1.pooler.supabase.com", false},
		{"db.abc.supabase.co.evil.com", false},
		{"", false},
		{"supabase.co", false},
	}
	for _, tc := range cases {
		if got := IsSupabaseDirectDBHost(tc.host); got != tc.want {
			t.Errorf("IsSupabaseDirectDBHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestLooksLikeTCPDialFailure(t *testing.T) {
	if !looksLikeTCPDialFailure(`dial tcp [2600:1f1e::1]:5432: network is unreachable`) {
		t.Fatal("expected dial tcp")
	}
	if looksLikeTCPDialFailure("password authentication failed") {
		t.Fatal("auth error should not match")
	}
}

func TestLooksLikeIPv6EgressBlocked(t *testing.T) {
	if !looksLikeIPv6EgressBlocked("dial tcp [2600::1]:5432: connect: network is unreachable") {
		t.Fatal("unreachable")
	}
	if !looksLikeIPv6EgressBlocked("dial tcp [2a00::1]:5432: i/o timeout") {
		t.Fatal("timeout with dial")
	}
	if looksLikeIPv6EgressBlocked("dial tcp 10.0.0.1:5432: i/o timeout") {
		t.Fatal("timeout em IPv4 não deve ser classificado como bloqueio IPv6")
	}
}
