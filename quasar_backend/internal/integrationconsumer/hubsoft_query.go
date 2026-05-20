package integrationconsumer

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
		"incluir_contrato":     "nao",
		"incluir_stfc":         "nao",
		"incluir_mvno":         "nao",
		"incluir_anexos":       "nao",
		"incluir_desbloqueios": "nao",
		"order_by":             "data_cadastro",
		"order_type":           "desc",
	}
}
