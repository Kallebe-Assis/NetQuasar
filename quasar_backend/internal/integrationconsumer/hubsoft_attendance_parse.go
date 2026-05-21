package integrationconsumer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AttendanceItem atendimento normalizado para a UI.
type AttendanceItem struct {
	ID          string                 `json:"id,omitempty"`
	Protocol    string                 `json:"protocol,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Subject     string                 `json:"subject,omitempty"`
	Description string                 `json:"description,omitempty"`
	OpenedAt    string                 `json:"opened_at,omitempty"`
	ClosedAt    string                 `json:"closed_at,omitempty"`
	Pending     *bool                  `json:"pending,omitempty"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}

// AttendanceResult resultado da consulta de atendimentos.
type AttendanceResult struct {
	OK        bool             `json:"ok"`
	Message   string           `json:"message,omitempty"`
	Items     []AttendanceItem `json:"items"`
	RawStatus string           `json:"raw_status,omitempty"`
}

// ParseHubsoftClientAttendance interpreta JSON da Hubsoft (atendimentos).
func ParseHubsoftClientAttendance(raw []byte) AttendanceResult {
	out := AttendanceResult{Items: []AttendanceItem{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	out.RawStatus = strings.TrimSpace(fmt.Sprint(doc["status"]))
	st := strings.ToLower(out.RawStatus)
	if st == "error" || st == "erro" {
		out.OK = false
		out.Message = firstNonEmpty(
			strings.TrimSpace(fmt.Sprint(doc["msg"])),
			strings.TrimSpace(fmt.Sprint(doc["message"])),
			"Erro retornado pela API",
		)
		return out
	}
	items := extractAttendanceArray(doc)
	for _, it := range items {
		if row, ok := mapAttendanceItem(it); ok {
			out.Items = append(out.Items, row)
		}
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhum atendimento encontrado."
	}
	return out
}

func extractAttendanceArray(doc map[string]any) []any {
	for _, key := range []string{"atendimentos", "registros", "results", "data", "items"} {
		if arr, ok := doc[key].([]any); ok && len(arr) > 0 {
			return arr
		}
	}
	if data, ok := doc["data"].(map[string]any); ok {
		for _, key := range []string{"atendimentos", "registros", "results", "items"} {
			if arr, ok := data[key].([]any); ok && len(arr) > 0 {
				return arr
			}
		}
	}
	if _, hasProto := doc["protocolo"]; hasProto {
		return []any{doc}
	}
	return nil
}

func mapAttendanceItem(it any) (AttendanceItem, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return AttendanceItem{}, false
	}
	row := AttendanceItem{
		ID:       pickStr(m, "id_atendimento", "id", "uuid_atendimento"),
		Protocol: pickStr(m, "protocolo", "numero_protocolo", "protocolo_atendimento"),
		Status:   pickStr(m, "status", "status_atendimento", "situacao", "situacao_atendimento"),
		Subject: firstNonEmpty(
			pickStr(m, "assunto", "titulo", "motivo", "tipo_atendimento", "categoria"),
		),
		Description: firstNonEmpty(
			pickStr(m, "descricao", "descricao_atendimento", "observacao", "observacoes", "detalhe", "detalhes", "conteudo", "relato", "mensagem", "texto"),
		),
		OpenedAt: pickStr(m, "data_cadastro", "data_abertura", "created_at", "aberto_em"),
		ClosedAt: pickStr(m, "data_fechamento", "fechado_em", "closed_at"),
	}
	if row.Description == "" {
		row.Description = pickStr(m, "descricao_resumida", "resumo")
	}
	if row.Protocol == "" && row.ID == "" && row.Status == "" && row.Subject == "" && row.Description == "" {
		return AttendanceItem{}, false
	}
	if p := pickStr(m, "pendente", "aberto", "em_aberto"); p != "" {
		low := strings.ToLower(p)
		v := low == "sim" || low == "true" || low == "1" || low == "s"
		row.Pending = &v
	}
	row.Raw = cloneRawMap(m)
	return row, true
}
