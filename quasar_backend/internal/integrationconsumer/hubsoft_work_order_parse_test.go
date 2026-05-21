package integrationconsumer

import "testing"

func TestParseHubsoftClientWorkOrder_items(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"ordens_servico":[
			{"numero_ordem_servico":"OS-42","status":"pendente","descricao":"Instalação fibra","data_cadastro":"2024-02-01"}
		]
	}`)
	r := ParseHubsoftClientWorkOrder(raw)
	if !r.OK || len(r.Items) != 1 {
		t.Fatalf("ok=%v items=%d", r.OK, len(r.Items))
	}
	if r.Items[0].Number != "OS-42" || r.Items[0].Description != "Instalação fibra" {
		t.Fatalf("item=%+v", r.Items[0])
	}
}

func TestParseHubsoftClientWorkOrder_nested_servico(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"ordens_servico":[{
			"id_ordem_servico":"99",
			"numero_ordem_servico":"135826133143118941",
			"status":"aguardando_agendamento",
			"protocolo":"20260513143000177488",
			"data_cadastro":"13/05/2026 14:31:18",
			"data_inicio_programado":"01/06/2026 07:10:00",
			"cliente_servico":{
				"id_cliente_servico":8,
				"nome":"200 MB - PRÉ PAGO",
				"status":"Aguardando Instalação",
				"status_prefixo":"aguardando_instalacao",
				"valor":79.9
			}
		}]
	}`)
	r := ParseHubsoftClientWorkOrder(raw)
	if !r.OK || len(r.Items) != 1 {
		t.Fatalf("ok=%v items=%d msg=%s", r.OK, len(r.Items), r.Message)
	}
	it := r.Items[0]
	if it.PlanName != "200 MB - PRÉ PAGO" {
		t.Fatalf("plan=%q", it.PlanName)
	}
	if it.ServiceStatus != "Aguardando Instalação" {
		t.Fatalf("svc=%q", it.ServiceStatus)
	}
	if it.Value != "R$ 79.90" {
		t.Fatalf("value=%q", it.Value)
	}
	if it.StatusLabel != "Aguardando agendamento" {
		t.Fatalf("status_label=%q", it.StatusLabel)
	}
	if it.AttendanceProtocol != "20260513143000177488" {
		t.Fatalf("protocol=%q", it.AttendanceProtocol)
	}
	if stringsContainsMap(it.Description) {
		t.Fatalf("description should not contain map[: %q", it.Description)
	}
}

func stringsContainsMap(s string) bool {
	return len(s) >= 4 && s[:4] == "map["
}
