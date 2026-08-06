package mikrotikcollect

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func collectTimeoutFromCtx(ctx context.Context, requested time.Duration) time.Duration {
	if requested <= 0 {
		requested = 45 * time.Second
	}
	if dl, ok := ctx.Deadline(); ok {
		rem := time.Until(dl) - 3*time.Second
		if rem < 8*time.Second {
			if rem < 5*time.Second {
				return 5 * time.Second
			}
			return rem
		}
		if rem < requested {
			return rem
		}
	}
	if requested > 90*time.Second {
		return 90 * time.Second
	}
	return requested
}

func mergeCollectOutputs(dst *CollectOutput, src CollectOutput) {
	if dst.Fields == nil {
		dst.Fields = make(map[string]FieldResult)
	}
	for k, v := range src.Fields {
		dst.Fields[k] = v
	}
	dst.Status.Enabled += src.Status.Enabled
	dst.Status.Collected += src.Status.Collected
	dst.Status.Failed += src.Status.Failed
	dst.Status.Skipped = append(dst.Status.Skipped, src.Status.Skipped...)
	dst.Status.MissingOID = append(dst.Status.MissingOID, src.Status.MissingOID...)
	if dst.Status.Message == "" && src.Status.Message != "" {
		dst.Status.Message = src.Status.Message
	}
}

// CollectAndStore executa coleta MikroTik (SNMP + telnet conforme perfil) e persiste em telemetry_samples.
// Inclui métricas de telemetria (CPU, memória, temperatura, uptime, …) e secção óptica/SFP habilitada.
func CollectAndStore(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (CollectOutput, TelnetCollectOutput, error) {
	timeout = collectTimeoutFromCtx(ctx, timeout)
	profile := LoadGlobalProfile(ctx, pool)

	telemBudget := time.Duration(float64(timeout) * 0.65)
	if telemBudget < 10*time.Second {
		telemBudget = timeout
	}
	telemetry := CollectMetrics(ctx, host, community, profile, CollectOpts{
		WalkTarget: TargetTelemetry,
		Timeout:    telemBudget,
	})

	opticalBudget := timeout - telemBudget
	if opticalBudget < 8*time.Second {
		opticalBudget = 8 * time.Second
	}
	if ctx.Err() == nil {
		optical := CollectMetrics(ctx, host, community, profile, CollectOpts{
			WalkTarget: TargetInterfaces,
			Sections:   []string{"optical"},
			Timeout:    opticalBudget,
		})
		mergeCollectOutputs(&telemetry, optical)
	}

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
