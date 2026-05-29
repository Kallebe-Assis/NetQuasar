package monitorworker

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmetrics"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
	"github.com/rs/zerolog"
)

func normalizeTelemetryOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}

func mikrotikScalarFromMetrics(metrics map[string]any, fieldKey string) *float64 {
	if metrics == nil {
		return nil
	}
	raw, ok := metrics["mikrotik_collection"]
	if !ok || raw == nil {
		return nil
	}
	doc, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	fields, ok := doc["fields"].(map[string]any)
	if !ok {
		return nil
	}
	fr, ok := fields[fieldKey].(map[string]any)
	if !ok {
		return nil
	}
	okVal, _ := fr["ok"].(bool)
	if !okVal {
		return nil
	}
	val := fr["value"]
	switch x := val.(type) {
	case float64:
		return &x
	case int:
		f := float64(x)
		return &f
	case int64:
		f := float64(x)
		return &f
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", "."), 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func parseTempCFromTelemetry(metrics map[string]any, vars []probing.SNMPVar) *float64 {
	if f := mikrotikScalarFromMetrics(metrics, "temperature"); f != nil {
		v := snmpmetrics.NormalizeAmbientTempCelsius(*f)
		if v > -273 && v < 500 {
			return &v
		}
	}
	if metrics == nil {
		return nil
	}
	prof, ok := metrics["profile"].(map[string]any)
	if !ok || prof == nil {
		return nil
	}
	raw, ok := prof["temp_primary_oid"]
	if !ok || raw == nil {
		return nil
	}
	want, _ := raw.(string)
	want = normalizeTelemetryOID(want)
	if want == "" {
		return nil
	}
	for _, v := range vars {
		if normalizeTelemetryOID(v.OID) == want {
			f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v.Value), ",", "."), 64)
			if err != nil {
				return nil
			}
			f = snmpmetrics.NormalizeAmbientTempCelsius(f)
			if f > -273 && f < 500 {
				return &f
			}
			return nil
		}
	}
	return nil
}

func telemetryVarsByOID(vars []probing.SNMPVar) map[string]string {
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		oid := normalizeTelemetryOID(v.OID)
		if oid == "" {
			continue
		}
		out[oid] = strings.TrimSpace(v.Value)
	}
	return out
}

func profileOID(metrics map[string]any, key string) string {
	if metrics == nil {
		return ""
	}
	prof, ok := metrics["profile"].(map[string]any)
	if !ok || prof == nil {
		return ""
	}
	raw, ok := prof[key]
	if !ok || raw == nil {
		return ""
	}
	s, _ := raw.(string)
	return normalizeTelemetryOID(s)
}

func parseFloatOID(vars map[string]string, oid string) *float64 {
	if oid == "" {
		return nil
	}
	v, ok := vars[oid]
	if !ok {
		return nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", "."), 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseCPUFromTelemetry(metrics map[string]any, vars []probing.SNMPVar) *float64 {
	if f := mikrotikScalarFromMetrics(metrics, "cpu_load"); f != nil {
		v := *f
		if v > 100 {
			v = v / 10.0
		}
		if v >= 0 && v <= 1000 {
			return &v
		}
	}
	if f := mikrotikScalarFromMetrics(metrics, "cpu_hr"); f != nil {
		return f
	}
	byOID := telemetryVarsByOID(vars)
	primary := profileOID(metrics, "cpu_primary_oid")
	if f := parseFloatOID(byOID, primary); f != nil {
		v := *f
		if primary == "1.3.6.1.4.1.2021.11.11.0" { // idle -> used
			v = 100 - v
		}
		if primary == "1.3.6.1.4.1.14988.1.1.3.10.0" && v > 100 {
			v = v / 10.0
		}
		if v >= 0 && v <= 1000 {
			return &v
		}
	}
	availOID := profileOID(metrics, "cpu_available_oid")
	if f := parseFloatOID(byOID, availOID); f != nil {
		v := *f
		if v >= 0 && v <= 100 {
			used := 100 - v
			return &used
		}
	}
	return nil
}

func parseMemoryFromTelemetry(metrics map[string]any, vars []probing.SNMPVar) *float64 {
	used := mikrotikScalarFromMetrics(metrics, "memory_used")
	size := mikrotikScalarFromMetrics(metrics, "memory_total")
	if used != nil && size != nil && *size > 0 {
		pct := 100.0 * (*used) / (*size)
		return &pct
	}
	byOID := telemetryVarsByOID(vars)
	usedOID := profileOID(metrics, "memory_used_oid")
	sizeOID := profileOID(metrics, "memory_size_oid")
	usedVal := parseFloatOID(byOID, usedOID)
	sizeVal := parseFloatOID(byOID, sizeOID)
	if usedVal == nil || sizeVal == nil || *sizeVal <= 0 {
		return nil
	}
	u := *usedVal
	sz := *sizeVal
	// memAvailReal (disponível) vira utilizado.
	if usedOID == "1.3.6.1.4.1.2021.4.6.0" && sz >= u {
		u = sz - u
	}
	if u < 0 || u > sz {
		return nil
	}
	pct := 100.0 * u / sz
	return &pct
}

func parseUptimeMinutesFromTelemetry(metrics map[string]any, vars []probing.SNMPVar) *float64 {
	if f := mikrotikScalarFromMetrics(metrics, "sys_uptime"); f != nil {
		min := (*f / 100.0) / 60.0
		return &min
	}
	byOID := telemetryVarsByOID(vars)
	uOID := profileOID(metrics, "uptime_oid")
	if f := parseFloatOID(byOID, uOID); f != nil {
		min := (*f / 100.0) / 60.0 // sysUpTime ticks (centésimos)
		return &min
	}
	for oid, v := range byOID {
		if oid == "1.3.6.1.2.1.1.3.0" {
			f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err == nil {
				min := (f / 100.0) / 60.0
				return &min
			}
			break
		}
	}
	return nil
}

func likelyMikrotikDevice(category, brand, model, description string) bool {
	hay := strings.ToLower(strings.TrimSpace(strings.Join([]string{category, brand, model, description}, " ")))
	if hay == "" {
		return false
	}
	if strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") {
		return true
	}
	return strings.Contains(hay, "ccr") || strings.Contains(hay, "crs") || strings.Contains(hay, "rb")
}

func copyFloat64Ptr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// evaluateMikrotikSfpAfterMonitoringTelemetry corre walks leves (IF-MIB + mtxr) e aplica limiares SFP globais.
func evaluateMikrotikSfpAfterMonitoringTelemetry(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, deviceDesc, host, community, category, brand, model string,
) {
	if !likelyMikrotikDevice(category, brand, model, deviceDesc) {
		return
	}
	h := strings.TrimSpace(host)
	c := strings.TrimSpace(community)
	if h == "" || c == "" {
		return
	}
	walkMk, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: h, Port: 161, Community: c, RootOID: snmpmikrotik.DefaultOpticalWalkRoot,
		Version: "2c", Timeout: 18 * time.Second, Retries: 0, MaxRows: 8000,
	})
	walkMkIf, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: h, Port: 161, Community: c, RootOID: snmpmikrotik.DefaultInterfaceStatsNameWalkRoot,
		Version: "2c", Timeout: 14 * time.Second, Retries: 0, MaxRows: 3000,
	})
	walkIF, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: h, Port: 161, Community: c, RootOID: "1.3.6.1.2.1.2.2.1",
		Version: "2c", Timeout: 28 * time.Second, Retries: 0, MaxRows: 10000,
	})
	merged := append(append(append([]probing.SNMPVar{}, walkIF...), walkMk...), walkMkIf...)
	if len(merged) == 0 {
		return
	}
	ifRows := snmpifparse.BuildIfTable(merged)
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, merged)
	sfpEval := make([]alertthresholds.SfpInterfaceRow, 0, len(ifRows))
	for _, r := range ifRows {
		op := optMap[r.IfIndex]
		disp := strings.TrimSpace(r.DisplayName)
		if disp == "" {
			disp = "if" + strconv.Itoa(r.IfIndex)
		}
		sfpEval = append(sfpEval, alertthresholds.SfpInterfaceRow{
			IfIndex:     r.IfIndex,
			DisplayName: disp,
			Sfp:         snmpmikrotik.IsSfpPort(r.DisplayName, r.Descr, op),
			TxDBm:       copyFloat64Ptr(op.TxDBm),
			RxDBm:       copyFloat64Ptr(op.RxDBm),
		})
	}
	alertthresholds.EvaluateMikrotikSFPAfterSnapshot(ctx, pool, log, deviceID, strings.TrimSpace(deviceDesc), h, sfpEval)
}

// RunPostTelemetryAlertEval aplica limiares globais (ex.: temperatura) e SFP MikroTik após uma amostra de telemetria.
func RunPostTelemetryAlertEval(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, deviceDesc, host, community string,
	category, brand, model string,
	col telemetryengine.CollectResult,
) {
	hasVars := len(col.SNMP.Vars) > 0
	_, hasMk := col.Metrics["mikrotik_collection"]
	if !hasVars && !hasMk {
		return
	}
	if c := parseCPUFromTelemetry(col.Metrics, col.SNMP.Vars); c != nil {
		alertthresholds.EvaluateGlobalGteMetric(ctx, pool, log, deviceID, deviceDesc, host, "cpu_usage_pct", *c)
	}
	if m := parseMemoryFromTelemetry(col.Metrics, col.SNMP.Vars); m != nil {
		alertthresholds.EvaluateGlobalGteMetric(ctx, pool, log, deviceID, deviceDesc, host, "memory_usage_pct", *m)
	}
	if t := parseTempCFromTelemetry(col.Metrics, col.SNMP.Vars); t != nil {
		alertthresholds.EvaluateGlobalGteMetric(ctx, pool, log, deviceID, deviceDesc, host, "temperature_c", *t)
	}
	if u := parseUptimeMinutesFromTelemetry(col.Metrics, col.SNMP.Vars); u != nil {
		alertthresholds.EvaluateGlobalGteMetric(ctx, pool, log, deviceID, deviceDesc, host, "uptime_minutes", *u)
	}
	evaluateMikrotikSfpAfterMonitoringTelemetry(ctx, pool, log, deviceID, deviceDesc, host, community, category, brand, model)
}
