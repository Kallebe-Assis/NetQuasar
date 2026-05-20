package integrationconsumer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ClientCard dados normalizados para a UI.
type ClientCard struct {
	ID          string                 `json:"id,omitempty"`
	Code        string                 `json:"code,omitempty"`
	Name        string                 `json:"name,omitempty"`
	TradeName   string                 `json:"trade_name,omitempty"`
	Document    string                 `json:"document,omitempty"`
	Email       string                 `json:"email,omitempty"`
	Phone       string                 `json:"phone,omitempty"`
	IPv4        string                 `json:"ipv4,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Address     string                 `json:"address,omitempty"`
	Services    []ServiceSummary       `json:"services,omitempty"`
	Details     map[string]string      `json:"details,omitempty"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}

// ServiceSummary resumo de serviço/plano do cliente.
type ServiceSummary struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
	Login  string `json:"login,omitempty"`
	IPv4   string `json:"ipv4,omitempty"`
}

// SearchResult resultado estruturado da consulta.
type SearchResult struct {
	OK       bool         `json:"ok"`
	Message  string       `json:"message,omitempty"`
	Clients  []ClientCard `json:"clients"`
	RawStatus string      `json:"raw_status,omitempty"`
}

// ParseHubsoftClientSearch interpreta JSON típico da Hubsoft (status + clientes/registros).
func ParseHubsoftClientSearch(raw []byte) SearchResult {
	out := SearchResult{Clients: []ClientCard{}}
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
	items := extractClientArray(doc)
	for _, it := range items {
		if card, ok := mapClientItem(it); ok {
			out.Clients = append(out.Clients, card)
		}
	}
	out.OK = true
	if len(out.Clients) == 0 {
		out.Message = "Nenhum cliente encontrado para este termo."
	}
	return out
}

func extractClientArray(doc map[string]any) []any {
	for _, key := range []string{"clientes", "registros", "results", "data", "items"} {
		if arr, ok := doc[key].([]any); ok && len(arr) > 0 {
			return arr
		}
	}
	if data, ok := doc["data"].(map[string]any); ok {
		for _, key := range []string{"clientes", "registros", "results", "items"} {
			if arr, ok := data[key].([]any); ok && len(arr) > 0 {
				return arr
			}
		}
	}
	// único objeto cliente
	if _, hasName := doc["nome_razaosocial"]; hasName {
		return []any{doc}
	}
	if _, hasName := doc["nome"]; hasName {
		return []any{doc}
	}
	return nil
}

func mapClientItem(it any) (ClientCard, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return ClientCard{}, false
	}
	card := ClientCard{
		ID:        pickStr(m, "id_cliente", "id", "uuid_cliente", "codigo_cliente"),
		Code:      pickStr(m, "codigo_cliente", "codigo", "id_cliente"),
		Name:      pickStr(m, "nome_razaosocial", "nome", "razao_social", "name"),
		TradeName: pickStr(m, "nome_fantasia", "fantasia"),
		Document:  pickStr(m, "cpf_cnpj", "cpf", "cnpj", "documento"),
		Email:     pickStr(m, "email_principal", "email", "email_secundario"),
		Phone:     pickStr(m, "telefone", "telefone_principal", "celular"),
		Status:    pickStr(m, "status_cadastro", "status", "situacao"),
		Address:   formatAddress(m),
		IPv4:      extractClientIPv4(m),
		Details:   map[string]string{},
	}
	if card.Name == "" && card.Code == "" && card.Document == "" {
		return ClientCard{}, false
	}
	card.Raw = cloneRawMap(m)
	card.Services = mapServices(m)
	if card.IPv4 == "" {
		card.IPv4 = firstServiceIPv4(card.Services)
	}
	// Campos úteis extra
	for _, k := range []string{"observacao", "observacoes", "data_cadastro", "cidade", "bairro"} {
		if v := pickStr(m, k); v != "" {
			card.Details[k] = v
		}
	}
	return card, true
}

func mapServices(m map[string]any) []ServiceSummary {
	var raw []any
	for _, key := range []string{"servicos", "services", "planos", "cliente_servico"} {
		if arr, ok := m[key].([]any); ok {
			raw = arr
			break
		}
	}
	var out []ServiceSummary
	for _, it := range raw {
		sm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ServiceSummary{
			ID:     pickStr(sm, "id_cliente_servico", "id", "uuid_cliente_servico"),
			Name:   pickStr(sm, "nome", "descricao", "plano", "servico"),
			Status: pickStr(sm, "status", "status_servico", "servico_status"),
			Login:  pickStr(sm, "login_radius", "login", "usuario"),
			IPv4:   pickIPv4FromMap(sm),
		})
	}
	return out
}

func firstServiceIPv4(services []ServiceSummary) string {
	var ips []string
	seen := map[string]struct{}{}
	for _, s := range services {
		if s.IPv4 == "" {
			continue
		}
		if _, dup := seen[s.IPv4]; dup {
			continue
		}
		seen[s.IPv4] = struct{}{}
		ips = append(ips, s.IPv4)
	}
	return strings.Join(ips, ", ")
}

func formatAddress(m map[string]any) string {
	parts := []string{
		pickStr(m, "endereco_instalacao", "endereco", "logradouro"),
		pickStr(m, "numero"),
		pickStr(m, "bairro"),
		pickStr(m, "cidade"),
		pickStr(m, "uf", "estado"),
	}
	var clean []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, ", ")
}

func pickStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func cloneRawMap(m map[string]any) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
