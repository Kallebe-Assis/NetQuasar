package integrationconsumer

import "strings"

// HubsoftBuscaValue traduz o «busca» genérico da UI (ver DefaultClientSearchBusca e
// IsLoginBusca) para o vocabulário documentado da Hubsoft (GET /integracao/cliente, ver
// BuscaOptions em config.go). Hoje só «login» (usado por omissão nas consultas de login — o
// mesmo valor genérico que o ramo IXC reconhece via IsLoginBusca) precisa de tradução: a Hubsoft
// não aceita "login" (devolve "O tipo de busca (login) não é válido"), só "login_radius". Os
// restantes valores (nome_razaosocial, cpf_cnpj, …) já vêm directamente do vocabulário Hubsoft e
// passam inalterados.
func HubsoftBuscaValue(busca string) string {
	if strings.ToLower(strings.TrimSpace(busca)) == "login" {
		return "login_radius"
	}
	return busca
}

// HubsoftSearchQueryOverrides parâmetros GET /integracao/cliente para modo resumido ou detalhado.
func HubsoftSearchQueryOverrides(detailed bool) map[string]string {
	if detailed {
		return map[string]string{
			"inativo":              "todos",
			"limit":                "100",
			"cancelado":            "sim",
			"ultima_conexao":       "sim",
			"incluir_alarmes":      "sim",
			"incluir_contrato":     "sim",
			"incluir_stfc":         "sim",
			"incluir_mvno":         "sim",
			"incluir_anexos":       "sim",
			"incluir_desbloqueios": "sim",
			"order_by":             "data_cadastro",
			"order_type":           "desc",
		}
	}
	return map[string]string{
		"inativo":              "todos",
		"limit":                "20",
		"cancelado":            "nao",
		"ultima_conexao":       "sim",
		"incluir_alarmes":      "nao",
		"incluir_contrato":     "sim",
		"incluir_stfc":         "nao",
		"incluir_mvno":         "nao",
		"incluir_anexos":       "nao",
		"incluir_desbloqueios": "nao",
		"order_by":             "data_cadastro",
		"order_type":           "desc",
	}
}
