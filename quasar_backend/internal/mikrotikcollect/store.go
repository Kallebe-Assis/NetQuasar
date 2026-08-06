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

func persistTelemetrySample(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, telemetry CollectOutput, snmpVars []probing.SNMPVar, telnetOut TelnetCollectOutput) error {
	b, err := BuildTelemetryMetricsJSON(telemetry, snmpVars, telnetOut)
	if err != nil {
		return err
	}
	storeCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		storeCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-storeCtx.Done():
				return storeCtx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
		_, lastErr = pool.Exec(storeCtx, `
			INSERT INTO telemetry_samples (device_id, collected_at, metrics)
			VALUES ($1, now(), $2::jsonb)
		`, deviceID, b)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func snmpVarsFromGETFields(fields map[string]FieldResult) []probing.SNMPVar {
	var snmpVars []probing.SNMPVar
	for _, fr := range fields {
		if !fr.OK || fr.CollectMode != ModeSNMPGet || fr.Value == nil {
			continue
		}
		snmpVars = append(snmpVars, probing.SNMPVar{OID: fr.OID, Value: formatSNMPValue(fr.Value)})
	}
	return snmpVars
}

func remainingBudget(ctx context.Context, minKeep time.Duration) time.Duration {
	dl, ok := ctx.Deadline()
	if !ok {
		return 30 * time.Second
	}
	rem := time.Until(dl) - minKeep
	if rem < 0 {
		return 0
	}
	return rem
}

// HealthSections secções usadas no ciclo rápido de KPIs (CPU, memória, temperatura, uptime).
var HealthSections = []string{"health", "system"}

// CollectHealthAndStore coleta só GETs de saúde/sistema, persiste e devolve rápido.
// Usado pelo ciclo paralelo de monitoramento — não faz walks ópticos/PPPoE/telnet.
func CollectHealthAndStore(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (CollectOutput, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	timeout = collectTimeoutFromCtx(ctx, timeout)
	if timeout > 25*time.Second {
		timeout = 25 * time.Second
	}
	profile := LoadGlobalProfile(ctx, pool)
	telemetry := CollectMetrics(ctx, host, community, profile, CollectOpts{
		WalkTarget:  TargetTelemetry,
		Sections:    HealthSections,
		ScalarsOnly: true,
		Timeout:     timeout,
	})
	snmpVars := snmpVarsFromGETFields(telemetry.Fields)
	err := persistTelemetrySample(ctx, pool, deviceID, telemetry, snmpVars, TelnetCollectOutput{})
	return telemetry, err
}

// CollectAndStore executa coleta MikroTik (SNMP + telnet conforme perfil) e persiste em telemetry_samples.
// Persiste KPIs de saúde (CPU/mem/temp/uptime) assim que a fase de telemetria termina,
// antes de walks ópticos/telnet que possam consumir o deadline do worker.
func CollectAndStore(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (CollectOutput, TelnetCollectOutput, error) {
	timeout = collectTimeoutFromCtx(ctx, timeout)
	profile := LoadGlobalProfile(ctx, pool)

	telemBudget := time.Duration(float64(timeout) * 0.55)
	if telemBudget < 12*time.Second {
		telemBudget = timeout
		if telemBudget > 25*time.Second {
			telemBudget = 25 * time.Second
		}
	}
	telemetry := CollectMetrics(ctx, host, community, profile, CollectOpts{
		WalkTarget: TargetTelemetry,
		Timeout:    telemBudget,
	})
	snmpVars := snmpVarsFromGETFields(telemetry.Fields)
	telnetOut := TelnetCollectOutput{}

	// Grava amostra mínima de KPIs imediatamente — o ciclo de monitoramento depende disto.
	persistErr := persistTelemetrySample(ctx, pool, deviceID, telemetry, snmpVars, telnetOut)

	opticalBudget := remainingBudget(ctx, 8*time.Second)
	if opticalBudget >= 8*time.Second && ctx.Err() == nil {
		if opticalBudget > 35*time.Second {
			opticalBudget = 35 * time.Second
		}
		optical := CollectMetrics(ctx, host, community, profile, CollectOpts{
			WalkTarget: TargetInterfaces,
			Sections:   []string{"optical"},
			Timeout:    opticalBudget,
		})
		mergeCollectOutputs(&telemetry, optical)
		snmpVars = snmpVarsFromGETFields(telemetry.Fields)
	}

	telnetProfile := LoadTelnetProfileForDevice(ctx, pool, deviceID)
	if HasEnabledTelnetMetrics(telnetProfile.Metrics) && remainingBudget(ctx, 5*time.Second) >= 15*time.Second {
		creds := LoadTelnetCredentialsForDevice(ctx, pool, deviceID)
		telnetTO := remainingBudget(ctx, 5*time.Second)
		if telnetTO > 60*time.Second {
			telnetTO = 60 * time.Second
		}
		if telnetTO < 20*time.Second {
			telnetTO = 20 * time.Second
		}
		telnetOut = CollectTelnetMetrics(ctx, host, creds, telnetProfile, telnetTO)
	}

	// Regrava com óptica/telnet se houver dados extra; se falhar, mantém a amostra mínima.
	if err := persistTelemetrySample(ctx, pool, deviceID, telemetry, snmpVars, telnetOut); err != nil && persistErr == nil {
		persistErr = err
	}
	return telemetry, telnetOut, persistErr
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
