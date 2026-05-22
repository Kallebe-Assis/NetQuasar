package integrationconsumer

import "testing"

func TestEnrichServicesWithContractStatus(t *testing.T) {
	services := []ServiceSummary{
		{ContratoID: "100", StatusInternet: "A", StatusLabel: "Ativo"},
		{Login: "user1", ContratoID: "100"},
	}
	out := enrichServicesWithContractStatus(services)
	if out[1].StatusInternet != "A" {
		t.Fatalf("login status_internet = %q, want A", out[1].StatusInternet)
	}
	if out[1].StatusLabel != "Ativo" {
		t.Fatalf("login status_label = %q, want Ativo", out[1].StatusLabel)
	}
}

func TestBuildClientCardsFromLoginServices(t *testing.T) {
	items := []ServiceSummary{
		{ClientID: "42", ClientName: "Cliente X", Login: "a@net", Online: "S"},
		{ClientID: "42", Login: "b@net", Online: "N"},
		{ClientID: "99", Login: "solo@net"},
	}
	cards := BuildClientCardsFromLoginServices(items)
	if len(cards) != 2 {
		t.Fatalf("len(cards)=%d, want 2", len(cards))
	}
	if cards[0].ID != "42" || len(cards[0].Services) != 2 {
		t.Fatalf("client 42: id=%s services=%d", cards[0].ID, len(cards[0].Services))
	}
}
