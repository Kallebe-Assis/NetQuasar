package switchcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// CollectAndStore executa coleta SNMP/telnet para Switch e persiste em telemetry_samples.
func CollectAndStore(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (mikrotikcollect.CollectOutput, mikrotikcollect.TelnetCollectOutput, error) {
	profile := LoadGlobalProfile(ctx, pool)
	telemetry := mikrotikcollect.CollectMetrics(ctx, host, community, profile, mikrotikcollect.CollectOpts{
		WalkTarget: mikrotikcollect.TargetTelemetry,
		Timeout:    timeout,
	})
	var snmpVars []probing.SNMPVar
	for _, fr := range telemetry.Fields {
		if !fr.OK {
			continue
		}
		if fr.CollectMode == mikrotikcollect.ModeSNMPGet && fr.Value != nil {
			snmpVars = append(snmpVars, probing.SNMPVar{OID: fr.OID, Value: formatSNMPValue(fr.Value)})
		}
	}
	telnetProfile := LoadTelnetProfileForDevice(ctx, pool, deviceID)
	telnetOut := mikrotikcollect.TelnetCollectOutput{}
	if HasEnabledTelnetMetrics(telnetProfile.Metrics) {
		creds := mikrotikcollect.LoadTelnetCredentialsForDevice(ctx, pool, deviceID)
		telnetTO := timeout * 3
		if telnetTO < 30*time.Second {
			telnetTO = 30 * time.Second
		}
		if telnetTO > 120*time.Second {
			telnetTO = 120 * time.Second
		}
		telnetOut = mikrotikcollect.CollectTelnetMetricsWithCatalog(ctx, host, creds, telnetProfile, telnetTO, TelnetMetricCatalog)
	}
	b, err := buildTelemetryMetricsJSON(telemetry, snmpVars, telnetOut)
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

// CollectInterfaceWalks devolve vars SNMP para interface_snapshots.
func CollectInterfaceWalks(ctx context.Context, host, community string, pool *pgxpool.Pool, total time.Duration) ([]probing.SNMPVar, bool) {
	profile := LoadGlobalProfile(ctx, pool)
	out := mikrotikcollect.CollectMetrics(ctx, host, community, profile, mikrotikcollect.CollectOpts{
		WalkTarget: mikrotikcollect.TargetInterfaces,
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

func buildTelemetryMetricsJSON(collect mikrotikcollect.CollectOutput, snmpVars []probing.SNMPVar, telnet mikrotikcollect.TelnetCollectOutput) ([]byte, error) {
	doc := map[string]any{
		"switch_collection": collect,
		"snmp": map[string]any{
			"vars": snmpVars,
		},
	}
	if telnet.ProfileID != "" || telnet.Collected > 0 || telnet.Failed > 0 || telnet.Message != "" {
		doc["switch_telnet_collection"] = telnet
	}
	return json.Marshal(doc)
}

func formatSNMPValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func workerShareTimeout(total time.Duration, share float64, min, max time.Duration) time.Duration {
	if total <= 0 {
		return min
	}
	d := time.Duration(float64(total) * share)
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}
