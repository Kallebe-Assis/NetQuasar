package integrationconsumer

import (
	"strings"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func TestBuildIXCWorkOrderAttempts_templateQtype(t *testing.T) {
	body := `{"qtype":"su_oss_chamado.id_cliente","query":"0","oper":"=","page":"1","rp":"20"}`
	attempts := BuildIXCWorkOrderAttempts(ClientWorkOrderConfig{}, "codigo_cliente", "72532", body)
	if len(attempts) == 0 {
		t.Fatal("no attempts")
	}
	if attempts[0].Qtype != "su_oss_chamado.id_cliente" || attempts[0].Query != "72532" {
		t.Fatalf("first=%+v", attempts[0])
	}
}

func TestNormalizeIXCWorkOrderQtype(t *testing.T) {
	if got := normalizeIXCWorkOrderQtype("id_cliente", "codigo_cliente"); got != "su_oss_chamado.id_cliente" {
		t.Fatalf("got %q", got)
	}
}

func TestParseIXCClientWorkOrder_registros(t *testing.T) {
	raw := []byte(`{"page":"1","total":"1","registros":[{"id":"10","protocolo":"OS-1","status":"A","id_cliente":"72532"}]}`)
	r := ParseIXCClientWorkOrder(raw)
	if !r.OK || len(r.Items) != 1 {
		t.Fatalf("result=%+v", r)
	}
	if r.Items[0].Number != "OS-1" {
		t.Fatalf("number=%s", r.Items[0].Number)
	}
}

func TestLooksLikeIXCWorkOrderRequest(t *testing.T) {
	rc := integrationhttp.RequestConfig{
		Path: "/su_oss_chamado",
		Headers: map[string]string{"ixcsoft": "listar"},
		BodyTemplate: `{"qtype":"su_oss_chamado.id_cliente"}`,
	}
	if !LooksLikeIXCWorkOrderRequest(rc) {
		t.Fatal("expected IXC work order")
	}
}

func TestApplyIXCWorkOrderBodySearchAttempt(t *testing.T) {
	body := `{"qtype":"su_oss_chamado.id_cliente","query":"0","oper":"=","sortname":"su_oss_chamado.id","sortorder":"asc"}`
	out := ApplyIXCWorkOrderBodySearchAttempt(body, IXCSearchAttempt{
		Qtype: "su_oss_chamado.id_cliente",
		Query: "99",
		Oper:  "=",
	})
	if !strings.Contains(out, `"query":"99"`) && !strings.Contains(out, `"query": "99"`) {
		t.Fatalf("body=%s", out)
	}
}
