package monitorworker

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmetrics"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
	"github.com/rs/zerolog"
)

// normalizeTelemetryOID remove o ponto inicial opcional de OIDs SNMP.
func normalizeTelemetryOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}

// asFloat64 converte valores típicos de telemetria (float, int, string, json.Number) para float64.
func asFloat64(val any) (float64, bool) {
	switch x := val.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", "."), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// scalarFromCollectionBag lê um campo escalar de mikrotik_collection / switch_collection.
// Aceita CollectOutput em memória (struct) ou mapa após round-trip JSON.
func scalarFromCollectionBag(raw any, fieldKey string) *float64 {
	if raw == nil {
		return nil
	}
	switch doc := raw.(type) {
	case mikrotikcollect.CollectOutput:
		if v, ok := mikrotikcollect.ScalarFromFields(doc.Fields, fieldKey); ok {
			return &v
		}
		return nil
	case *mikrotikcollect.CollectOutput:
		if doc == nil {
			return nil
		}
		if v, ok := mikrotikcollect.ScalarFromFields(doc.Fields, fieldKey); ok {
			return &v
		}
		return nil
	case map[string]any:
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
		if f, ok := asFloat64(fr["value"]); ok {
			return &f
		}
		return nil
	default:
		// Fallback: serializar e tratar como mapa (ex.: tipos aninhados inesperados).
		b, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		return scalarFromCollectionBag(m, fieldKey)
	}
}

// collectionScalarFromMetrics procura o campo nos bags MikroTik/Switch (SNMP e Telnet).
func collectionScalarFromMetrics(metrics map[string]any, fieldKeys ...string) *float64 {
	if metrics == nil {
		return nil
	}
	bags := []string{
		"mikrotik_collection", "switch_collection",
		"mikrotik_telnet_collection", "switch_telnet_collection",
	}
	for _, key := range fieldKeys {
		for _, bag := range bags {
			if f := scalarFromCollectionBag(metrics[bag], key); f != nil {
				return f
			}
		}
	}
	return nil
}

// parseTempCFromTelemetry obtém temperatura em °C a partir da coleta MikroTik/Switch ou OID de perfil.
func parseTempCFromTelemetry(metrics map[string]any, vars []probing.SNMPVar) *float64 {
	// Preferência: temperature → board_temperature → cpu_temperature → telnet_sys_temperature.
	if f := collectionScalarFromMetrics(metrics, "temperature", "board_temperature", "cpu_temperature", "telnet_sys_temperature"); f != nil {
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
	if f := collectionScalarFromMetrics(metrics, "cpu_load", "cpu_hr"); f != nil {
		v := *f
		if v > 100 {
			v = v / 10.0
		}
		if v >= 0 && v <= 1000 {
			return &v
		}
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
	used := collectionScalarFromMetrics(metrics, "memory_used")
	size := collectionScalarFromMetrics(metrics, "memory_total", "memory_size")
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
	if f := collectionScalarFromMetrics(metrics, "sys_uptime"); f != nil {
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

func parseUptimeMinutesFromSNMP(sn probing.SNMPGetResult) *float64 {
	if m, ok := SnmpUptimeMinutes(sn); ok && m >= 0 {
		return &m
	}
	return nil
}

// RunPostTelemetryAlertEval aplica limiares globais (CPU, memória, temperatura, uptime)
// após telemetria SNMP — usado pelo worker de monitoramento e pelo refresh manual da API.
func RunPostTelemetryAlertEval(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, deviceDesc, host, community string,
	category, brand, model string,
	col telemetryengine.CollectResult,
) {
	_ = community
	_ = brand
	_ = model
	hasVars := len(col.SNMP.Vars) > 0
	hasCollection := false
	for _, bag := range []string{"mikrotik_collection", "switch_collection", "mikrotik_telnet_collection", "switch_telnet_collection"} {
		if _, ok := col.Metrics[bag]; ok {
			hasCollection = true
			break
		}
	}
	if !hasVars && !hasCollection {
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
	uptimeMin := parseUptimeMinutesFromTelemetry(col.Metrics, col.SNMP.Vars)
	if uptimeMin == nil {
		uptimeMin = parseUptimeMinutesFromSNMP(col.SNMP)
	}
	if uptimeMin != nil {
		_, _, hasGlobalUptime := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "uptime_minutes", category)
		if hasGlobalUptime {
			alertthresholds.EvaluateGlobalGteMetric(ctx, pool, log, deviceID, deviceDesc, host, "uptime_minutes", *uptimeMin)
		} else {
			evaluateUptimeRestartAlert(ctx, pool, log, deviceID, deviceDesc, host, *uptimeMin)
		}
	}
	if _, hasBng := col.Metrics["bng_collection"]; hasBng {
		alertthresholds.EvaluateBngSubscriberDropAlerts(ctx, pool, log, deviceID, deviceDesc, host, "monitoring_telemetry")
	}
}
