package monitorworker

import "time"

const (
	defaultTelemetryTimeoutMs    = 120_000
	defaultInterfaceTimeoutMs    = 120_000
	defaultOltIfDerivedTimeoutMs = 180_000
	defaultOltOnuTelnetTimeoutMs = 600_000
	minCollectionTimeoutMs       = 5_000
	maxCollectionTimeoutMs       = 600_000
	maxOltOnuTelnetTimeoutMs     = 3_600_000
	// maxOltIfDerivedTimeoutMs — tecto próprio da coleta SNMP de PON/ONU (olt_if_derived_pon_
	// timeout_ms), bem mais alto que o tecto genérico (maxCollectionTimeoutMs, 10 min) partilhado
	// por telemetria/interfaces/MikroTik/BNG. Confirmado em produção: com OLTs de centenas/milhares
	// de ONUs em 8+ PONs, 10 min cortava a varredura a meio (só as primeiras ~5 PONs completas) —
	// o contexto expirava e o resto simplesmente não era coletado, sem erro visível. 1 h alinha
	// com o tecto já generoso da fase telnet (maxOltOnuTelnetTimeoutMs).
	maxOltIfDerivedTimeoutMs = 3_600_000
)

// ClampCollectionTimeoutMsPublic expõe o clamp de timeouts de coleta para o pacote api.
func ClampCollectionTimeoutMsPublic(ms, defaultMs int) int {
	return clampCollectionTimeoutMs(ms, defaultMs)
}

// ClampOltOnuTelnetTimeoutMsPublic limita o timeout da fase telnet ONU/PON (até 1 h).
func ClampOltOnuTelnetTimeoutMsPublic(ms, defaultMs int) int {
	if ms < minCollectionTimeoutMs {
		return defaultMs
	}
	if ms > maxOltOnuTelnetTimeoutMs {
		return maxOltOnuTelnetTimeoutMs
	}
	return ms
}

// ClampOltIfDerivedTimeoutMsPublic limita o timeout da coleta SNMP de PON/ONU (olt_if_derived_
// pon_timeout_ms) — tecto próprio, bem mais alto que o genérico (ver maxOltIfDerivedTimeoutMs).
func ClampOltIfDerivedTimeoutMsPublic(ms, defaultMs int) int {
	if ms < minCollectionTimeoutMs {
		return defaultMs
	}
	if ms > maxOltIfDerivedTimeoutMs {
		return maxOltIfDerivedTimeoutMs
	}
	return ms
}

func clampCollectionTimeoutMs(ms, defaultMs int) int {
	if ms < minCollectionTimeoutMs {
		return defaultMs
	}
	if ms > maxCollectionTimeoutMs {
		return maxCollectionTimeoutMs
	}
	return ms
}

func (c intervalConfig) telemetryTimeout() time.Duration {
	return time.Duration(clampCollectionTimeoutMs(c.TelemetryTimeoutMs, defaultTelemetryTimeoutMs)) * time.Millisecond
}

func (c intervalConfig) mikrotikTimeout() time.Duration {
	return time.Duration(clampCollectionTimeoutMs(c.MikrotikTimeoutMs, defaultInterfaceTimeoutMs)) * time.Millisecond
}

func (c intervalConfig) bngTimeout() time.Duration {
	ms := c.BngTimeoutMs
	if ms < minCollectionTimeoutMs {
		ms = c.TelemetryTimeoutMs
	}
	return time.Duration(clampCollectionTimeoutMs(ms, defaultTelemetryTimeoutMs)) * time.Millisecond
}

func (c intervalConfig) interfaceTimeout(oltPhase bool, mikrotikPhase bool) time.Duration {
	if mikrotikPhase {
		return c.mikrotikTimeout()
	}
	ms := clampCollectionTimeoutMs(c.InterfaceTimeoutMs, defaultInterfaceTimeoutMs)
	if oltPhase && ms > 75_000 {
		ms = 75_000
	}
	return time.Duration(ms) * time.Millisecond
}

func (c intervalConfig) oltIfDerivedTimeout() time.Duration {
	return time.Duration(ClampOltIfDerivedTimeoutMsPublic(c.OltIfDerivedTimeoutMs, defaultOltIfDerivedTimeoutMs)) * time.Millisecond
}

func (c intervalConfig) oltOnuTelnetTimeout() time.Duration {
	return time.Duration(clampOltOnuTelnetTimeoutMs(c.OltOnuTelnetTimeoutMs, defaultOltOnuTelnetTimeoutMs)) * time.Millisecond
}

func clampOltOnuTelnetTimeoutMs(ms, defaultMs int) int {
	return ClampOltOnuTelnetTimeoutMsPublic(ms, defaultMs)
}
