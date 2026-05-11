package snmpmetrics

// NormalizeAmbientTempCelsius converte leituras SNMP frequentes para °C reais.
// Muitos agentes (ex.: MikroTik, ENTITY-SENSOR em décimos) devolvem temperatura ×10 (ex.: 350 → 35 °C).
// Alinhado com extractExtendedMetrics em monitoramento activo.
func NormalizeAmbientTempCelsius(read float64) float64 {
	if read > 100 {
		return read / 10.0
	}
	return read
}
