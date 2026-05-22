package integrationconsumer

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

const (
	ProviderAuto    = "auto"
	ProviderHubsoft = "hubsoft"
	ProviderIXC     = "ixc"
	ProviderGeneric = "generic"
)

// DetectClientSearchProfile infere o ERP a partir da configuração e da requisição HTTP.
// Requisições com formato IXC (POST /cliente + ixcsoft:listar) têm prioridade sobre «hubsoft» configurado por engano.
func DetectClientSearchProfile(configured string, rc integrationhttp.RequestConfig, baseURL string) string {
	configured = strings.ToLower(strings.TrimSpace(configured))
	if LooksLikeIXCRequest(rc, baseURL) {
		return ProviderIXC
	}
	switch configured {
	case ProviderHubsoft, ProviderIXC, ProviderGeneric:
		return configured
	}
	if looksLikeHubsoftRequest(rc) {
		return ProviderHubsoft
	}
	return ProviderGeneric
}

// LooksLikeIXCRequest indica webservice IXC (consulta deve usar POST listar, não query Hubsoft).
func LooksLikeIXCRequest(rc integrationhttp.RequestConfig, baseURL string) bool {
	base := strings.ToLower(baseURL)
	if strings.Contains(base, "ixc") || strings.Contains(base, "/webservice/") {
		return true
	}
	for k, v := range rc.Headers {
		if strings.EqualFold(k, "ixcsoft") && strings.TrimSpace(v) != "" {
			return true
		}
	}
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if strings.Contains(path, "/cliente") && !strings.Contains(path, "integracao") {
		return true
	}
	body := strings.ToLower(rc.BodyTemplate)
	if strings.Contains(body, "qtype") && strings.Contains(body, "sortname") {
		return true
	}
	return false
}

func looksLikeHubsoftRequest(rc integrationhttp.RequestConfig) bool {
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	return strings.Contains(path, "integracao/cliente")
}

// BuscaOptionsForProfile opções de «busca» na UI conforme o ERP.
func BuscaOptionsForProfile(profile string) []BuscaOption {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProviderIXC:
		return BuscaIXCOptions()
	case ProviderHubsoft:
		return BuscaOptions()
	default:
		return BuscaGenericOptions()
	}
}

// BuscaIXCOptions tipos de pesquisa IXC (corpo POST listar).
func BuscaIXCOptions() []BuscaOption {
	return []BuscaOption{
		{Value: "nome_razaosocial", Label: "Razão social"},
		{Value: "cpf_cnpj", Label: "CPF/CNPJ"},
		{Value: "nome_fantasia", Label: "Nome fantasia"},
		{Value: "codigo_cliente", Label: "ID / código cliente"},
		{Value: "telefone", Label: "Telefone"},
		{Value: "email", Label: "E-mail"},
		{Value: "login", Label: "Login PPPoE/RADIUS"},
	}
}

// BuscaGenericOptions opções genéricas (variáveis {{busca}} / {{termo_busca}} na requisição).
func BuscaGenericOptions() []BuscaOption {
	opts := BuscaOptions()
	out := append([]BuscaOption(nil), opts...)
	out = append(out, BuscaOption{Value: "custom", Label: "Personalizado (usa variáveis na requisição)"})
	return out
}

// ClientSearchVariables variáveis globais injectadas na execução da consulta.
func ClientSearchVariables(busca, termo string) map[string]string {
	qtype := IXCBuscaToQtype(busca)
	query := IXCTermoForQuery(busca, termo)
	return map[string]string{
		"busca":       busca,
		"termo_busca": termo,
		"query":       query,
		"qtype":       qtype,
	}
}
