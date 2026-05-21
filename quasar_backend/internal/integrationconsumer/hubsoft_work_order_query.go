package integrationconsumer

// HubsoftWorkOrderQueryOverrides parâmetros GET /integracao/cliente/ordem_servico.
func HubsoftWorkOrderQueryOverrides(busca, termo string) map[string]string {
	return map[string]string{
		"busca":               busca,
		"termo_busca":         termo,
		"limit":               "50",
		"order_by":            "data_cadastro",
		"order_type":          "desc",
		"exibir_atendimento":  "true",
	}
}
