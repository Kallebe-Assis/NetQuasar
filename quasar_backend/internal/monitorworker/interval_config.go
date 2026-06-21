package monitorworker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

type intervalConfig struct {
	PingTimeoutMs          int
	TelemetryTimeoutMs     int
	InterfaceTimeoutMs     int
	OltIfDerivedTimeoutMs  int
	ICMPPayloadBytes       int
	OfflineThreshold       int
	PingSeconds            int
	TelemetrySeconds       int
	IfaceSeconds           int
	OltDerivedSeconds      int
	PipelineCycleSeconds   int
	MikrotikTimeoutMs      int
	PingParallel           bool
}

// ResolveTelemetrySeconds devolve segundos de telemetria a usar (evita COALESCE em SQL por compatibilidade).
func ResolveTelemetrySeconds(telemetrySeconds, telemetryMinutes int) int {
	if telemetrySeconds >= 60 {
		return telemetrySeconds
	}
	tm := telemetryMinutes
	if tm < 2 {
		tm = 2
	}
	sec := tm * 60
	if sec < 60 {
		return 120
	}
	return sec
}

func loadClampMonitoringIntervals(ctx context.Context, pool *pgxpool.Pool) (intervalConfig, error) {
	var c intervalConfig
	var telSecRaw, telMin int
	if err := pool.QueryRow(ctx, `
		SELECT ping_timeout_ms, icmp_payload_bytes, offline_ping_fail_threshold,
			ping_seconds,
			telemetry_seconds, telemetry_minutes,
			interface_snapshot_seconds, olt_if_derived_pon_seconds,
			telemetry_timeout_ms, interface_snapshot_timeout_ms, olt_if_derived_pon_timeout_ms,
			pipeline_cycle_seconds, mikrotik_timeout_ms,
			COALESCE(ping_parallel, true)
		FROM monitoring_intervals WHERE id=1
	`).Scan(&c.PingTimeoutMs, &c.ICMPPayloadBytes, &c.OfflineThreshold, &c.PingSeconds,
		&telSecRaw, &telMin, &c.IfaceSeconds, &c.OltDerivedSeconds,
		&c.TelemetryTimeoutMs, &c.InterfaceTimeoutMs, &c.OltIfDerivedTimeoutMs,
		&c.PipelineCycleSeconds, &c.MikrotikTimeoutMs, &c.PingParallel); err != nil {
		return intervalConfig{}, err
	}
	c.TelemetrySeconds = ResolveTelemetrySeconds(telSecRaw, telMin)

	if c.TelemetrySeconds < 60 {
		c.TelemetrySeconds = 120
	}
	if c.IfaceSeconds < 60 {
		c.IfaceSeconds = 180
	}
	if c.OltDerivedSeconds < 60 {
		c.OltDerivedSeconds = 240
	}
	if c.PipelineCycleSeconds < 30 {
		c.PipelineCycleSeconds = 120
	}
	if c.PingSeconds < 30 {
		c.PingSeconds = 30
	}
	if c.MikrotikTimeoutMs < 5000 {
		c.MikrotikTimeoutMs = 120000
	}
	if c.PingTimeoutMs < 1000 {
		c.PingTimeoutMs = 1000
	}
	if c.PingTimeoutMs > 30000 {
		c.PingTimeoutMs = 30000
	}
	if c.OfflineThreshold < 1 {
		c.OfflineThreshold = 3
	}
	if c.OfflineThreshold > 50 {
		c.OfflineThreshold = 50
	}
	c.ICMPPayloadBytes = probing.ClampICMPPayloadBytes(c.ICMPPayloadBytes)
	return c, nil
}
