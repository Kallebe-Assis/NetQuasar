package networkevents

import "testing"

func TestCatalogCodesUnique(t *testing.T) {
	seen := map[string]string{}
	for _, typ := range Types {
		if typ.Code == "" || typ.Label == "" || typ.CategoryCode == "" {
			t.Fatalf("tipo incompleto: %+v", typ)
		}
		if CategoryByCode(typ.CategoryCode) == nil {
			t.Fatalf("categoria desconhecida %q no tipo %s", typ.CategoryCode, typ.Code)
		}
		if prev, ok := seen[typ.Code]; ok {
			t.Fatalf("código duplicado %s (%s / %s)", typ.Code, prev, typ.Label)
		}
		seen[typ.Code] = typ.Label
	}
	if len(Types) < 200 {
		t.Fatalf("catálogo incompleto: %d tipos", len(Types))
	}
}
