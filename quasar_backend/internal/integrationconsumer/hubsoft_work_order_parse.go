package integrationconsumer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WorkOrderItem ordem de serviço normalizada para a UI.
type WorkOrderItem struct {
	ID                 string                 `json:"id,omitempty"`
	Number             string                 `json:"number,omitempty"`
	Status             string                 `json:"status,omitempty"`
	StatusLabel        string                 `json:"status_label,omitempty"`
	PlanName           string                 `json:"plan_name,omitempty"`
	ServiceStatus      string                 `json:"service_status,omitempty"`
	Value              string                 `json:"value,omitempty"`
	Description        string                 `json:"description,omitempty"`
	ScheduledAt        string                 `json:"scheduled_at,omitempty"`
	CreatedAt          string                 `json:"created_at,omitempty"`
	AttendanceProtocol string                 `json:"attendance_protocol,omitempty"`
	Raw                map[string]interface{} `json:"raw,omitempty"`
}

// WorkOrderResult resultado da consulta de ordens de serviço.
type WorkOrderResult struct {
	OK        bool            `json:"ok"`
	Message   string          `json:"message,omitempty"`
	Items     []WorkOrderItem `json:"items"`
	RawStatus string          `json:"raw_status,omitempty"`
}

// ParseHubsoftClientWorkOrder interpreta JSON da Hubsoft (ordens de serviço).
func ParseHubsoftClientWorkOrder(raw []byte) WorkOrderResult {
	out := WorkOrderResult{Items: []WorkOrderItem{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	out.RawStatus = strings.TrimSpace(scalarToString(doc["status"]))
	st := strings.ToLower(out.RawStatus)
	if st == "error" || st == "erro" {
		out.OK = false
		out.Message = firstNonEmpty(
			strings.TrimSpace(scalarToString(doc["msg"])),
			strings.TrimSpace(scalarToString(doc["message"])),
			"Erro retornado pela API",
		)
		return out
	}
	items := extractWorkOrderArray(doc)
	for _, it := range items {
		if row, ok := mapWorkOrderItem(it); ok {
			out.Items = append(out.Items, row)
		}
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhuma ordem de serviço encontrada."
	}
	return out
}

func extractWorkOrderArray(doc map[string]any) []any {
	for _, key := range []string{
		"ordens_servico", "ordem_servico", "ordens", "registros", "results", "data", "items",
	} {
		if arr, ok := doc[key].([]any); ok && len(arr) > 0 {
			return arr
		}
	}
	if data, ok := doc["data"].(map[string]any); ok {
		for _, key := range []string{"ordens_servico", "ordem_servico", "ordens", "registros", "results", "items"} {
			if arr, ok := data[key].([]any); ok && len(arr) > 0 {
				return arr
			}
		}
	}
	if pickStr(doc, "numero_ordem_servico", "id_ordem_servico", "id") != "" {
		return []any{doc}
	}
	return nil
}

func mapWorkOrderItem(it any) (WorkOrderItem, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return WorkOrderItem{}, false
	}
	row := WorkOrderItem{
		ID:     pickStr(m, "id_ordem_servico", "id", "uuid_ordem_servico"),
		Number: pickStr(m, "numero_ordem_servico", "numero", "numero_os"),
		Status: pickStr(m, "status", "status_ordem_servico", "situacao", "status_prefixo"),
		Description: firstNonEmpty(
			pickStr(m, "descricao", "descricao_ordem_servico", "observacao", "observacoes", "detalhe", "detalhes"),
		),
		ScheduledAt: pickStr(m, "data_inicio_programado", "data_agendamento", "agendado_para", "data_programada"),
		CreatedAt:   pickStr(m, "data_cadastro", "created_at", "aberto_em"),
	}
	if row.Number == "" {
		row.Number = row.ID
	}
	row.StatusLabel = formatWorkOrderStatusLabel(row.Status)

	planName, svcStatus, value := extractWorkOrderServiceBlock(m)
	row.PlanName = planName
	row.ServiceStatus = svcStatus
	row.Value = value

	if row.Description == "" && row.PlanName != "" {
		parts := []string{row.PlanName}
		if row.ServiceStatus != "" {
			parts = append(parts, row.ServiceStatus)
		}
		if row.Value != "" {
			parts = append(parts, row.Value)
		}
		row.Description = strings.Join(parts, " · ")
	}

	row.AttendanceProtocol = pickStr(m, "protocolo", "numero_protocolo", "protocolo_atendimento", "numero_atendimento")
	if at := pickNestedMap(m, "atendimento"); at != nil {
		row.AttendanceProtocol = firstNonEmpty(
			row.AttendanceProtocol,
			pickStr(at, "protocolo", "numero_protocolo", "protocolo_atendimento"),
		)
	}

	if row.Number == "" && row.ID == "" && row.Status == "" && row.PlanName == "" && row.Description == "" {
		return WorkOrderItem{}, false
	}
	row.Raw = cloneRawMap(m)
	return row, true
}

func extractWorkOrderServiceBlock(m map[string]any) (planName, serviceStatus, value string) {
	for _, key := range []string{
		"cliente_servico", "servico", "plano", "cliente_servico_plano", "servico_cliente", "dados_servico",
	} {
		sub := pickNestedMap(m, key)
		if sub == nil {
			continue
		}
		planName = firstNonEmpty(
			planName,
			pickStr(sub, "nome", "descricao", "nome_plano", "nome_servico", "plano", "descricao_plano"),
		)
		serviceStatus = firstNonEmpty(
			serviceStatus,
			pickStr(sub, "status", "status_servico", "situacao"),
			formatWorkOrderStatusLabel(pickStr(sub, "status_prefixo")),
		)
		if value == "" {
			value = formatMoneyValue(sub["valor"], sub["valor_plano"], sub["preco"], sub["mensalidade"])
		}
	}
	return planName, serviceStatus, value
}

func formatMoneyValue(keys ...any) string {
	for _, k := range keys {
		s := scalarToString(k)
		if s == "" {
			continue
		}
		var f float64
		if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
			return fmt.Sprintf("R$ %.2f", f)
		}
		return s
	}
	return ""
}

func formatWorkOrderStatusLabel(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	labels := map[string]string{
		"pendente":               "Pendente",
		"aguardando_agendamento": "Aguardando agendamento",
		"aguardando_instalacao":  "Aguardando instalação",
		"finalizado":             "Finalizado",
		"cancelado":              "Cancelado",
		"em_andamento":           "Em andamento",
		"concluido":              "Concluído",
	}
	if lbl, ok := labels[s]; ok {
		return lbl
	}
	return humanizeUnderscore(s)
}

func humanizeUnderscore(s string) string {
	parts := strings.Fields(strings.ReplaceAll(s, "_", " "))
	for i, p := range parts {
		if p == "" {
			continue
		}
		low := strings.ToLower(p)
		if len(low) > 0 {
			parts[i] = strings.ToUpper(low[:1]) + low[1:]
		}
	}
	return strings.Join(parts, " ")
}
