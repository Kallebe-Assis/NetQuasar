package monitorworker

import "time"

const (
	defaultTelemetryTimeoutMs      = 120_000
	defaultInterfaceTimeoutMs      = 120_000
	defaultOltIfDerivedTimeoutMs   = 180_000
	defaultOltOnuTelnetTimeoutMs   = 600_000
	minCollectionTimeoutMs         = 5_000
	maxCollectionTimeoutMs         = 600_000
	maxOltOnuTelnetTimeoutMs       = 3_600_000
)

// ClampCollectionTimeoutMsPublic expõe o clamp de timeouts de coleta para o pacote api.
func ClampCollectionTimeoutMsPublic(ms, defaultMs int) int {
	return clampCollectionTimeoutMs(ms, defaultMs)
}

// ClampOltOnuTelnetTimeoutMsPublic limita o timeout da fase telnet ONU/PON (até 30 min).
func ClampOltOnuTelnetTimeoutMsPublic(ms, defaultMs int) int {
	if ms < minCollectionTimeoutMs {
		return defaultMs
	}
	if ms > maxOltOnuTelnetTimeoutMs {
		return maxOltOnuTelnetTimeoutMs
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
	return time.Duration(clampCollectionTimeoutMs(c.OltIfDerivedTimeoutMs, defaultOltIfDerivedTimeoutMs)) * time.Millisecond
}

func (c intervalConfig) oltOnuTelnetTimeout() time.Duration {
	return time.Duration(clampOltOnuTelnetTimeoutMs(c.OltOnuTelnetTimeoutMs, defaultOltOnuTelnetTimeoutMs)) * time.Millisecond
}

func clampOltOnuTelnetTimeoutMs(ms, defaultMs int) int {
	return ClampOltOnuTelnetTimeoutMsPublic(ms, defaultMs)
}
