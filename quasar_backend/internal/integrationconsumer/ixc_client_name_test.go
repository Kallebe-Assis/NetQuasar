package integrationconsumer

import "testing"

func TestCardNeedsClientNameEnrich(t *testing.T) {
	if !cardNeedsClientNameEnrich(ClientCard{ID: "42", Name: "42"}) {
		t.Fatal("name==id should need enrich")
	}
	if cardNeedsClientNameEnrich(ClientCard{ID: "42", Name: "Empresa LTDA"}) {
		t.Fatal("real name should not need enrich")
	}
}

func TestIxcClientNameFromRow(t *testing.T) {
	row := map[string]any{
		"cliente": map[string]any{"razao": "Provedor X"},
	}
	if got := ixcClientNameFromRow(row); got != "Provedor X" {
		t.Fatalf("got %q", got)
	}
}
