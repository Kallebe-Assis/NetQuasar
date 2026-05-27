package api

import (
	"context"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
)

type collectionTimeouts struct {
	TelemetryMs      int
	InterfaceMs      int
	OltIfDerivedMs   int
}

func (s *Server) loadCollectionTimeouts(ctx context.Context) collectionTimeouts {
	out := collectionTimeouts{
		TelemetryMs:    120_000,
		InterfaceMs:    120_000,
		OltIfDerivedMs: 300_000,
	}
	pool := s.DB()
	if pool == nil {
		return out
	}
	_ = pool.QueryRow(ctx, `
		SELECT telemetry_timeout_ms, interface_snapshot_timeout_ms, olt_if_derived_pon_timeout_ms
		FROM monitoring_intervals WHERE id=1
	`).Scan(&out.TelemetryMs, &out.InterfaceMs, &out.OltIfDerivedMs)
	return out
}

func (t collectionTimeouts) OltRefreshTotal() time.Duration {
	ms := monitorworker.ClampCollectionTimeoutMsPublic(t.OltIfDerivedMs, 300_000)
	if tel := monitorworker.ClampCollectionTimeoutMsPublic(t.TelemetryMs, 120_000); tel > ms {
		ms = tel
	}
	return time.Duration(ms) * time.Millisecond
}

func (t collectionTimeouts) InterfaceRefreshTotal() time.Duration {
	ms := monitorworker.ClampCollectionTimeoutMsPublic(t.InterfaceMs, 120_000)
	return time.Duration(ms) * time.Millisecond
}

func (t collectionTimeouts) SnmpPerWalkTimeout(total time.Duration) time.Duration {
	if total <= 0 {
		return 50 * time.Second
	}
	w := time.Duration(float64(total) * 0.85)
	if w < 5*time.Second {
		return 5 * time.Second
	}
	if w > total {
		return total
	}
	return w
}

func (t collectionTimeouts) TelnetPhaseTimeout(total time.Duration) time.Duration {
	if total <= 0 {
		return 45 * time.Second
	}
	w := time.Duration(float64(total) * 0.65)
	if w < 10*time.Second {
		return 10 * time.Second
	}
	if w > total {
		return total
	}
	return w
}
