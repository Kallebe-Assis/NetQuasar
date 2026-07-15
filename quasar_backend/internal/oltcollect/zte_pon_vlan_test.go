package oltcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestDiscoverZteVlanCatalog_sortedAndSuggestIgnore(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: ZteVlanNameOIDBase + ".1", Value: "VLAN0001"},
		{OID: ZteVlanDescOIDBase + ".1", Value: ""},
		{OID: ZteVlanNameOIDBase + ".111", Value: "GERENCIA"},
		{OID: ZteVlanDescOIDBase + ".111", Value: ""},
		{OID: ZteVlanNameOIDBase + ".101", Value: "VLAN0101"},
		{OID: ZteVlanDescOIDBase + ".101", Value: "PPPOE-PON09"},
		{OID: ZteVlanNameOIDBase + ".9", Value: "VLAN0009"},
		{OID: ZteVlanDescOIDBase + ".9", Value: "PPPOE-PON01"},
	}
	got := DiscoverZteVlanCatalogFromSNMPVars(vars)
	if len(got) != 4 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].VID != 1 || !got[0].Ignored {
		t.Fatalf("vid1=%+v", got[0])
	}
	if got[1].VID != 9 || got[1].Ignored || got[1].Pon != 1 {
		t.Fatalf("vid9=%+v", got[1])
	}
	if got[2].VID != 101 || got[2].Pon != 9 {
		t.Fatalf("vid101=%+v", got[2])
	}
	if got[3].VID != 111 || !got[3].Ignored {
		t.Fatalf("vid111=%+v", got[3])
	}
}

func TestResolveAuthorizeVlanForPon_respectsIgnored(t *testing.T) {
	cfg := OnuReportConfig{
		AuthorizeVlanCatalog: []AuthorizeVlanCatalogEntry{
			{VID: 9, Description: "PPPOE-PON01", Pon: 1, Ignored: true},
			{VID: 10, Description: "PPPOE-PON02", Pon: 2, Ignored: false},
		},
	}
	if _, ok := ResolveAuthorizeVlanForPon(cfg, 1); ok {
		t.Fatal("ignored entry should not resolve")
	}
	v, ok := ResolveAuthorizeVlanForPon(cfg, 2)
	if !ok || v != "10" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}

func TestMergeAuthorizeVlanCatalog_preservesIgnored(t *testing.T) {
	prev := []AuthorizeVlanCatalogEntry{{VID: 9, Ignored: true}, {VID: 10, Ignored: false}}
	disc := []AuthorizeVlanCatalogEntry{
		{VID: 9, Description: "PPPOE-PON01", Pon: 1, Ignored: false},
		{VID: 10, Description: "PPPOE-PON02", Pon: 2, Ignored: true},
	}
	got := MergeAuthorizeVlanCatalog(disc, prev)
	if !got[0].Ignored || got[1].Ignored {
		t.Fatalf("%+v", got)
	}
}

func TestPonFromZteVlanDescription(t *testing.T) {
	cases := map[string]int{
		"PPPOE-PON01": 1,
		"PPPOE-PON16": 16,
		"pppoe-pon9":  9,
		"PPPOE_PON08": 8,
		"GERENCIA":    0,
		"":            0,
	}
	for in, want := range cases {
		if got := PonFromZteVlanDescription(in); got != want {
			t.Fatalf("%q: got=%d want=%d", in, got, want)
		}
	}
}
