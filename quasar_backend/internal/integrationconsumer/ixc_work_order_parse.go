package integrationconsumer

import "strings"

// ParseClientWorkOrder interpreta resposta conforme o perfil do ERP.
func ParseClientWorkOrder(raw []byte, profile string) WorkOrderResult {
	raw = ResponseBodyForParse(raw)
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProviderIXC:
		return ParseIXCClientWorkOrder(raw)
	default:
		return ParseHubsoftClientWorkOrder(raw)
	}
}

// ParseIXCClientWorkOrder interpreta JSON IXC (su_oss_chamado — registros).
func ParseIXCClientWorkOrder(raw []byte) WorkOrderResult {
	out := WorkOrderResult{Items: []WorkOrderItem{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	doc, err := decodeJSONDocument(raw)
	if err != nil {
		out.Message = NonJSONResponseHint(raw, 0)
		return out
	}
	if msg := ixcErrorMessage(doc); msg != "" {
		out.OK = false
		out.Message = msg
		return out
	}
	items := extractWorkOrderArray(doc)
	for _, it := range items {
		if row, ok := mapIXCWorkOrderItem(it); ok {
			out.Items = append(out.Items, row)
		}
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhuma ordem de serviço encontrada."
	}
	return out
}

func mapIXCWorkOrderItem(it any) (WorkOrderItem, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return WorkOrderItem{}, false
	}
	row := WorkOrderItem{
		ID:     pickStr(m, "id", "id_oss_chamado", "id_ordem_servico"),
		Number: pickStr(m, "protocolo", "numero", "numero_ordem_servico", "id"),
		Status: pickStr(m, "status", "status_oss", "situacao", "estado"),
		Description: firstNonEmpty(
			pickStr(m, "mensagem", "menssagem", "assunto", "titulo", "descricao", "observacao", "tipo"),
		),
		ScheduledAt: pickStr(m, "data_agenda", "data_agendamento", "data_inicio_programado", "data_reservada"),
		CreatedAt:   pickStr(m, "data_abertura", "data_cadastro", "data_criacao", "data"),
		AttendanceProtocol: pickStr(m, "protocolo_atendimento", "id_ticket", "protocolo"),
	}
	if row.Number == "" {
		row.Number = row.ID
	}
	row.StatusLabel = formatWorkOrderStatusLabel(row.Status)
	row.PlanName = firstNonEmpty(pickStr(m, "tipo", "id_assunto", "assunto"))
	if row.Number == "" && row.ID == "" && row.Status == "" && row.Description == "" {
		if pickStr(m, "id_cliente", "id_contrato") == "" {
			return WorkOrderItem{}, false
		}
	}
	row.Raw = cloneRawMap(m)
	return row, true
}
