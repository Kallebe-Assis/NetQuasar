package integrationconsumer

import "strings"

// ParseClientAttendance interpreta resposta conforme o perfil do ERP.
func ParseClientAttendance(raw []byte, profile string) AttendanceResult {
	raw = ResponseBodyForParse(raw)
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProviderIXC:
		return ParseIXCClientAttendance(raw)
	default:
		return ParseHubsoftClientAttendance(raw)
	}
}

// ParseIXCClientAttendance interpreta JSON IXC (su_ticket — registros).
func ParseIXCClientAttendance(raw []byte) AttendanceResult {
	out := AttendanceResult{Items: []AttendanceItem{}}
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
	items := extractAttendanceArray(doc)
	for _, it := range items {
		if row, ok := mapIXCAttendanceItem(it); ok {
			out.Items = append(out.Items, row)
		}
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhum atendimento encontrado."
	}
	return out
}

func mapIXCAttendanceItem(it any) (AttendanceItem, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return AttendanceItem{}, false
	}
	row := AttendanceItem{
		ID:       pickStr(m, "id", "id_ticket", "su_ticket_id"),
		Protocol: pickStr(m, "protocolo", "numero_protocolo"),
		Status:   pickStr(m, "status", "su_status", "situacao", "estado"),
		Subject: firstNonEmpty(
			pickStr(m, "titulo", "assunto", "id_assunto", "motivo", "tipo", "categoria"),
		),
		Description: firstNonEmpty(
			pickStr(m, "menssagem", "mensagem", "descricao", "observacao", "obs", "relato", "conteudo"),
		),
		OpenedAt: pickStr(m, "data_criacao", "data_abertura", "data_cadastro", "data", "abertura"),
		ClosedAt: pickStr(m, "data_fechamento", "data_encerramento", "fechamento", "data_ultima_alteracao"),
	}
	if row.Protocol == "" && row.ID == "" && row.Subject == "" && row.Description == "" {
		if pickStr(m, "id_cliente", "id_contrato", "id_login") == "" {
			return AttendanceItem{}, false
		}
	}
	if st := strings.ToLower(firstNonEmpty(pickStr(m, "su_status"), pickStr(m, "status", "situacao"))); st != "" {
		v := st == "a" || st == "aberto" || strings.Contains(st, "abert") || st == "pendente"
		row.Pending = &v
	}
	row.Raw = cloneRawMap(m)
	return row, true
}
