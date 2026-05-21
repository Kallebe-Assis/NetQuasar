package integrationconsumer

// HubsoftAttendanceQueryOverrides parâmetros GET /integracao/cliente/atendimento.
func HubsoftAttendanceQueryOverrides(busca, termo, apenasPendente string) map[string]string {
	if apenasPendente == "" {
		apenasPendente = "nao"
	}
	return map[string]string{
		"busca":            busca,
		"termo_busca":      termo,
		"limit":            "50",
		"apenas_pendente":  apenasPendente,
		"order_by":         "data_cadastro",
		"order_type":       "desc",
	}
}
