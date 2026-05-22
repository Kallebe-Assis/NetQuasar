package integrationconsumer

import (
	"strings"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

func TestPrepareIXCAttendanceListRequest(t *testing.T) {
	rc := integrationhttp.RequestConfig{
		Method: "POST",
		Path:   "/su_ticket",
		Headers: map[string]string{"ixcsoft": "listar"},
		BodyTemplate: `{"qtype":"su_ticket.id","query":"0","oper":">","page":"1","rp":"20","sortname":"su_ticket.id","sortorder":"desc"}`,
		BodyType: "json",
	}
	out := PrepareIXCAttendanceListRequest(rc, "codigo_cliente", "12345", ClientAttendanceConfig{}, ClientSearchConfig{})
	if len(out.QueryParams) != 0 {
		t.Fatal("query must be empty")
	}
	if !strings.Contains(out.BodyTemplate, `"query":"12345"`) && !strings.Contains(out.BodyTemplate, `"query": "12345"`) {
		t.Fatalf("body=%s", out.BodyTemplate)
	}
	if !strings.Contains(out.BodyTemplate, "su_ticket.id_cliente") {
		t.Fatalf("qtype must reference su_ticket.id_cliente: %s", out.BodyTemplate)
	}
}

func TestBuildIXCAttendanceAttempts_templateQtypeFirst(t *testing.T) {
	body := `{"qtype":"su_ticket.id_cliente","query":"0","oper":">","page":"1","rp":"50"}`
	attempts := BuildIXCAttendanceAttempts(ClientAttendanceConfig{}, "codigo_cliente", "72532", body)
	if len(attempts) == 0 {
		t.Fatal("no attempts")
	}
	if attempts[0].Qtype != "su_ticket.id_cliente" || attempts[0].Query != "72532" || attempts[0].Oper != "=" {
		t.Fatalf("first=%+v", attempts[0])
	}
}

func TestNormalizeIXCAttendanceQtype_clienteID(t *testing.T) {
	if got := normalizeIXCAttendanceQtype("id_cliente", "codigo_cliente"); got != "su_ticket.id_cliente" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeIXCAttendanceQtype("su_ticket.id", "codigo_cliente"); got != "su_ticket.id_cliente" {
		t.Fatalf("ticket id field must not be used for client: %q", got)
	}
}

func TestFilterAttendanceByClient_trustAPIWhenNoIdClienteInRows(t *testing.T) {
	items := []AttendanceItem{{
		ID:       "72532",
		Protocol: "2026001",
		Subject:  "Suporte",
		Raw:      map[string]interface{}{"id": "72532", "protocolo": "2026001", "titulo": "Suporte"},
	}}
	// Sem id_cliente no JSON — filtro não deve esvaziar se já veio da API filtrada (simulado fora).
	out := FilterAttendanceByClient(items, "codigo_cliente", "999")
	if len(out) != 0 {
		t.Fatalf("filter without id_cliente in row should drop: %+v", out)
	}
}

func TestFilterAttendanceByClient_idCliente(t *testing.T) {
	items := []AttendanceItem{{
		Protocol: "T1",
		Raw:      map[string]interface{}{"id_cliente": "42", "assunto": "A"},
	}, {
		Protocol: "T2",
		Raw:      map[string]interface{}{"id_cliente": "99"},
	}}
	out := FilterAttendanceByClient(items, "codigo_cliente", "42")
	if len(out) != 1 || out[0].Protocol != "T1" {
		t.Fatalf("filter=%+v", out)
	}
}

func TestParseIXCClientAttendance_registros(t *testing.T) {
	raw := []byte(`{"page":"1","total":"1","registros":[{"id":"99","protocolo":"2026001","assunto":"Suporte","status":"A"}]}`)
	r := ParseIXCClientAttendance(raw)
	if !r.OK || len(r.Items) != 1 {
		t.Fatalf("result=%+v", r)
	}
	if r.Items[0].Protocol != "2026001" {
		t.Fatalf("protocol=%s", r.Items[0].Protocol)
	}
}
