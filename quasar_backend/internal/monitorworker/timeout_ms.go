package monitorworker

import "time"

const (
	defaultTelemetryTimeoutMs     = 120_000
	defaultInterfaceTimeoutMs     = 120_000
	defaultOltIfDerivedTimeoutMs  = 180_000
	minCollectionTimeoutMs        = 5_000
	maxCollectionTimeoutMs        = 600_000
)

// ClampCollectionTimeoutMsPublic expõe o clamp de timeouts de coleta para o pacote api.
func ClampCollectionTimeoutMsPublic(ms, defaultMs int) int {
	return clampCollectionTimeoutMs(ms, defaultMs)
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

func (c intervalConfig) interfaceTimeout(oltPhase bool) time.Duration {
	ms := clampCollectionTimeoutMs(c.InterfaceTimeoutMs, defaultInterfaceTimeoutMs)
	if oltPhase && ms > 75_000 {
		// Mantém limite prático por OLT na fase dedicada, sem ignorar totalmente a configuração.
		ms = 75_000
	}
	return time.Duration(ms) * time.Millisecond
}

func (c intervalConfig) oltIfDerivedTimeout() time.Duration {
	return time.Duration(clampCollectionTimeoutMs(c.OltIfDerivedTimeoutMs, defaultOltIfDerivedTimeoutMs)) * time.Millisecond
}
