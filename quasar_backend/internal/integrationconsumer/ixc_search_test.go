package integrationconsumer

import (
	"strings"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func TestDetectClientSearchProfile_IXC_POST(t *testing.T) {
	rc := integrationhttp.RequestConfig{
		Method: "POST",
		Path:   "/cliente",
		Headers: map[string]string{"ixcsoft": "listar"},
		BodyTemplate: `{"qtype":"cliente.id","query":"0","oper":">","rp":"1"}`,
		BodyType: "json",
	}
	if DetectClientSearchProfile(ProviderAuto, rc, "https://ixc.example/webservice/v1") != ProviderIXC {
		t.Fatal("expected ixc")
	}
}

func TestApplyIXCBodySearch_cpf(t *testing.T) {
	body := ApplyIXCBodySearch("", "cpf_cnpj", "13240117690", false, ClientSearchConfig{})
	if !strings.Contains(body, "cliente.cnpj_cpf") {
		t.Fatalf("qtype: %s", body)
	}
	if !strings.Contains(body, "13240117690") {
		t.Fatalf("query: %s", body)
	}
}

func TestIXCSearchAttempts_cpf(t *testing.T) {
	atts := BuildIXCSearchAttempts(ClientSearchConfig{}, "cpf_cnpj", "132.401.176-90")
	if len(atts) < 3 {
		t.Fatalf("expected multiple attempts, got %d", len(atts))
	}
	if atts[0].Qtype != "cliente.cnpj_cpf" || atts[0].Oper != "=" {
		t.Fatalf("first=%+v", atts[0])
	}
}

func TestFilterClientsByDocument(t *testing.T) {
	clients := []ClientCard{
		{Document: "132.401.176-90", Name: "A"},
		{Document: "99999999999", Name: "B"},
	}
	out := FilterClientsByDocument(clients, "13240117690")
	if len(out) != 1 || out[0].Name != "A" {
		t.Fatalf("filter=%+v", out)
	}
}

func TestPrepareIXC_migratesQueryToBody(t *testing.T) {
	rc := integrationhttp.RequestConfig{
		Method: "POST",
		Path:   "/cliente",
		QueryParams: []integrationhttp.ParamKV{
			{Key: "qtype", Value: "{{qtype}}"},
			{Key: "oper", Value: ">"},
			{Key: "sortname", Value: "cliente.id"},
		},
		BodyType: "json",
	}
	out := PrepareIXCClientListRequest(rc, "cpf_cnpj", "13240117690", false, ClientSearchConfig{})
	if len(out.QueryParams) != 0 {
		t.Fatal("query should be empty")
	}
	if !strings.Contains(out.BodyTemplate, "cliente.cnpj_cpf") {
		t.Fatalf("body=%s", out.BodyTemplate)
	}
	if !strings.Contains(out.BodyTemplate, "13240117690") {
		t.Fatalf("body=%s", out.BodyTemplate)
	}
}

func TestApplyClientSearchContext_IXC_coercesGETToPOST(t *testing.T) {
	rc := integrationhttp.RequestConfig{
		Method: "GET",
		Path:   "/cliente",
		QueryParams: []integrationhttp.ParamKV{
			{Key: "busca", Value: "cpf_cnpj"},
			{Key: "termo_busca", Value: "13240117690"},
		},
	}
	out := ApplyClientSearchContext(rc, ProviderIXC, "cpf_cnpj", "13240117690", false, ClientSearchConfig{})
	if out.Method != "POST" {
		t.Fatalf("method=%s", out.Method)
	}
	if len(out.QueryParams) != 0 {
		t.Fatalf("query params should be cleared: %+v", out.QueryParams)
	}
	if out.Headers["ixcsoft"] != "listar" {
		t.Fatalf("header=%v", out.Headers)
	}
	if !strings.Contains(out.BodyTemplate, "13240117690") {
		t.Fatalf("body=%s", out.BodyTemplate)
	}
}

func TestParseIXCClientSearch_registros(t *testing.T) {
	raw := []byte(`{"page":"1","total":"1","registros":[{"id":"99","razao":"Cliente IXC","cnpj_cpf":"13240117690"}]}`)
	r := ParseIXCClientSearch(raw)
	if !r.OK || len(r.Clients) != 1 {
		t.Fatalf("result=%+v", r)
	}
	if r.Clients[0].Document != "13240117690" {
		t.Fatalf("doc=%s", r.Clients[0].Document)
	}
}
