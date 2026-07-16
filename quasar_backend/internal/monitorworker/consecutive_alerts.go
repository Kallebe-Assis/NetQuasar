package monitorworker

const minConsecutivePingsForAlert = 3
const minConsecutiveLatencyForAlert = 2

// ConsecutivePingsRequired exige no mínimo 3 leituras consecutivas de falha ICMP/TCP
// antes de abrir alerta de equipamento offline.
func ConsecutivePingsRequired(configured int) int {
	if configured < minConsecutivePingsForAlert {
		return minConsecutivePingsForAlert
	}
	return configured
}

// ConsecutiveLatencyRequired confirma latência alta em 2 coletas consecutivas
// (1.ª regista; 2.ª confirma com média ou descarta se normalizar) — independente do limiar de offline ICMP.
func ConsecutiveLatencyRequired(configured int) int {
	_ = configured
	return minConsecutiveLatencyForAlert
}

func consecutivePingsRequired(configured int) int {
	return ConsecutivePingsRequired(configured)
}

func consecutiveLatencyRequired(configured int) int {
	return ConsecutiveLatencyRequired(configured)
}

func (c intervalConfig) alertConsecutiveRequired() int {
	return consecutivePingsRequired(c.OfflineThreshold)
}

func (c intervalConfig) latencyConsecutiveRequired() int {
	return consecutiveLatencyRequired(c.OfflineThreshold)
}
