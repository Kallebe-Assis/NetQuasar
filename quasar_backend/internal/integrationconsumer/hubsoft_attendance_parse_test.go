package integrationconsumer

import "testing"

func TestParseHubsoftClientAttendance_items(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"atendimentos":[
			{"protocolo":"AT-100","status":"Aberto","data_cadastro":"2024-01-10","assunto":"Sem internet"}
		]
	}`)
	r := ParseHubsoftClientAttendance(raw)
	if !r.OK || len(r.Items) != 1 {
		t.Fatalf("ok=%v items=%d", r.OK, len(r.Items))
	}
	if r.Items[0].Protocol != "AT-100" || r.Items[0].Subject != "Sem internet" {
		t.Fatalf("item=%+v", r.Items[0])
	}
}
