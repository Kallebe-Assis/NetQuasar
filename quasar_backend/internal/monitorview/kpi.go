package monitorview

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmetrics"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

type metricsProfile struct {
	CPUPrimaryOID   string `json:"cpu_primary_oid"`
	CPUAvailableOID string `json:"cpu_available_oid"`
	MemoryUsedOID   string `json:"memory_used_oid"`
	MemorySizeOID   string `json:"memory_size_oid"`
	TempPrimaryOID  string `json:"temp_primary_oid"`
	UptimeOID       string `json:"uptime_oid"`
}

// ExtractDeviceKPIs extrai KPIs compactos de metrics JSON (telemetria) e opcionalmente detail do probe cache.
func ExtractDeviceKPIs(metricsJSON, detailJSON []byte, collectedAt *time.Time) DeviceKPIs {
	vars := snmpVarsFromMetricsOrDetail(metricsJSON, detailJSON)
	prof := profileFromMetrics(metricsJSON)
	cpu, mem, uptime, temp := extractExtendedMetrics(vars, prof)
	mergeMikrotikKPIs(metricsJSON, &cpu, &mem, &temp, &uptime)

	out := DeviceKPIs{
		CPUPercent:    cpu,
		MemoryPercent: mem,
		TemperatureC:  temp,
		Uptime:        uptime,
	}
	if collectedAt != nil && !collectedAt.IsZero() {
		out.CollectedAt = collectedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// KPIsDetailPatch devolve JSON para merge em device_probe_cache.detail.
func KPIsDetailPatch(kpis DeviceKPIs) []byte {
	b, _ := json.Marshal(map[string]any{"monitor_kpis": kpis})
	return b
}

func snmpVarsMapFromSNMPBlock(block map[string]any) map[string]string {
	out := make(map[string]string)
	if block == nil {
		return out
	}
	rawVars, ok := block["vars"].([]any)
	if !ok {
		return out
	}
	for _, item := range rawVars {
		vm, _ := item.(map[string]any)
		if vm == nil {
			continue
		}
		oid := cleanOID(anyString(vm["oid"]))
		val := strings.TrimSpace(anyString(vm["value"]))
		if oid != "" {
			out[oid] = val
		}
	}
	return out
}

func anyString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func snmpVarsFromProbeDetail(detailJSON []byte, into map[string]string) {
	if len(detailJSON) == 0 || into == nil {
		return
	}
	var root map[string]any
	if json.Unmarshal(detailJSON, &root) != nil {
		return
	}
	if snmp, ok := root["snmp"].(map[string]any); ok {
		for k, v := range snmpVarsMapFromSNMPBlock(snmp) {
			into[k] = v
		}
	}
}

func snmpVarsFromMetricsOrDetail(metricsJSON, detailJSON []byte) map[string]string {
	merged := make(map[string]string)
	snmpVarsFromProbeDetail(detailJSON, merged)
	for k, v := range snmpVarsFromMetrics(metricsJSON) {
		merged[k] = v
	}
	return merged
}

func snmpVarsFromMetrics(b []byte) map[string]string {
	out := make(map[string]string)
	if len(b) == 0 {
		return out
	}
	var env struct {
		SNMP *struct {
			Vars []struct {
				OID   string `json:"oid"`
				Value string `json:"value"`
			} `json:"vars"`
		} `json:"snmp"`
	}
	if json.Unmarshal(b, &env) != nil || env.SNMP == nil {
		return out
	}
	for _, v := range env.SNMP.Vars {
		oid := cleanOID(v.OID)
		if oid == "" {
			continue
		}
		out[oid] = strings.TrimSpace(v.Value)
	}
	return out
}

func profileFromMetrics(b []byte) metricsProfile {
	var out metricsProfile
	if len(b) == 0 {
		return out
	}
	var env struct {
		Profile metricsProfile `json:"profile"`
	}
	if json.Unmarshal(b, &env) != nil {
		return out
	}
	return env.Profile
}

func cleanOID(v string) string {
	v = strings.TrimSpace(v)
	return strings.TrimLeft(v, ".")
}

func extractExtendedMetrics(vars map[string]string, prof metricsProfile) (cpu *float64, mem *float64, uptime string, temp *float64) {
	var uptimeTicks string
	const storDescr = "1.3.6.1.2.1.25.2.3.1.3."
	const storType = "1.3.6.1.2.1.25.2.3.1.2."
	const storUsed = "1.3.6.1.2.1.25.2.3.1.6."
	const storSize = "1.3.6.1.2.1.25.2.3.1.5."
	const storRAMTypeOID = "1.3.6.1.2.1.25.2.1.2"

	if oid := cleanOID(prof.CPUPrimaryOID); oid != "" {
		if v, ok := vars[oid]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				if oid == "1.3.6.1.4.1.2021.11.11.0" {
					cpu = ptrFloat(100 - f)
				} else {
					if oid == "1.3.6.1.4.1.14988.1.1.3.10.0" && f > 100 {
						f = f / 10.0
					}
					if f >= 0 && f <= 10000 {
						cpu = ptrFloat(f)
					}
				}
			}
		}
	}
	if cpu == nil {
		if oid := cleanOID(prof.CPUAvailableOID); oid != "" {
			if v, ok := vars[oid]; ok {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					cpu = cpuUsedFromAvailableOID(oid, f)
				}
			}
		}
	}
	if oid := cleanOID(prof.TempPrimaryOID); oid != "" {
		if v, ok := vars[oid]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				f = snmpmetrics.NormalizeAmbientTempCelsius(f)
				if f > -273 && f < 500 {
					temp = ptrFloat(f)
				}
			}
		}
	}
	if oid := cleanOID(prof.UptimeOID); oid != "" {
		if v, ok := vars[oid]; ok {
			uptimeTicks = strings.TrimSpace(v)
		}
	}
	if uptimeTicks == "" {
		if v, ok := vars["1.3.6.1.2.1.1.3.0"]; ok {
			uptimeTicks = strings.TrimSpace(v)
		}
	}
	if mem == nil {
		if oid := cleanOID(prof.MemoryUsedOID); oid != "" {
			if usedStr, ok := vars[oid]; ok {
				sizeOID := cleanOID(prof.MemorySizeOID)
				if sizeStr, ok2 := vars[sizeOID]; ok2 {
					used, err1 := strconv.ParseFloat(strings.TrimSpace(usedStr), 64)
					size, err2 := strconv.ParseFloat(strings.TrimSpace(sizeStr), 64)
					if err1 == nil && err2 == nil && size > 0 {
						pct := 100.0 * used / size
						if pct >= 0 && pct <= 100.0001 {
							mem = ptrFloat(pct)
						}
					}
				}
			}
		}
	}
	if mem == nil {
		for idx, descr := range vars {
			if !strings.HasPrefix(idx, storDescr) {
				continue
			}
			suffix := strings.TrimPrefix(idx, storDescr)
			if !strings.Contains(strings.ToLower(descr), "memory") && !strings.Contains(strings.ToLower(descr), "ram") {
				continue
			}
			typeOID := storType + suffix
			if vars[typeOID] != storRAMTypeOID {
				continue
			}
			usedStr := vars[storUsed+suffix]
			sizeStr := vars[storSize+suffix]
			used, err1 := strconv.ParseFloat(strings.TrimSpace(usedStr), 64)
			size, err2 := strconv.ParseFloat(strings.TrimSpace(sizeStr), 64)
			if err1 == nil && err2 == nil && size > 0 {
				pct := 100.0 * used / size
				if pct >= 0 && pct <= 100.0001 {
					mem = ptrFloat(pct)
					break
				}
			}
		}
	}
	uptime = vsolparse.FormatUptimeDisplay(uptimeTicks)
	if uptime == "" {
		for oid, val := range vars {
			if vsolparse.IsVsolUptimeOID(oid) {
				if u := vsolparse.FormatUptimeDisplay(val); u != "" {
					uptime = u
					break
				}
			}
		}
	}
	if uptime == "" {
		uptime = "—"
	}
	return cpu, mem, uptime, temp
}

func cpuUsedFromAvailableOID(oid string, f float64) *float64 {
	const ucdIdle = "1.3.6.1.4.1.2021.11.11.0"
	const mikLoad = "1.3.6.1.4.1.14988.1.1.3.10.0"
	const hrProc = "1.3.6.1.2.1.25.3.3.1.2"
	switch {
	case oid == ucdIdle && f >= 0 && f <= 100:
		return ptrFloat(100 - f)
	case oid == mikLoad:
		if f > 100 {
			f /= 10
		}
		if f >= 0 && f <= 100 {
			return ptrFloat(f)
		}
	case strings.HasPrefix(oid, hrProc+"."):
		if f >= 0 && f <= 10000 {
			return ptrFloat(f)
		}
	case oid == hrProc:
		if f >= 0 && f <= 10000 {
			return ptrFloat(f)
		}
	default:
		idle := f
		if strings.HasPrefix(oid, "1.3.6.1.4.1.14988") && idle > 100 && idle <= 1000 {
			idle /= 10
		}
		if idle >= 0 && idle <= 100 {
			return ptrFloat(100 - idle)
		}
	}
	return nil
}

func ptrFloat(f float64) *float64 { return &f }

func mikrotikFieldFloat(fields map[string]any, key string) *float64 {
	fr, _ := fields[key].(map[string]any)
	if fr == nil {
		return nil
	}
	ok, _ := fr["ok"].(bool)
	if !ok {
		return nil
	}
	v := fr["value"]
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	case string:
		p, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return nil
		}
		f = p
	default:
		return nil
	}
	if f < -273 || f > 10000 || (f != f) {
		return nil
	}
	return ptrFloat(f)
}

func mikrotikFieldString(fields map[string]any, key string) string {
	fr, _ := fields[key].(map[string]any)
	if fr == nil {
		return ""
	}
	ok, _ := fr["ok"].(bool)
	if !ok {
		return ""
	}
	return strings.TrimSpace(anyString(fr["value"]))
}

func mergeMikrotikKPIs(metricsJSON []byte, cpu, mem, temp **float64, uptime *string) {
	if len(metricsJSON) == 0 {
		return
	}
	var root map[string]any
	if json.Unmarshal(metricsJSON, &root) != nil {
		return
	}
	mk, _ := root["mikrotik_collection"].(map[string]any)
	if mk == nil {
		return
	}
	fields, _ := mk["fields"].(map[string]any)
	if fields == nil {
		return
	}
	if *cpu == nil {
		if v := mikrotikFieldFloat(fields, "cpu_load"); v != nil {
			f := *v
			if f > 100 {
				f /= 10
			}
			if f >= 0 && f <= 100 {
				*cpu = ptrFloat(f)
			}
		}
		if *cpu == nil {
			*cpu = mikrotikFieldFloat(fields, "cpu_hr")
		}
	}
	if *mem == nil {
		used := mikrotikFieldFloat(fields, "memory_used")
		total := mikrotikFieldFloat(fields, "memory_total")
		if used != nil && total != nil && *total > 0 {
			pct := 100.0 * (*used) / (*total)
			if pct >= 0 && pct <= 100.0001 {
				*mem = ptrFloat(pct)
			}
		}
	}
	if *temp == nil {
		for _, k := range []string{"temperature", "board_temperature", "cpu_temperature"} {
			if v := mikrotikFieldFloat(fields, k); v != nil {
				n := snmpmetrics.NormalizeAmbientTempCelsius(*v)
				if n > -273 && n < 500 {
					*temp = ptrFloat(n)
					break
				}
			}
		}
	}
	if uptime != nil && (*uptime == "" || *uptime == "—") {
		if u := mikrotikFieldString(fields, "sys_uptime"); u != "" {
			if formatted := vsolparse.FormatUptimeDisplay(u); formatted != "" {
				*uptime = formatted
			} else {
				*uptime = u
			}
		}
	}
}

// KPIsFromProbeDetail lê KPIs em cache no detail do probe (evita join em telemetry_samples).
func KPIsFromProbeDetail(detailJSON []byte) (DeviceKPIs, bool) {
	if len(detailJSON) == 0 {
		return DeviceKPIs{}, false
	}
	var root map[string]any
	if json.Unmarshal(detailJSON, &root) != nil {
		return DeviceKPIs{}, false
	}
	raw, ok := root["monitor_kpis"]
	if !ok || raw == nil {
		return DeviceKPIs{}, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return DeviceKPIs{}, false
	}
	var kpis DeviceKPIs
	if json.Unmarshal(b, &kpis) != nil {
		return DeviceKPIs{}, false
	}
	return kpis, true
}
