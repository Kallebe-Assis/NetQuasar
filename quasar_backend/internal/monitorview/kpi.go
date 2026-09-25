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
	mem = memoryPercentFromVars(vars, prof)
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

const (
	hrStorDescr   = "1.3.6.1.2.1.25.2.3.1.3."
	hrStorType    = "1.3.6.1.2.1.25.2.3.1.2."
	hrStorSize    = "1.3.6.1.2.1.25.2.3.1.5."
	hrStorUsed    = "1.3.6.1.2.1.25.2.3.1.6."
	hrStorRAMType = "1.3.6.1.2.1.25.2.1.2"
	ucdMemTotal   = "1.3.6.1.4.1.2021.4.5.0"
	ucdMemAvail   = "1.3.6.1.4.1.2021.4.6.0"
	hrMemorySize  = "1.3.6.1.2.1.25.2.2.0"
)

func snmpFloat(vars map[string]string, oid string) (float64, bool) {
	v, ok := vars[oid]
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f, err == nil
}

func isHrStorageRAM(v string) bool {
	v = strings.TrimSpace(v)
	return cleanOID(v) == hrStorRAMType || strings.EqualFold(v, "hrStorageRam") || strings.HasSuffix(cleanOID(v), "25.2.1.2")
}

// memoryPercentFromVars devolve a % de memória usada a partir das variáveis SNMP, tentando em ordem:
//  1. OIDs "usado/tamanho" do perfil do equipamento (UCD memAvailReal conta como "disponível");
//  2. hrStorage (RAM) — por descrição ("physical/real memory", "ram", "memory") ou pelo tipo hrStorageRam;
//  3. UCD-SNMP (memTotalReal e memAvailReal) e hrMemorySize + memAvailReal.
func memoryPercentFromVars(vars map[string]string, prof metricsProfile) *float64 {
	pct := func(used, size float64) *float64 {
		if size > 0 && used >= 0 {
			if p := 100.0 * used / size; p <= 100.0001 {
				return ptrFloat(p)
			}
		}
		return nil
	}
	// Só o OID "usado" configurado (sem tamanho): perfis como o da OLT VSOL expõem a % de memória direto.
	if mu := cleanOID(prof.MemoryUsedOID); mu != "" && cleanOID(prof.MemorySizeOID) == "" {
		if v, ok := snmpFloat(vars, mu); ok && v >= 0 && v <= 100 {
			return ptrFloat(v)
		}
	}
	if mu, ms := cleanOID(prof.MemoryUsedOID), cleanOID(prof.MemorySizeOID); mu != "" && ms != "" {
		used, okU := snmpFloat(vars, mu)
		size, okS := snmpFloat(vars, ms)
		if okU && okS && size > 0 {
			if mu == ucdMemAvail { // memAvailReal: o valor é o que sobra
				if used >= 0 && size >= used {
					return ptrFloat(100.0 * (size - used) / size)
				}
			} else if p := pct(used, size); p != nil {
				return p
			}
		}
	}

	descr := map[string]string{}
	typ := map[string]string{}
	for oid, val := range vars {
		switch {
		case strings.HasPrefix(oid, hrStorDescr):
			descr[strings.TrimPrefix(oid, hrStorDescr)] = val
		case strings.HasPrefix(oid, hrStorType):
			typ[strings.TrimPrefix(oid, hrStorType)] = val
		}
	}
	tryIdx := func(idx string) *float64 {
		used, okU := snmpFloat(vars, hrStorUsed+idx)
		size, okS := snmpFloat(vars, hrStorSize+idx)
		if okU && okS {
			return pct(used, size)
		}
		return nil
	}
	for idx, d := range descr {
		ld := strings.ToLower(d)
		if strings.Contains(ld, "physical memory") || strings.Contains(ld, "real memory") || strings.Contains(ld, "main memory") ||
			ld == "ram" || strings.HasPrefix(ld, "ram ") || strings.Contains(ld, "memory") && isHrStorageRAM(typ[idx]) {
			if p := tryIdx(idx); p != nil {
				return p
			}
		}
	}
	for idx, t := range typ {
		if isHrStorageRAM(t) {
			if p := tryIdx(idx); p != nil {
				return p
			}
		}
	}

	if total, ok := snmpFloat(vars, ucdMemTotal); ok {
		if avail, ok2 := snmpFloat(vars, ucdMemAvail); ok2 && total > 0 && avail >= 0 && total >= avail {
			return ptrFloat(100.0 * (total - avail) / total)
		}
	}
	if total, ok := snmpFloat(vars, hrMemorySize); ok {
		if avail, ok2 := snmpFloat(vars, ucdMemAvail); ok2 && total > 0 && avail >= 0 && total >= avail {
			return ptrFloat(100.0 * (total - avail) / total)
		}
	}
	return nil
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

// mikrotikFieldBytes lê um campo numérico SEM o teto de 10000 de mikrotikFieldFloat — memória vem em
// bytes/KiB (centenas de milhões), e esse teto descartava sempre memory_used/memory_total.
func mikrotikFieldBytes(fields map[string]any, key string) *float64 {
	fr, _ := fields[key].(map[string]any)
	if fr == nil {
		return nil
	}
	if ok, _ := fr["ok"].(bool); !ok {
		return nil
	}
	var f float64
	switch x := fr["value"].(type) {
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
	if f != f || f < 0 {
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
		used := mikrotikFieldBytes(fields, "memory_used")
		total := mikrotikFieldBytes(fields, "memory_total")
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
