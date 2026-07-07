package mikrotikcollect

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// CollectAndStore executa coleta MikroTik (SNMP + telnet conforme perfil) e persiste em telemetry_samples.
func CollectAndStore(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (CollectOutput, TelnetCollectOutput, error) {
	profile := LoadGlobalProfile(ctx, pool)
	telemetry := CollectMetrics(ctx, host, community, profile, CollectOpts{
		WalkTarget: TargetTelemetry,
		Timeout:    timeout,
	})
	var snmpVars []probing.SNMPVar
	for _, fr := range telemetry.Fields {
		if !fr.OK {
			continue
		}
		if fr.CollectMode == ModeSNMPGet && fr.Value != nil {
			snmpVars = append(snmpVars, probing.SNMPVar{OID: fr.OID, Value: formatSNMPValue(fr.Value)})
		}
	}
	telnetProfile := LoadTelnetProfileForDevice(ctx, pool, deviceID)
	telnetOut := TelnetCollectOutput{}
	if HasEnabledTelnetMetrics(telnetProfile.Metrics) {
		creds := LoadTelnetCredentialsForDevice(ctx, pool, deviceID)
		telnetTO := timeout * 3
		if telnetTO < 30*time.Second {
			telnetTO = 30 * time.Second
		}
		if telnetTO > 120*time.Second {
			telnetTO = 120 * time.Second
		}
		telnetOut = CollectTelnetMetrics(ctx, host, creds, telnetProfile, telnetTO)
	}
	b, err := BuildTelemetryMetricsJSON(telemetry, snmpVars, telnetOut)
	if err != nil {
		return telemetry, telnetOut, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return telemetry, telnetOut, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO telemetry_samples (device_id, collected_at, metrics)
			VALUES ($1, now(), $2::jsonb)
		`, deviceID, b)
		if err == nil {
			break
		}
	}
	return telemetry, telnetOut, err
}

// CollectInterfaceWalks devolve vars SNMP para interface_snapshots conforme perfil.
func CollectInterfaceWalks(ctx context.Context, host, community string, pool *pgxpool.Pool, total time.Duration) ([]probing.SNMPVar, bool) {
	profile := LoadGlobalProfile(ctx, pool)
	out := CollectMetrics(ctx, host, community, profile, CollectOpts{
		WalkTarget: TargetInterfaces,
		Timeout:    workerShareTimeout(total, 0.35, 12*time.Second, 90*time.Second),
	})
	var merged []probing.SNMPVar
	truncated := false
	for _, fr := range out.Fields {
		if fr.Walk == nil {
			continue
		}
		merged = append(merged, fr.Walk.Vars...)
		if fr.Walk.Truncated {
			truncated = true
		}
	}
	return merged, truncated
}

func workerShareTimeout(total time.Duration, frac float64, min, cap time.Duration) time.Duration {
	if total <= 0 {
		total = 120 * time.Second
	}
	w := time.Duration(float64(total) * frac)
	if w < min {
		return min
	}
	if w > cap {
		return cap
	}
	return w
}

func formatSNMPValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
