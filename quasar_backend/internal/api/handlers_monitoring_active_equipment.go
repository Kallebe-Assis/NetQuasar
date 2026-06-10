package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmetrics"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

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

func snmpVarsFromMetricsOrDetail(metricsJSON []byte, detailJSON []byte) map[string]string {
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

type metricsProfile struct {
	CPUPrimaryOID   string `json:"cpu_primary_oid"`
	CPUAvailableOID string `json:"cpu_available_oid"`
	MemoryUsedOID   string `json:"memory_used_oid"`
	MemorySizeOID   string `json:"memory_size_oid"`
	TempPrimaryOID  string `json:"temp_primary_oid"`
	UptimeOID       string `json:"uptime_oid"`
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

func formatSysUpTimeTicks(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ticks, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return ""
	}
	sec := ticks / 100
	const day = uint64(86400)
	const hour = uint64(3600)
	const min = uint64(60)
	d := sec / day
	sec %= day
	h := sec / hour
	sec %= hour
	m := sec / min
	s := sec % min
	if d > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}

func reachabilityPingOK(detailJSON []byte, cacheOK *bool) *bool {
	if len(strings.TrimSpace(string(detailJSON))) == 0 {
		return cacheOK
	}
	var root map[string]any
	if err := json.Unmarshal(detailJSON, &root); err != nil {
		return cacheOK
	}
	rel, ok := root["reachability"].(map[string]any)
	if !ok {
		return cacheOK
	}
	switch v := rel["ok"].(type) {
	case bool:
		b := v
		return &b
	default:
		return cacheOK
	}
}

func extractExtendedMetrics(vars map[string]string, prof metricsProfile) (cpu *float64, mem *float64, uptime string, temp *float64) {
	var uptimeTicks string
	const storDescr = "1.3.6.1.2.1.25.2.3.1.3."
	const storType = "1.3.6.1.2.1.25.2.3.1.2."
	const storUsed = "1.3.6.1.2.1.25.2.3.1.6."
	const storSize = "1.3.6.1.2.1.25.2.3.1.5."
	const storRAMTypeOID = "1.3.6.1.2.1.25.2.1.2"
	// Primeiro usa estritamente os OIDs definidos no profile da coleta.
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
	if u := cleanOID(prof.UptimeOID); u != "" {
		uptimeTicks = strings.TrimSpace(vars[u])
	}
	if mu, ms := cleanOID(prof.MemoryUsedOID), cleanOID(prof.MemorySizeOID); mu != "" && ms != "" {
		used, eU := strconv.ParseFloat(strings.TrimSpace(vars[mu]), 64)
		size, eS := strconv.ParseFloat(strings.TrimSpace(vars[ms]), 64)
		if eU == nil && eS == nil && size > 0 {
			if mu == "1.3.6.1.4.1.2021.4.6.0" {
				// memAvailReal
				if used >= 0 && size >= used {
					mem = ptrFloat(100.0 * (size - used) / size)
				}
			} else if used >= 0 && used <= size {
				mem = ptrFloat(100.0 * used / size)
			}
		}
	}
	descIdx := make(map[string]string)
	typeIdx := make(map[string]string)
	for oid, val := range vars {
		if strings.HasPrefix(oid, storDescr) {
			descIdx[strings.TrimPrefix(oid, storDescr)] = val
			continue
		}
		if strings.HasPrefix(oid, storType) {
			typeIdx[strings.TrimPrefix(oid, storType)] = val
		}
	}
	for idx, descr := range descIdx {
		ld := strings.ToLower(descr)
		if !(strings.Contains(ld, "physical memory") || strings.Contains(ld, "ram") || strings.Contains(ld, "real memory")) {
			continue
		}
		u := strings.TrimSpace(vars[storUsed+idx])
		sz := strings.TrimSpace(vars[storSize+idx])
		used, e1 := strconv.ParseFloat(u, 64)
		szF, e2 := strconv.ParseFloat(sz, 64)
		if e1 == nil && e2 == nil && szF > 0 && used >= 0 {
			pct := 100.0 * used / szF
			if pct >= 0 && pct <= 100.0001 {
				mem = ptrFloat(pct)
			}
		}
		break
	}
	if mem == nil {
		for idx, typ := range typeIdx {
			if strings.TrimSpace(typ) != storRAMTypeOID {
				continue
			}
			u := strings.TrimSpace(vars[storUsed+idx])
			sz := strings.TrimSpace(vars[storSize+idx])
			used, e1 := strconv.ParseFloat(u, 64)
			szF, e2 := strconv.ParseFloat(sz, 64)
			if e1 == nil && e2 == nil && szF > 0 && used >= 0 {
				pct := 100.0 * used / szF
				if pct >= 0 && pct <= 100.0001 {
					mem = ptrFloat(pct)
					break
				}
			}
		}
	}
	if mem == nil {
		total, eT := strconv.ParseFloat(strings.TrimSpace(vars["1.3.6.1.4.1.2021.4.5.0"]), 64)
		avail, eA := strconv.ParseFloat(strings.TrimSpace(vars["1.3.6.1.4.1.2021.4.6.0"]), 64)
		if eT == nil && eA == nil && total > 0 && avail >= 0 && total >= avail {
			pct := 100.0 * (total - avail) / total
			mem = ptrFloat(pct)
		}
	}
	if mem == nil {
		// Alguns equipamentos expõem total em hrMemorySize e disponível em UCD.
		total, eT := strconv.ParseFloat(strings.TrimSpace(vars["1.3.6.1.2.1.25.2.2.0"]), 64)
		avail, eA := strconv.ParseFloat(strings.TrimSpace(vars["1.3.6.1.4.1.2021.4.6.0"]), 64)
		if eT == nil && eA == nil && total > 0 && avail >= 0 && total >= avail {
			pct := 100.0 * (total - avail) / total
			mem = ptrFloat(pct)
		}
	}
	for oid, val := range vars {
		if strings.Contains(oid, "1.3.6.1.2.1.1.3.0") {
			uptimeTicks = val
			continue
		}
		if strings.Contains(oid, "1.3.6.1.2.1.25.3.3.1.2") && cpu == nil {
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f >= 0 && f <= 10000 {
				cpu = ptrFloat(f)
			}
			continue
		}
		if oid == "1.3.6.1.4.1.14988.1.1.3.10.0" && cpu == nil {
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f >= 0 {
				// MikroTik frequentemente expõe em décimos (ex.: 340 => 34.0%).
				if f > 100 {
					f = f / 10.0
				}
				if f >= 0 && f <= 100 {
					cpu = ptrFloat(f)
				}
			}
			continue
		}
		if oid == "1.3.6.1.2.1.99.1.1.1.4.1" && strings.Contains(strings.ToLower(val), "nosuchobject") {
			continue
		}
		if oid == "1.3.6.1.4.1.14988.1.1.3.14.0" {
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
				f = snmpmetrics.NormalizeAmbientTempCelsius(f)
				if f > -273 && f < 200 {
					temp = ptrFloat(f)
				}
			}
			continue
		}
		if oid == "1.3.6.1.4.1.2021.11.11.0" && cpu == nil {
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f >= 0 && f <= 100 {
				cpu = ptrFloat(100 - f)
			}
			continue
		}
		if strings.Contains(strings.ToLower(oid), "temperature") ||
			strings.HasPrefix(oid, "1.3.6.1.2.1.99.1.1.1.4.") ||
			strings.HasPrefix(oid, "1.3.6.1.4.1.9.9.13.1.3.1.3.") ||
			oid == "1.3.6.1.4.1.14988.1.1.3.14.0" {
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
				f = snmpmetrics.NormalizeAmbientTempCelsius(f)
				if f > -273 && f < 500 {
					temp = ptrFloat(f)
				}
			}
		}
	}
	// Não usamos hrMemorySize como "used" porque ele representa capacidade total.
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

// cpuUsedFromAvailableOID interpreta o OID configurado em «CPU disponível»: normalmente % idle (0–100)
// e devolve % de utilização. OIDs conhecidos de carga (hrProcessorLoad, MikroTik load) tratam-se como uso direto.
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

// mergeMikrotikKPIs preenche CPU/memória/temperatura/uptime a partir de mikrotik_collection quando o parse SNMP genérico falha.
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

// GET /monitoring/active-equipment
// Lista equipamentos com rede Normal (não Bridge), ping ligado e operação Ativo; inclui última sondagem + última telemetria para campos SNMP conhecidos.
func (s *Server) monitoringActiveEquipment(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.description, d.category::text,
			COALESCE(NULLIF(TRIM(BOTH FROM d.brand), ''), '')::text,
			host(d.ip)::text,
			c.checked_at, c.latency_ms, c.ok, c.reach_ok, COALESCE(c.ping_fail_streak, 0),
			c.detail::text,
			tel.metrics::text AS telemetry_metrics,
			tel.collected_at AS telemetry_collected_at
		FROM devices d
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		LEFT JOIN LATERAL (
			SELECT metrics, collected_at
			FROM telemetry_samples ts
			WHERE ts.device_id = d.id
			ORDER BY ts.collected_at DESC
			LIMIT 1
		) tel ON true
		WHERE TRIM(BOTH FROM COALESCE(d.network_status, '')) = 'Normal'
			AND COALESCE(TRIM(BOTH FROM d.network_status), '') <> ''
			AND d.ping_enabled = true
			AND TRIM(BOTH FROM COALESCE(d.operational_mode, '')) = 'Ativo'
			AND d.ip IS NOT NULL
			AND TRIM(BOTH FROM host(d.ip)::text) <> ''
		ORDER BY d.description
		LIMIT 600
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, cat, brand, ip string
		var ca any
		var lat *int64
		var probeOK *bool
		var reachOK *bool
		var pingFailStreak int
		var detail *string
		var metrics *string
		var telemetryAt *time.Time

		if err := rows.Scan(&id, &desc, &cat, &brand, &ip, &ca, &lat, &probeOK, &reachOK, &pingFailStreak, &detail, &metrics, &telemetryAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}

		var detB, metB []byte
		if detail != nil {
			detB = []byte(*detail)
		}
		if metrics != nil {
			metB = []byte(*metrics)
		}
		vars := snmpVarsFromMetricsOrDetail(metB, detB)
		prof := profileFromMetrics(metB)
		cpu, mem, uptime, temp := extractExtendedMetrics(vars, prof)
		mergeMikrotikKPIs(metB, &cpu, &mem, &temp, &uptime)

		row := map[string]any{
			"id":               id,
			"description":      desc,
			"category":         cat,
			"brand":            brand,
			"ip":               ip,
			"checked_at":       ca,
			"latency_ms":       nil,
			"probe_ok":         probeOK,
			"ping_fail_streak": pingFailStreak,
			"cpu_percent":      cpu,
			"memory_percent":   mem,
			"uptime":           uptime,
			"temperature_c":    temp,
			"ping_reachable":   nil,
		}
		if reachOK != nil {
			row["ping_reachable"] = *reachOK
		}
		if lat != nil {
			row["latency_ms"] = *lat
		}
		if telemetryAt != nil {
			row["telemetry_collected_at"] = telemetryAt.UTC().Format(time.RFC3339)
		}
		list = append(list, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}
