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

// ServiceSummary resumo de serviço/plano/contrato do cliente.
type ServiceSummary struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
	Login  string `json:"login,omitempty"`
	IPv4   string `json:"ipv4,omitempty"`
}

// SearchResult resultado estruturado da consulta.
type SearchResult struct {
	OK        bool         `json:"ok"`
	Message   string       `json:"message,omitempty"`
	Clients   []ClientCard `json:"clients"`
	RawStatus string       `json:"raw_status,omitempty"`
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
		Details:   map[string]string{},
	}
	if card.Name == "" && card.Code == "" && card.Document == "" {
		return ClientCard{}, false
	}
	card.Raw = cloneRawMap(m)
	card.Services = mapServices(m)
	for _, k := range []string{"observacao", "observacoes", "data_cadastro", "cidade", "bairro"} {
		if v := pickStr(m, k); v != "" {
			card.Details[k] = v
		}
	}
	return card, true
}

func mapServices(m map[string]any) []ServiceSummary {
	seen := map[string]struct{}{}
	var out []ServiceSummary
	appendItem := func(sm map[string]any) {
		svc := mapServiceItem(sm)
		if svc.Name == "" && svc.Login == "" && svc.IPv4 == "" && svc.ID == "" {
			return
		}
		key := svc.ID + "|" + svc.Login + "|" + svc.IPv4 + "|" + svc.Name
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, svc)
	}
	for _, key := range []string{"servicos", "services", "planos", "cliente_servico", "contratos", "contrato"} {
		raw, ok := m[key].([]any)
		if !ok {
			continue
		}
		for _, it := range raw {
			sm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			for _, nestKey := range []string{"servicos", "services", "cliente_servico", "cliente_servicos"} {
				if nested, ok := sm[nestKey].([]any); ok {
					for _, n := range nested {
						if nsm, ok := n.(map[string]any); ok {
							appendItem(nsm)
						}
					}
				}
			}
			appendItem(sm)
		}
	}
	return out
}

func mapServiceItem(sm map[string]any) ServiceSummary {
	name := pickStr(sm, "nome", "descricao", "plano", "servico", "nome_plano", "nome_servico", "descricao_plano")
	if name == "" {
		name = pickStr(sm, "tipo", "tipo_servico", "pacote")
	}
	return ServiceSummary{
		ID:     pickStr(sm, "id_cliente_servico", "id_contrato", "id", "uuid_cliente_servico", "codigo_contrato", "codigo"),
		Name:   name,
		Status: pickStr(sm, "status", "status_servico", "servico_status", "status_contrato"),
		Login:  pickStr(sm, "login_radius", "login", "usuario", "login_pppoe"),
		IPv4:   pickIPv4FromMap(sm),
	}
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
