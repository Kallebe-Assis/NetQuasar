package integrationconsumer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseIXCClientSearch interpreta JSON típico do webservice IXC (page/total/registros).
func ParseIXCClientSearch(raw []byte) SearchResult {
	out := SearchResult{Clients: []ClientCard{}}
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
	items := extractIXCClientArray(doc)
	for _, it := range items {
		if card, ok := mapIXCClientItem(it); ok {
			out.Clients = append(out.Clients, card)
		}
	}
	out.OK = true
	if len(out.Clients) == 0 {
		out.Message = "Nenhum cliente encontrado para este termo."
	} else if strings.Contains(string(raw), responseTruncatedSuffix) || looksLikeTruncatedIXCList(string(raw)) {
		out.Message = fmt.Sprintf("%d cliente(s) carregados (resposta da API truncada no limite de transferência).", len(out.Clients))
	}
	return out
}

func looksLikeTruncatedIXCList(s string) bool {
	if json.Valid([]byte(s)) {
		return false
	}
	return strings.Contains(s, `"registros"`) && strings.Contains(s, `"page"`)
}

func ixcErrorMessage(doc map[string]any) string {
	for _, key := range []string{"message", "msg", "mensagem", "erro", "error"} {
		if v, ok := doc[key]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	t := strings.ToLower(strings.TrimSpace(fmt.Sprint(doc["type"])))
	if t == "error" || t == "erro" {
		return firstNonEmpty(
			strings.TrimSpace(fmt.Sprint(doc["message"])),
			"Erro retornado pela API IXC",
		)
	}
	return ""
}

func extractIXCClientArray(doc map[string]any) []any {
	if arr, ok := doc["registros"].([]any); ok {
		return arr
	}
	return extractClientArray(doc)
}

func mapIXCClientItem(it any) (ClientCard, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return ClientCard{}, false
	}
	// Alguns retornos aninham em «cliente».
	if sub, ok := m["cliente"].(map[string]any); ok {
		return mapClientItem(sub)
	}
	card := ClientCard{
		ID:        pickStr(m, "id", "id_cliente", "codigo_cliente", "cliente_id"),
		Code:      pickStr(m, "id", "codigo_cliente", "codigo"),
		Name:      pickStr(m, "razao", "nome_razaosocial", "nome", "razao_social", "name"),
		TradeName: pickStr(m, "fantasia", "nome_fantasia"),
		Document:  pickStr(m, "cnpj_cpf", "cpf_cnpj", "cpf", "cnpj", "documento"),
		Email:     pickStr(m, "email", "email_principal"),
		Phone:     pickStr(m, "telefone", "fone", "whatsapp", "celular"),
		Status:    pickStr(m, "status", "status_cadastro", "ativo", "situacao"),
		Address:   formatAddress(m),
		Details:   map[string]string{},
	}
	if card.Name == "" && card.Code == "" && card.Document == "" {
		return ClientCard{}, false
	}
	card.Raw = cloneRawMap(m)
	card.Services = mapServices(m)
	card.IPv4 = extractClientIPv4(m)
	return card, true
}
