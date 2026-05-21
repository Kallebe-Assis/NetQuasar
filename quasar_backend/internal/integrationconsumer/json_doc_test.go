package integrationconsumer

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func TestResponseBodyForParse_truncated(t *testing.T) {
	raw := []byte(`{"page":"1","registros":[{"id":"1","razao":"A","cnpj_cpf":"1"}]}` + responseTruncatedSuffix)
	out := ResponseBodyForParse(raw)
	if !decodeValidJSON(out) {
		t.Fatalf("invalid: %s", out)
	}
}

func TestResponseBodyForParse_embedded(t *testing.T) {
	raw := []byte(`Notice: x\n{"registros":[{"id":"1","razao":"A","cnpj_cpf":"1"}]}`)
	out := ResponseBodyForParse(raw)
	r := ParseIXCClientSearch(out)
	if !r.OK || len(r.Clients) != 1 {
		t.Fatalf("result=%+v", r)
	}
}

func decodeValidJSON(b []byte) bool {
	_, err := decodeJSONDocument(b)
	return err == nil
}

func TestDecodeJSONDocument_partialRegistros(t *testing.T) {
	// Simula JSON cortado no meio do 2º cliente
	s := `{"page":"1","total":"4024","registros":[{"id":"1","razao":"A","cnpj_cpf":"11111111111"},{"id":"2","razao":"B","cnpj_cpf":"22222222222"},{"id":"3","razao":"C","cnpj_cpf":"333`
	doc, err := decodeJSONDocument([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	items, _ := doc["registros"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 complete items, got %d", len(items))
	}
	r := ParseIXCClientSearch([]byte(s))
	if !r.OK || len(r.Clients) != 2 {
		t.Fatalf("parse=%+v", r)
	}
}

func TestDetectClientSearchProfile_IXC_over_hubsoft_config(t *testing.T) {
	rc := integrationhttp.RequestConfig{
		Method:  "POST",
		Path:    "/cliente",
		Headers: map[string]string{"ixcsoft": "listar"},
		BodyTemplate: `{"qtype":"cliente.id","query":"0","oper":">"}`,
	}
	if DetectClientSearchProfile(ProviderHubsoft, rc, "https://example.com") != ProviderIXC {
		t.Fatal("expected ixc over misconfigured hubsoft")
	}
}
