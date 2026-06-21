package monitorworker

const minConsecutivePingsForAlert = 3

// ConsecutivePingsRequired exige no mínimo 3 leituras consecutivas (latência alta ou falha ICMP/TCP)
// antes de abrir alerta — 2 falhas + 1 sucesso (ou 2 altas + 1 baixa) não disparam alarme.
func ConsecutivePingsRequired(configured int) int {
	if configured < minConsecutivePingsForAlert {
		return minConsecutivePingsForAlert
	}
	return configured
}

func consecutivePingsRequired(configured int) int {
	return ConsecutivePingsRequired(configured)
}

func (c intervalConfig) alertConsecutiveRequired() int {
	return consecutivePingsRequired(c.OfflineThreshold)
}
