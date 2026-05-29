package oltcollect

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// snmpIfIndexMin distingue índice IF-MIB (ex.: ZTE 285278465) de slot PON VSOL (1–64).
const snmpIfIndexMin = 4096

type ponIfRef struct {
	PonPort int
	Compact string
	Name    string
	IfIndex int
}

var retryableMetrics = map[string]bool{
	MetricRxPower:     true,
	MetricTxPower:     true,
	MetricPonRxPower:  true,
	MetricPonTxPower:  true,
	MetricPonVoltage:  true,
	MetricPonCurrent:  true,
	MetricPonTemp:     true,
	MetricTemperature: true,
}

func isPonOnlyMetric(key string) bool {
	return key == MetricPonStatus || key == MetricPonRxPower || key == MetricPonTxPower || key == MetricPonVoltage || key == MetricPonCurrent || key == MetricPonTemp
}

// CollectOnuMetrics executa snmpwalk em cada tabela OID configurada e agrega por PON/ONU.
// maxPons > 0 filtra portas PON ao limite cadastrado no equipamento (devices.max_pons).
func CollectOnuMetrics(ctx context.Context, host, community string, metrics OnuMetricsConfig, totalBudget time.Duration, maxPons int) (
	summary map[string]any, pons []map[string]any, walkLog []map[string]any, err error,
) {
	keys := metrics.EnabledMetrics()
	if len(keys) == 0 {
		return nil, nil, nil, fmt.Errorf("nenhuma MIB SNMP configurada para monitoramento deste modelo")
	}
	if totalBudget <= 0 {
		totalBudget = 100 * time.Second
	}
	weight := func(metric string) int {
		switch metric {
		case MetricRxPower, MetricTxPower, MetricPonRxPower, MetricPonTxPower, MetricPonVoltage, MetricPonCurrent, MetricPonTemp, MetricTemperature:
			return 2 // costumam ter mais linhas/latência; ganham mais budget
		default:
			return 1
		}
	}
	totalWeight := 0
	for _, k := range keys {
		totalWeight += weight(k)
	}
	if totalWeight <= 0 {
		totalWeight = len(keys)
	}

	type cell struct {
		pon, onu int
		value    string
		valueInt int
		hasInt   bool
	}
	byKey := map[string]*map[string]any{}
	getRow := func(pon, onu int) map[string]any {
		k := fmt.Sprintf("%d.%d", pon, onu)
		if byKey[k] == nil {
			row := map[string]any{"pon": pon, "onu": onu}
			byKey[k] = &row
		}
		return *byKey[k]
	}

	summary = map[string]any{
		"olt_collection_mode": "onu_metrics_collect",
		"onu_metrics_count":   len(keys),
	}
	walkLog = make([]map[string]any, 0, len(keys))
	t0All := time.Now()
	ponCountsByPon := map[int]ponCountPair{}
	ponOptical := map[int]map[string]any{}
	ponStatus := map[int]map[string]any{}
	mapDescrOID := "1.3.6.1.2.1.2.2.1.2"
	mapNameOID := "1.3.6.1.2.1.31.1.1.1.1"
	if ps, ok := metrics[MetricPonStatus]; ok && strings.EqualFold(ps.StatusMode, StatusModeIfMibIndex) {
		if o := strings.TrimSpace(ps.IfDescrOID); o != "" {
			mapDescrOID = o
		}
		if o := strings.TrimSpace(ps.IfNameOID); o != "" {
			mapNameOID = o
		}
	}
	ponByIfIndex := loadPonIfIndexMap(ctx, host, community, 20*time.Second, mapDescrOID, mapNameOID)
	onuPonByOnu := map[int]int{}
	if needsVsolOnuPonIndex(metrics) {
		onuPonByOnu = loadOnuPonIndexMap(ctx, host, community, 12*time.Second)
	}

	for _, key := range keys {
		def := metrics[key]
		perWalk := (totalBudget * time.Duration(weight(key))) / time.Duration(totalWeight)
		if perWalk < 12*time.Second {
			perWalk = 12 * time.Second
		}
		if perWalk > 75*time.Second {
			perWalk = 75 * time.Second
		}
		base := probing.NormalizeSNMPOID(def.OID)
		if key == MetricStatus && strings.EqualFold(def.StatusMode, StatusModeIfMibIndex) && strings.TrimSpace(base) == "" {
			base = probing.NormalizeSNMPOID(def.IfOperOID)
			if strings.TrimSpace(base) == "" {
				base = "1.3.6.1.2.1.2.2.1.8"
			}
		}
		if key == MetricStatus && strings.EqualFold(def.StatusMode, StatusModePonCounts) {
			base = probing.NormalizeSNMPOID(def.OnlineCountOID)
		}
		entry := map[string]any{"metric": key, "oid": base, "status": "ok"}
		t0 := time.Now()
		var vars []probing.SNMPVar
		var trunc bool
		var note string
		if key == MetricStatus && strings.EqualFold(def.StatusMode, StatusModePonCounts) {
			onBase := probing.NormalizeSNMPOID(def.OnlineCountOID)
			offBase := probing.NormalizeSNMPOID(def.OfflineCountOID)
			onVars, onTrunc, onNote := snmpWalkMetric(ctx, host, community, onBase, perWalk, key)
			offVars, offTrunc, offNote := snmpWalkMetric(ctx, host, community, offBase, perWalk, key)
			counts, matched := mergePonCountWalks(onBase, onVars, offBase, offVars)
			for pon, c := range counts {
				ponCountsByPon[pon] = c
			}
			entry["oid"] = onBase
			entry["offline_oid"] = offBase
			entry["matched_rows"] = matched
			entry["var_count"] = len(onVars) + len(offVars)
			if onTrunc || offTrunc {
				entry["truncated"] = true
			}
			entry["note"] = firstNonEmptyString(onNote, offNote)
			entry["elapsed_ms"] = time.Since(t0).Milliseconds()
			walkLog = append(walkLog, entry)
			continue
		}
		vars, trunc, note = snmpWalkMetric(ctx, host, community, base, perWalk, key)
		entry["elapsed_ms"] = time.Since(t0).Milliseconds()
		entry["var_count"] = len(vars)
		if trunc {
			entry["truncated"] = true
		}
		if note != "" {
			entry["note"] = note
		}
		if len(vars) == 0 {
			entry["status"] = "empty"
		}

		matched := 0
		if key == MetricStatus && strings.EqualFold(def.StatusMode, StatusModeIfMibIndex) {
			rows, statusMatches := collectStatusFromIfMib(ctx, host, community, perWalk, def, vars)
			for k, rowData := range rows {
				parts := strings.Split(k, ".")
				if len(parts) != 2 {
					continue
				}
				pon, errPon := strconv.Atoi(parts[0])
				onu, errOnu := strconv.Atoi(parts[1])
				if errPon != nil || errOnu != nil {
					continue
				}
				row := getRow(pon, onu)
				for rk, rv := range rowData {
					row[rk] = rv
				}
			}
			matched = statusMatches
			if matched == 0 {
				entry["note"] = firstNonEmptyString(fmt.Sprint(entry["note"]),
					"status ONU: nenhuma interface ONU no IF-MIB (ZTE C320 só tem gpon_olt-*). Use modo «Tabela PON/ONU» com OID enterprise .ifIndex.ONU")
			}
		} else if key == MetricStatus && strings.EqualFold(def.StatusMode, StatusModeRxPowerThreshold) {
			matched = collectStatusFromRxPowerVars(vars, base, def, ponByIfIndex, onuPonByOnu, getRow)
			if matched == 0 {
				entry["note"] = firstNonEmptyString(fmt.Sprint(entry["note"]),
					"status por RX: nenhuma linha .ifIndex.ONU — confira OID/divisor da tabela de potência RX")
			}
		} else if isPonOnlyMetric(key) {
			if key == MetricPonStatus {
				useIfMib := !strings.EqualFold(strings.TrimSpace(def.StatusMode), StatusModePonOnuSuffix)
				if useIfMib {
					rows, statusMatches := collectPonStatusFromIfMib(ctx, host, community, perWalk, def, vars)
					for pon, rowData := range rows {
						if ponStatus[pon] == nil {
							ponStatus[pon] = map[string]any{}
						}
						for rk, rv := range rowData {
							ponStatus[pon][rk] = rv
						}
						if ref, ok := ponByIfIndexForPort(ponByIfIndex, pon); ok {
							ponStatus[pon]["if_index"] = ref.IfIndex
							ponStatus[pon]["pon_compact"] = ref.Compact
							ponStatus[pon]["pon_name"] = ref.Name
						}
					}
					matched = statusMatches
					if matched == 0 {
						entry["note"] = firstNonEmptyString(fmt.Sprint(entry["note"]), "status PON sem correspondência por ifDescr")
					}
				} else {
					for _, v := range vars {
						ponKey, ok := ParsePonFromSuffix(base, v.OID, ponByIfIndex)
						if !ok {
							continue
						}
						pon, ok := resolvePonPortKey(ponKey, ponByIfIndex)
						if !ok {
							continue
						}
						matched++
						n, err := parseStatusInt(v.Value)
						if err != nil {
							continue
						}
						on := StatusIsOnline(n, def)
						st := "down"
						if on {
							st = "up"
						}
						if ponStatus[pon] == nil {
							ponStatus[pon] = map[string]any{}
						}
						ponStatus[pon]["pon_oper_status"] = n
						ponStatus[pon]["status"] = st
						if ref, ok := ponByIfIndexForPort(ponByIfIndex, pon); ok {
							ponStatus[pon]["if_index"] = ref.IfIndex
							ponStatus[pon]["pon_compact"] = ref.Compact
							ponStatus[pon]["pon_name"] = ref.Name
						}
					}
				}
				entry["matched_rows"] = matched
				walkLog = append(walkLog, entry)
				continue
			}
			for _, v := range vars {
				ponKey, ok := ParsePonFromSuffix(base, v.OID, ponByIfIndex)
				if !ok {
					continue
				}
				pon, ok := resolvePonPortKey(ponKey, ponByIfIndex)
				if !ok {
					continue
				}
				matched++
				if ponOptical[pon] == nil {
					ponOptical[pon] = map[string]any{}
				}
				val := strings.TrimSpace(v.Value)
				switch key {
				case MetricPonRxPower:
					if f, ok := parseOpticalDbm(val, def.ValueDivisor); ok {
						ponOptical[pon]["rx_dbm"] = f
					} else {
						ponOptical[pon]["rx_dbm_raw"] = val
					}
				case MetricPonTxPower:
					if f, ok := parseOpticalDbm(val, def.ValueDivisor); ok {
						ponOptical[pon]["tx_dbm"] = f
					} else {
						ponOptical[pon]["tx_dbm_raw"] = val
					}
				case MetricPonVoltage:
					if f, ok := parseScaledFloat(val, def.ValueDivisor); ok {
						ponOptical[pon]["voltage"] = f
					} else {
						ponOptical[pon]["voltage_raw"] = val
					}
				case MetricPonCurrent:
					if f, ok := parseScaledFloat(val, def.ValueDivisor); ok {
						ponOptical[pon]["current"] = f
					} else {
						ponOptical[pon]["current_raw"] = val
					}
				case MetricPonTemp:
					if f, ok := parseScaledFloat(val, def.ValueDivisor); ok {
						ponOptical[pon]["temperature"] = f
					} else {
						ponOptical[pon]["temperature_raw"] = val
					}
				}
			}
		} else {
			for _, v := range vars {
				pon, onu, ifIdx, ok := ParsePonOnuSuffixMapped(base, v.OID, ponByIfIndex, onuPonByOnu)
				if !ok {
					continue
				}
				matched++
				row := getRow(pon, onu)
				if ifIdx > 0 {
					row["if_index"] = ifIdx
					if ref, ok := ponByIfIndex[ifIdx]; ok {
						row["pon_compact"] = ref.Compact
						row["pon_name"] = ref.Name
					}
				}
				val := normalizeSnmpDisplayValue(v.Value)
				switch key {
				case MetricSerial:
					row["serial"] = val
				case MetricModel:
					row["model"] = val
				case MetricRxPower:
					if dbm, display, ok := parseOnuRxDbm(v.Value, def.ValueDivisor); ok {
						row["rx_dbm"] = dbm
						row["rx_pwr"] = display
					} else if display != "" {
						row["rx_pwr"] = display
					} else {
						row["rx_pwr"] = normalizeSnmpDisplayValue(v.Value)
					}
				case MetricTxPower:
					row["tx_pwr"] = val
				case MetricTemperature:
					row["temp"] = val
				case MetricVlan:
					row["vlan"] = val
				case MetricStatus:
					n, err := strconv.Atoi(val)
					if err == nil {
						row["onu_online_sta"] = n
						row["online"] = StatusIsOnline(n, def)
					} else {
						row["status_raw"] = val
					}
				}
			}
		}
		entry["matched_rows"] = matched
		walkLog = append(walkLog, entry)
	}

	if shouldApplyStatusFromRx(metrics) {
		applyStatusFromRxOnRows(byKey, rxThresholdDef(metrics))
	}

	onuRows := make([]map[string]any, 0, len(byKey))
	for _, ptr := range byKey {
		onuRows = append(onuRows, *ptr)
	}
	sort.Slice(onuRows, func(i, j int) bool {
		pi, _ := onuRows[i]["pon"].(int)
		pj, _ := onuRows[j]["pon"].(int)
		if pi != pj {
			return pi < pj
		}
		oi, _ := onuRows[i]["onu"].(int)
		oj, _ := onuRows[j]["onu"].(int)
		return oi < oj
	})

	online, offline := 0, 0
	for _, r := range onuRows {
		if on, ok := r["online"].(bool); ok {
			if on {
				online++
			} else {
				offline++
			}
		}
	}
	if len(ponCountsByPon) > 0 && len(onuRows) == 0 {
		for _, c := range ponCountsByPon {
			online += c.online
			offline += c.offline
		}
	}
	pons = BuildPonsFromOnuRows(onuRows, ponByIfIndex)
	if len(ponCountsByPon) > 0 {
		pons = mergePonsWithCountMaps(pons, ponCountsByPon)
	}
	pons = mergePonOpticalIntoPons(pons, ponOptical, ponByIfIndex)
	pons = mergePonStatusIntoPons(pons, ponStatus, ponByIfIndex)
	if maxPons > 0 {
		pons = oltifderive.FilterPonRowsByMaxSlots(pons, maxPons)
	}
	if len(ponCountsByPon) > 0 && len(onuRows) > 0 {
		online, offline = 0, 0
		for _, p := range pons {
			online += intFromAny(p["onu_online"])
			offline += intFromAny(p["onu_offline"])
		}
	}
	arr := make([]any, 0, len(onuRows))
	for _, r := range onuRows {
		arr = append(arr, r)
	}

	summary["vsol_onu_rows"] = arr
	summary["vsol_onu_count"] = len(onuRows)
	summary["vsol_onu_online"] = online
	summary["vsol_onu_offline"] = offline
	summary["onu_metrics_enabled_keys"] = onuMetricsEnabledKeys(metrics)
	if shouldApplyStatusFromRx(metrics) {
		summary["onu_status_by_rx"] = true
		summary["offline_rx_dbm"] = offlineRxDbmThreshold(rxThresholdDef(metrics))
	}
	summary["onu_metrics_walks"] = walkLog
	summary["onu_metrics_elapsed_ms"] = time.Since(t0All).Milliseconds()
	if len(onuRows) == 0 && len(ponCountsByPon) == 0 {
		summary["onu_metrics_note"] = "Nenhuma ONU encontrada nos walks SNMP — verifique OIDs, community e conectividade"
	}
	return summary, pons, walkLog, nil
}

type ponCountPair struct {
	online, offline int
}

func snmpWalkMetric(ctx context.Context, host, community, base string, budget time.Duration, metricKey string) ([]probing.SNMPVar, bool, string) {
	vars, trunc, note := doSNMPWalk(ctx, host, community, base, budget)
	if !needsMetricRetry(metricKey, trunc, note, len(vars)) {
		return vars, trunc, note
	}
	retryBudget := budget + budget/2
	if retryBudget > 90*time.Second {
		retryBudget = 90 * time.Second
	}
	vars2, trunc2, note2 := doSNMPWalk(ctx, host, community, base, retryBudget)
	if len(vars2) > len(vars) {
		if note2 != "" && !trunc2 {
			note = strings.TrimSpace(note + "; retry_ok")
		} else {
			note = note2
		}
		return vars2, trunc2, note
	}
	if note2 != "" {
		note = firstNonEmptyString(note, note2)
	}
	if trunc2 {
		trunc = true
	}
	return vars, trunc, note
}

func doSNMPWalk(ctx context.Context, host, community, base string, budget time.Duration) ([]probing.SNMPVar, bool, string) {
	wCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return probing.SNMPWalk(wCtx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: base,
		Version: "2c", Timeout: budget, Retries: 1, MaxRows: 25000,
	})
}

func needsMetricRetry(metricKey string, trunc bool, note string, rowCount int) bool {
	if !retryableMetrics[metricKey] {
		return false
	}
	if trunc {
		return true
	}
	n := strings.ToLower(note)
	if strings.Contains(n, "deadline") || strings.Contains(n, "timeout") {
		return true
	}
	return rowCount > 0 && rowCount < 50 && note != ""
}

func mergePonCountWalks(onBase string, onVars []probing.SNMPVar, offBase string, offVars []probing.SNMPVar) (map[int]ponCountPair, int) {
	out := map[int]ponCountPair{}
	for _, v := range onVars {
		pon, ok := ParsePonFromSuffix(onBase, v.OID, nil)
		if !ok {
			continue
		}
		n, err := parseGaugeInt(v.Value)
		if err != nil {
			continue
		}
		c := out[pon]
		c.online = n
		out[pon] = c
	}
	for _, v := range offVars {
		pon, ok := ParsePonFromSuffix(offBase, v.OID, nil)
		if !ok {
			continue
		}
		n, err := parseGaugeInt(v.Value)
		if err != nil {
			continue
		}
		c := out[pon]
		c.offline = n
		out[pon] = c
	}
	matched := 0
	for _, c := range out {
		if c.online > 0 || c.offline > 0 {
			matched++
		}
	}
	return out, matched
}

func mergePonsWithCountMaps(fromOnus []map[string]any, counts map[int]ponCountPair) []map[string]any {
	byPon := map[int]map[string]any{}
	for _, p := range fromOnus {
		pon := intFromAny(p["id"])
		if pon <= 0 {
			pon = intFromAny(p["pon"])
		}
		if pon <= 0 {
			continue
		}
		byPon[pon] = p
	}
	keys := make([]int, 0, len(counts))
	for pon := range counts {
		keys = append(keys, pon)
	}
	sort.Ints(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, pon := range keys {
		c := counts[pon]
		total := c.online + c.offline
		row, ok := byPon[pon]
		if !ok {
			id := fmt.Sprintf("%02d", pon)
			row = map[string]any{
				"id": id, "name": "GPON0/" + id,
				"status": "snmp_metrics", "source_slice": "onu_metrics_collect",
			}
		}
		row["onu_online"] = c.online
		row["onu_offline"] = c.offline
		row["onu_total"] = total
		row["status"] = "pon_online_offline"
		row["source_slice"] = "pon_online_offline_count"
		out = append(out, row)
	}
	if len(out) == 0 {
		return fromOnus
	}
	return out
}

// opticalDbmScale escolhe divisor efectivo (ZTE envia mili-dBm, ex. -23449 → -23,449 dBm).
func opticalDbmScale(configured int, raw float64) float64 {
	if configured > 1 {
		return float64(configured)
	}
	af := raw
	if af < 0 {
		af = -af
	}
	if af >= 8000 {
		return 1000
	}
	if af > 100 {
		return 100
	}
	return 1
}

func opticalDbmScaleForValue(configured int, raw float64, isDecimalString bool) float64 {
	if configured > 1 {
		return float64(configured)
	}
	// VSOL envia STRING "-15.60" já em dBm — não aplicar escala centi/milli.
	if isDecimalString {
		return 1
	}
	return opticalDbmScale(configured, raw)
}

func trimOpticalNumericSuffix(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if i := strings.IndexFunc(s, func(r rune) bool {
		return !(unicode.IsDigit(r) || r == '.' || r == '-' || r == '+')
	}); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// isPlausibleOnuRxDbm rejeita temperatura/índice (ex. 32 °C) erroneamente parseados como RX.
func isPlausibleOnuRxDbm(f float64) bool {
	if f == 0 {
		return true
	}
	return f < 0 && f >= -80
}

func parseOpticalDbm(raw string, divisor int) (float64, bool) {
	s := trimOpticalNumericSuffix(normalizeSnmpDisplayValue(sanitizeSNMPValue(raw)))
	if s == "" {
		return 0, false
	}
	isDecimalStr := strings.Contains(s, ".")
	normalizeDbm := func(f float64) float64 {
		return f / opticalDbmScaleForValue(divisor, f, isDecimalStr)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return normalizeDbm(f), true
	}
	if mm := reParenNumber.FindStringSubmatch(s); len(mm) == 2 {
		if f, err := strconv.ParseFloat(mm[1], 64); err == nil {
			return normalizeDbm(f), true
		}
	}
	return 0, false
}

func parseOnuRxDbm(raw string, divisor int) (dbm float64, display string, ok bool) {
	display = trimOpticalNumericSuffix(normalizeSnmpDisplayValue(sanitizeSNMPValue(raw)))
	if display == "" {
		return 0, "", false
	}
	f, parsed := parseOpticalDbm(raw, divisor)
	if parsed && isPlausibleOnuRxDbm(f) {
		return f, fmt.Sprintf("%.2f", f), true
	}
	return 0, display, false
}

func parseScaledFloat(raw string, divisor int) (float64, bool) {
	s := sanitizeSNMPValue(raw)
	if s == "" {
		return 0, false
	}
	normalize := func(f float64) float64 {
		if divisor > 1 {
			return f / float64(divisor)
		}
		return f
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return normalize(f), true
	}
	if mm := reParenNumber.FindStringSubmatch(s); len(mm) == 2 {
		if f, err := strconv.ParseFloat(mm[1], 64); err == nil {
			return normalize(f), true
		}
	}
	parts := reDigits.FindAllString(s, -1)
	if len(parts) == 0 {
		return 0, false
	}
	if f, err := strconv.ParseFloat(parts[len(parts)-1], 64); err == nil {
		return normalize(f), true
	}
	return 0, false
}

func resolvePonPortKey(ponKey int, ponByIfIndex map[int]ponIfRef) (int, bool) {
	if ponKey <= 0 {
		return 0, false
	}
	if ponKey >= snmpIfIndexMin {
		if ref, ok := ponByIfIndex[ponKey]; ok && ref.PonPort > 0 {
			return ref.PonPort, true
		}
		return 0, false
	}
	if len(ponByIfIndex) > 0 {
		for _, ref := range ponByIfIndex {
			if ref.PonPort == ponKey {
				return ponKey, true
			}
		}
		return 0, false
	}
	return ponKey, true
}

func mergePonStatusIntoPons(pons []map[string]any, statusByPon map[int]map[string]any, ponByIfIndex map[int]ponIfRef) []map[string]any {
	if len(statusByPon) == 0 {
		return pons
	}
	byPon := map[int]map[string]any{}
	byIf := map[int]map[string]any{}
	for _, p := range pons {
		pon := ponIndexFromRow(p)
		if pon > 0 {
			byPon[pon] = p
		}
		if ix := intFromAny(p["if_index"]); ix > 0 {
			byIf[ix] = p
		}
	}
	for pon, st := range statusByPon {
		row := byPon[pon]
		if row == nil {
			if ix := intFromAny(st["if_index"]); ix > 0 {
				row = byIf[ix]
			}
		}
		if row == nil {
			compact := strings.TrimSpace(anyString(st["pon_compact"]))
			if compact == "" {
				if ref, ok := ponByIfIndexForPort(ponByIfIndex, pon); ok {
					compact = ref.Compact
				}
			}
			if compact == "" && len(ponByIfIndex) > 0 {
				continue
			}
			id := fmt.Sprintf("%02d", pon)
			name := "GPON0/" + id
			if n := strings.TrimSpace(anyString(st["pon_name"])); n != "" {
				name = n
			} else if compact != "" {
				id = compact
				name = "PON-" + compact
			}
			row = map[string]any{
				"id": id, "name": name, "pon": pon,
				"onu_total": 0, "onu_online": 0, "onu_offline": 0,
				"source_slice": "onu_metrics_collect",
			}
			if compact != "" {
				row["pon_compact"] = compact
			}
			pons = append(pons, row)
			byPon[pon] = row
			if ix := intFromAny(st["if_index"]); ix > 0 {
				byIf[ix] = row
			}
		}
		if v, ok := st["pon_oper_status"]; ok {
			row["pon_oper_status"] = v
		}
		if v, ok := st["pon_oper_status_raw"]; ok {
			row["pon_oper_status_raw"] = v
		}
		if v, ok := st["if_oper_status"]; ok {
			row["if_oper_status"] = v
		}
		if v, ok := st["status"]; ok {
			row["status"] = v
		}
		if v, ok := st["if_index"]; ok {
			row["if_index"] = v
		}
		if v, ok := st["pon_compact"]; ok {
			row["pon_compact"] = v
		}
		if v, ok := st["pon_name"]; ok {
			row["pon_name"] = v
		}
	}
	sort.Slice(pons, func(i, j int) bool {
		return ponIndexFromRow(pons[i]) < ponIndexFromRow(pons[j])
	})
	return pons
}

func mergePonOpticalIntoPons(pons []map[string]any, optical map[int]map[string]any, ponByIfIndex map[int]ponIfRef) []map[string]any {
	if len(optical) == 0 {
		return pons
	}
	byPon := map[int]map[string]any{}
	byIf := map[int]map[string]any{}
	for _, p := range pons {
		pon := ponIndexFromRow(p)
		if pon > 0 {
			byPon[pon] = p
		}
		if ix := intFromAny(p["if_index"]); ix > 0 {
			byIf[ix] = p
		}
	}
	for ponKey, opt := range optical {
		pon, ok := resolvePonPortKey(ponKey, ponByIfIndex)
		if !ok {
			continue
		}
		row := byPon[pon]
		if row == nil {
			if ix := ponKey; ix >= snmpIfIndexMin {
				row = byIf[ix]
			}
		}
		if row == nil {
			if len(ponByIfIndex) > 0 {
				continue
			}
			id := fmt.Sprintf("%02d", pon)
			name := "GPON0/" + id
			row = map[string]any{
				"id": id, "name": name, "pon": pon,
				"onu_total": 0, "onu_online": 0, "onu_offline": 0,
				"status": "snmp_metrics", "source_slice": "onu_metrics_collect",
			}
			pons = append(pons, row)
			byPon[pon] = row
		}
		if v, ok := opt["rx_dbm"]; ok && v != nil {
			row["rx_dbm"] = v
		}
		if v, ok := opt["tx_dbm"]; ok && v != nil {
			row["tx_dbm"] = v
		}
		if v, ok := opt["voltage"]; ok && v != nil {
			row["voltage"] = v
		}
		if v, ok := opt["current"]; ok && v != nil {
			row["current"] = v
		}
		if v, ok := opt["temperature"]; ok && v != nil {
			row["temperature"] = v
		}
	}
	sort.Slice(pons, func(i, j int) bool {
		return ponIndexFromRow(pons[i]) < ponIndexFromRow(pons[j])
	})
	return pons
}

func ponIndexFromRow(p map[string]any) int {
	if p == nil {
		return 0
	}
	if n, ok := p["pon"].(int); ok && n > 0 {
		return n
	}
	id := strings.TrimSpace(anyString(p["id"]))
	if id == "" {
		id = strings.TrimSpace(anyString(p["name"]))
	}
	n, err := strconv.Atoi(strings.TrimLeft(id, "0"))
	if err == nil && n > 0 {
		return n
	}
	return intFromAny(p["id"])
}

func anyString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func ParsePonFromSuffix(baseOID, fullOID string, ponByIfIndex map[int]ponIfRef) (pon int, ok bool) {
	base := strings.TrimSuffix(strings.TrimSpace(probing.NormalizeSNMPOID(baseOID)), ".")
	full := strings.TrimSpace(probing.NormalizeSNMPOID(fullOID))
	if base == "" || full == "" || !strings.HasPrefix(full, base) {
		return 0, false
	}
	suffix := strings.TrimPrefix(full, base)
	suffix = strings.TrimPrefix(suffix, ".")
	if suffix == "" {
		return 0, false
	}
	segs := strings.Split(suffix, ".")
	segStr := segs[len(segs)-1]
	// Alguns OIDs Datacom trazem .ifIndex.lane (ex.: ...101744641.1).
	// Nesses casos a PON está no penúltimo segmento (ifIndex), não no lane.
	if len(segs) >= 2 {
		last, errLast := strconv.Atoi(strings.TrimSpace(segs[len(segs)-1]))
		prev, errPrev := strconv.Atoi(strings.TrimSpace(segs[len(segs)-2]))
		if errLast == nil && errPrev == nil && last > 0 && last <= 8 && prev > 10000 {
			segStr = segs[len(segs)-2]
		}
	}
	seg, err := strconv.Atoi(strings.TrimSpace(segStr))
	if err != nil || seg <= 0 {
		return 0, false
	}
	if seg >= snmpIfIndexMin {
		if ref, ok := ponByIfIndex[seg]; ok && ref.PonPort > 0 {
			return ref.PonPort, true
		}
		if len(segs) >= 2 {
			last, errLast := strconv.Atoi(strings.TrimSpace(segs[len(segs)-1]))
			prev, errPrev := strconv.Atoi(strings.TrimSpace(segs[len(segs)-2]))
			if errLast == nil && errPrev == nil && last > 0 && last <= 8 && prev >= snmpIfIndexMin {
				if ref, ok := ponByIfIndex[prev]; ok && ref.PonPort > 0 {
					return ref.PonPort, true
				}
				if len(ponByIfIndex) == 0 {
					if d := prev % 10; d > 0 && d <= 64 {
						return d, true
					}
				}
			}
		}
		if len(ponByIfIndex) == 0 {
			return seg, true
		}
		return 0, false
	}
	if seg <= 256 {
		return seg, true
	}
	return seg, true
}

func loadPonIfIndexMap(ctx context.Context, host, community string, budget time.Duration, ifDescrOID, ifNameOID string) map[int]ponIfRef {
	out := map[int]ponIfRef{}
	ifDescrOID = strings.TrimSpace(ifDescrOID)
	if ifDescrOID == "" {
		ifDescrOID = "1.3.6.1.2.1.2.2.1.2"
	}
	ifNameOID = strings.TrimSpace(ifNameOID)
	if ifNameOID == "" {
		ifNameOID = "1.3.6.1.2.1.31.1.1.1.1"
	}
	if budget <= 0 {
		budget = 20 * time.Second
	}
	descrByIdx := map[int]string{}
	nameByIdx := map[int]string{}
	vars, _, _ := doSNMPWalk(ctx, host, community, ifDescrOID, budget)
	for _, v := range vars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		descrByIdx[idx] = sanitizeSNMPValue(v.Value)
	}
	nameVars, _, _ := doSNMPWalk(ctx, host, community, ifNameOID, budget)
	for _, v := range nameVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		nameByIdx[idx] = sanitizeSNMPValue(v.Value)
	}
	seen := map[int]bool{}
	for idx := range descrByIdx {
		seen[idx] = true
	}
	for idx := range nameByIdx {
		seen[idx] = true
	}
	for idx := range seen {
		port, compact, ok := parsePonPortFromIfLabels(nameByIdx[idx], descrByIdx[idx])
		if !ok {
			continue
		}
		name := strings.TrimSpace(nameByIdx[idx])
		if name == "" {
			name = strings.TrimSpace(descrByIdx[idx])
		}
		out[idx] = ponIfRef{PonPort: port, Compact: compact, Name: name, IfIndex: idx}
	}
	return out
}

func ponByIfIndexForPort(m map[int]ponIfRef, port int) (ponIfRef, bool) {
	for _, ref := range m {
		if ref.PonPort == port {
			return ref, true
		}
	}
	return ponIfRef{}, false
}

func needsVsolOnuPonIndex(metrics OnuMetricsConfig) bool {
	for _, def := range metrics {
		if !def.Enabled {
			continue
		}
		if strings.Contains(def.OID, ".37950.") {
			return true
		}
	}
	return false
}

func loadOnuPonIndexMap(ctx context.Context, host, community string, budget time.Duration) map[int]int {
	out := map[int]int{}
	if budget <= 0 {
		budget = 12 * time.Second
	}
	base := probing.NormalizeSNMPOID(VSOLOnuPonIndexOID)
	vars, _, _ := doSNMPWalk(ctx, host, community, base, budget)
	for _, v := range vars {
		onu, ok := parseSingleOnuSuffix(base, v.OID)
		if !ok {
			continue
		}
		pon, err := strconv.Atoi(strings.TrimSpace(sanitizeSNMPValue(v.Value)))
		if err != nil || pon <= 0 {
			continue
		}
		out[onu] = pon
	}
	return out
}

func parseSingleOnuSuffix(baseOID, fullOID string) (onu int, ok bool) {
	base := strings.TrimSuffix(strings.TrimSpace(probing.NormalizeSNMPOID(baseOID)), ".")
	full := strings.TrimSpace(probing.NormalizeSNMPOID(fullOID))
	if base == "" || full == "" || !strings.HasPrefix(full, base) {
		return 0, false
	}
	suffix := strings.TrimPrefix(strings.TrimPrefix(full, base), ".")
	if suffix == "" {
		return 0, false
	}
	parts := strings.Split(suffix, ".")
	if len(parts) != 1 {
		return 0, false
	}
	onu, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || onu <= 0 {
		return 0, false
	}
	return onu, true
}

// ponSlotFromVsolPhaseWalkBase: na VSOL Pirapetinga/V1600 a fase usa …1.1.1.1.5.1.{onu} (PON 1).
// O ramo …5.2.{onu} não é slot PON — devolve outro campo (ex. los=1).
func ponSlotFromVsolPhaseWalkBase(base string) int {
	base = strings.TrimSuffix(strings.TrimSpace(probing.NormalizeSNMPOID(base)), ".")
	if strings.HasSuffix(base, ".5.1") {
		return 1
	}
	return 0
}

// ParsePonOnuSuffixMapped resolve sufixo .PON.ONU, .ifIndex.ONU (ZTE) ou .ONU com mapa PON VSOL.
func ParsePonOnuSuffixMapped(baseOID, fullOID string, ponByIfIndex map[int]ponIfRef, onuPonByOnu map[int]int) (ponPort, onu, ifIndex int, ok bool) {
	pon, onuNum, ok := ParsePonOnuSuffix(baseOID, fullOID)
	if !ok {
		return 0, 0, 0, false
	}
	if pon <= 0 && onuNum > 0 && len(onuPonByOnu) > 0 {
		if p, found := onuPonByOnu[onuNum]; found && p > 0 {
			pon = p
		}
	}
	if pon >= snmpIfIndexMin && len(ponByIfIndex) > 0 {
		ref, found := ponByIfIndex[pon]
		if !found {
			return 0, 0, 0, false
		}
		return ref.PonPort, onuNum, pon, true
	}
	return pon, onuNum, 0, true
}

func parseGaugeInt(raw string) (int, error) {
	s := sanitizeSNMPValue(raw)
	if s == "" {
		return 0, fmt.Errorf("empty gauge")
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	if mm := reParenNumber.FindStringSubmatch(s); len(mm) == 2 {
		return strconv.Atoi(mm[1])
	}
	parts := reDigits.FindAllString(s, -1)
	if len(parts) == 0 {
		return 0, fmt.Errorf("gauge parse: %q", raw)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func StatusIsOnline(val int, def OnuMetricDef) bool {
	for _, x := range def.OnlineValues {
		if val == x {
			return true
		}
	}
	if len(def.OnlineValues) == 0 && val == 1 {
		return true
	}
	for _, x := range def.OfflineValues {
		if val == x {
			return false
		}
	}
	if len(def.OfflineValues) == 0 {
		return false
	}
	return false
}

// StatusIsOnlineIfOper interpreta ifOperStatus (1=up, 2=down) com override por online/offline_values.
func StatusIsOnlineIfOper(val int, def OnuMetricDef) bool {
	for _, x := range def.OnlineValues {
		if val == x {
			return true
		}
	}
	for _, x := range def.OfflineValues {
		if val == x {
			return false
		}
	}
	return val == 1
}

var reDigits = regexp.MustCompile(`\d+`)
var reParenNumber = regexp.MustCompile(`\((\d+)\)\s*$`)

func collectStatusFromIfMib(
	ctx context.Context,
	host, community string,
	perWalk time.Duration,
	def OnuMetricDef,
	operVars []probing.SNMPVar,
) (map[string]map[string]any, int) {
	out := map[string]map[string]any{}
	ifDescrOID := strings.TrimSpace(def.IfDescrOID)
	if ifDescrOID == "" {
		ifDescrOID = "1.3.6.1.2.1.2.2.1.2"
	}
	ifNameOID := strings.TrimSpace(def.IfNameOID)
	if ifNameOID == "" {
		ifNameOID = "1.3.6.1.2.1.31.1.1.1.1"
	}
	ifOperOID := strings.TrimSpace(def.IfOperOID)
	if ifOperOID == "" {
		ifOperOID = "1.3.6.1.2.1.2.2.1.8"
	}
	wCtx1, cancel1 := context.WithTimeout(ctx, perWalk)
	descrVars, _, _ := probing.SNMPWalk(wCtx1, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: ifDescrOID,
		Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: 25000,
	})
	cancel1()
	wCtx1b, cancel1b := context.WithTimeout(ctx, perWalk)
	nameVars, _, _ := probing.SNMPWalk(wCtx1b, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: ifNameOID,
		Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: 25000,
	})
	cancel1b()
	if len(operVars) == 0 {
		wCtx2, cancel2 := context.WithTimeout(ctx, perWalk)
		operVars, _, _ = probing.SNMPWalk(wCtx2, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: ifOperOID,
			Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: 25000,
		})
		cancel2()
	}

	descrByIdx := map[int]string{}
	for _, v := range descrVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		descrByIdx[idx] = sanitizeSNMPValue(v.Value)
	}
	nameByIdx := map[int]string{}
	for _, v := range nameVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		nameByIdx[idx] = sanitizeSNMPValue(v.Value)
	}
	statusByIdx := map[int]int{}
	statusRawByIdx := map[int]string{}
	for _, v := range operVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		n, err := parseStatusInt(v.Value)
		if err != nil {
			continue
		}
		statusByIdx[idx] = n
		statusRawByIdx[idx] = strings.TrimSpace(v.Value)
	}
	seenIdx := map[int]struct{}{}
	for idx := range descrByIdx {
		seenIdx[idx] = struct{}{}
	}
	for idx := range nameByIdx {
		seenIdx[idx] = struct{}{}
	}
	matched := 0
	for idx := range seenIdx {
		status, ok := statusByIdx[idx]
		if !ok {
			continue
		}
		ifName := nameByIdx[idx]
		descr := descrByIdx[idx]
		pon, onu, compact, ok := oltifderive.ParseOnuIfLabels(ifName, descr)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%d.%d", pon, onu)
		on := StatusIsOnlineIfOper(status, def)
		out[key] = map[string]any{
			"onu_online_sta":      status,
			"oper_status":         status,
			"oper_status_label":   operStatusLabel(status),
			"oper_status_raw":     statusRawByIdx[idx],
			"online":              on,
			"if_index":            idx,
			"if_name":             ifName,
			"if_descr":            descr,
			"pon_compact":         compact,
		}
		matched++
	}
	return out, matched
}

func offlineRxDbmThreshold(def OnuMetricDef) float64 {
	if def.OfflineRxDbm != 0 {
		return def.OfflineRxDbm
	}
	return DefaultOfflineRxDbm
}

func isRxPowerInvalidRaw(raw int) bool {
	switch raw {
	case 65535000, -80000, 2147483647:
		return true
	default:
		return false
	}
}

// StatusFromRxPower online quando RX válido e >= limiar (dBm); <= limiar ou leitura 0 ⇒ offline.
func StatusFromRxPower(raw int, dbm float64, dbmOk bool, offlineThresholdDbm float64) bool {
	if isRxPowerInvalidRaw(raw) {
		return false
	}
	if !dbmOk {
		return false
	}
	// VSOL e similares enviam "0.00" quando não há sinal — não tratar como online.
	if dbm >= 0 {
		return false
	}
	return dbm >= offlineThresholdDbm
}

func shouldApplyStatusFromRx(metrics OnuMetricsConfig) bool {
	st, ok := metrics[MetricStatus]
	if !ok || !st.Enabled {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(st.StatusMode), StatusModeRxPowerThreshold) {
		return false
	}
	rx, ok := metrics[MetricRxPower]
	return ok && rx.Enabled
}

func rxThresholdDef(metrics OnuMetricsConfig) OnuMetricDef {
	def := OnuMetricDef{OfflineRxDbm: DefaultOfflineRxDbm}
	if st, ok := metrics[MetricStatus]; ok {
		def = st
	}
	if def.OfflineRxDbm == 0 {
		def.OfflineRxDbm = DefaultOfflineRxDbm
	}
	if rx, ok := metrics[MetricRxPower]; ok && rx.ValueDivisor > def.ValueDivisor {
		def.ValueDivisor = rx.ValueDivisor
	}
	return def
}

func onuMetricsEnabledKeys(metrics OnuMetricsConfig) []string {
	keys := []string{MetricSerial, MetricStatus, MetricRxPower, MetricTxPower, MetricTemperature, MetricModel, MetricVlan}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if def, ok := metrics[k]; ok && def.Enabled {
			out = append(out, k)
		}
	}
	return out
}

func collectStatusFromRxPowerVars(
	vars []probing.SNMPVar,
	base string,
	def OnuMetricDef,
	ponByIfIndex map[int]ponIfRef,
	onuPonByOnu map[int]int,
	getRow func(pon, onu int) map[string]any,
) int {
	threshold := offlineRxDbmThreshold(def)
	matched := 0
	for _, v := range vars {
		pon, onu, ifIdx, ok := ParsePonOnuSuffixMapped(base, v.OID, ponByIfIndex, onuPonByOnu)
		if !ok {
			continue
		}
		matched++
		row := getRow(pon, onu)
		if ifIdx > 0 {
			row["if_index"] = ifIdx
			if ref, ok := ponByIfIndex[ifIdx]; ok {
				row["pon_compact"] = ref.Compact
				row["pon_name"] = ref.Name
			}
		}
		rawInt, _ := parseGaugeInt(v.Value)
		dbm, display, dbmOk := parseOnuRxDbm(v.Value, def.ValueDivisor)
		online := StatusFromRxPower(rawInt, dbm, dbmOk, threshold)
		if dbmOk {
			row["rx_dbm"] = dbm
			row["rx_pwr"] = display
		} else if display != "" {
			row["rx_pwr"] = display
		} else {
			row["rx_pwr"] = normalizeSnmpDisplayValue(v.Value)
		}
		row["onu_online_sta"] = rawInt
		row["online"] = online
		row["status_source"] = "rx_threshold"
		row["offline_rx_dbm"] = threshold
		if online {
			row["oper_status_label"] = "up"
		} else {
			row["oper_status_label"] = "down"
		}
	}
	return matched
}

// applyStatusFromRxOnRows preenche online a partir de rx_dbm/rx_pwr já coletado (evita walk duplicado).
func applyStatusFromRxOnRows(byKey map[string]*map[string]any, def OnuMetricDef) {
	threshold := offlineRxDbmThreshold(def)
	for _, ptr := range byKey {
		row := *ptr
		dbm, dbmOk := rxDbmFromRow(row, def.ValueDivisor)
		if !dbmOk {
			continue
		}
		row["rx_dbm"] = dbm
		row["rx_pwr"] = fmt.Sprintf("%.2f", dbm)
		raw := intFromAny(row["onu_online_sta"])
		online := StatusFromRxPower(raw, dbm, true, threshold)
		row["online"] = online
		row["status_source"] = "rx_threshold"
		row["offline_rx_dbm"] = threshold
		if online {
			row["oper_status_label"] = "up"
		} else {
			row["oper_status_label"] = "down"
		}
	}
}

func rxDbmFromRow(row map[string]any, divisor int) (float64, bool) {
	if f, ok := row["rx_dbm"].(float64); ok && isPlausibleOnuRxDbm(f) {
		return f, true
	}
	if s := strings.TrimSpace(anyString(row["rx_pwr"])); s != "" {
		if f, _, ok := parseOnuRxDbm(s, divisor); ok {
			return f, true
		}
	}
	return 0, false
}

func operStatusLabel(status int) string {
	switch status {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	case 4:
		return "unknown"
	case 5:
		return "dormant"
	case 6:
		return "notPresent"
	case 7:
		return "lowerLayerDown"
	default:
		return strconv.Itoa(status)
	}
}

func collectPonStatusFromIfMib(
	ctx context.Context,
	host, community string,
	perWalk time.Duration,
	def OnuMetricDef,
	operVars []probing.SNMPVar,
) (map[int]map[string]any, int) {
	out := map[int]map[string]any{}
	ifDescrOID := strings.TrimSpace(def.IfDescrOID)
	if ifDescrOID == "" {
		ifDescrOID = "1.3.6.1.2.1.2.2.1.2"
	}
	ifOperOID := strings.TrimSpace(def.IfOperOID)
	if ifOperOID == "" {
		ifOperOID = "1.3.6.1.2.1.2.2.1.8"
	}
	ifNameOID := strings.TrimSpace(def.IfNameOID)
	if ifNameOID == "" {
		ifNameOID = "1.3.6.1.2.1.31.1.1.1.1"
	}
	wCtx1, cancel1 := context.WithTimeout(ctx, perWalk)
	descrVars, _, _ := probing.SNMPWalk(wCtx1, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: ifDescrOID,
		Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: 25000,
	})
	cancel1()
	wCtx1b, cancel1b := context.WithTimeout(ctx, perWalk)
	nameVars, _, _ := probing.SNMPWalk(wCtx1b, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: ifNameOID,
		Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: 25000,
	})
	cancel1b()
	if len(operVars) == 0 {
		wCtx2, cancel2 := context.WithTimeout(ctx, perWalk)
		operVars, _, _ = probing.SNMPWalk(wCtx2, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: ifOperOID,
			Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: 25000,
		})
		cancel2()
	}

	descrByIdx := map[int]string{}
	for _, v := range descrVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		descrByIdx[idx] = sanitizeSNMPValue(v.Value)
	}
	nameByIdx := map[int]string{}
	for _, v := range nameVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		nameByIdx[idx] = sanitizeSNMPValue(v.Value)
	}
	statusByIdx := map[int]int{}
	statusRawByIdx := map[int]string{}
	for _, v := range operVars {
		idx, ok := suffixIndex(v.OID)
		if !ok {
			continue
		}
		n, err := parseStatusInt(v.Value)
		if err != nil {
			continue
		}
		statusByIdx[idx] = n
		statusRawByIdx[idx] = strings.TrimSpace(v.Value)
	}
	matched := 0
	seenIdx := map[int]bool{}
	for idx := range descrByIdx {
		seenIdx[idx] = true
	}
	for idx := range nameByIdx {
		seenIdx[idx] = true
	}
	for idx := range seenIdx {
		status, ok := statusByIdx[idx]
		if !ok {
			continue
		}
		pon, compact, ok := parsePonPortFromIfLabels(nameByIdx[idx], descrByIdx[idx])
		if !ok {
			continue
		}
		on := StatusIsOnlineIfOper(status, def)
		st := "down"
		if on {
			st = "up"
		}
		name := strings.TrimSpace(nameByIdx[idx])
		if name == "" {
			name = strings.TrimSpace(descrByIdx[idx])
		}
		out[pon] = map[string]any{
			"pon_oper_status":     status,
			"pon_oper_status_raw": statusRawByIdx[idx],
			"if_oper_status":      st,
			"status":              st,
			"if_index":            idx,
			"pon_compact":         compact,
			"pon_name":            name,
		}
		matched++
	}
	return out, matched
}

func suffixIndex(oid string) (int, bool) {
	norm := strings.TrimSpace(probing.NormalizeSNMPOID(oid))
	if norm == "" {
		return 0, false
	}
	parts := strings.Split(norm, ".")
	if len(parts) == 0 {
		return 0, false
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	n, err := strconv.Atoi(last)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parsePonOnuFromIfDescr(raw string) (pon, onu int, ok bool) {
	s := sanitizeSNMPValue(raw)
	if s == "" {
		return 0, 0, false
	}
	var compact string
	pon, onu, compact, ok = oltifderive.ParseOnuIfLabels(s, s)
	_ = compact
	return pon, onu, ok
}

func parsePonPortFromIfLabels(ifName, descr string) (port int, compact string, ok bool) {
	name := sanitizeSNMPValue(ifName)
	desc := sanitizeSNMPValue(descr)
	disp := strings.TrimSpace(name)
	if disp == "" {
		disp = strings.TrimSpace(desc)
	}
	if disp == "" {
		return 0, "", false
	}
	if oltifderive.ClassifyKind(name, desc) != oltifderive.KindPON {
		return 0, "", false
	}
	compact = oltifderive.PonCompactFromPhy(name, desc)
	if compact == "" {
		return 0, "", false
	}
	port = oltifderive.PonPortFromCompact(compact)
	if port <= 0 {
		return 0, "", false
	}
	return port, compact, true
}

func parseStatusInt(raw string) (int, error) {
	s := sanitizeSNMPValue(raw)
	if s == "" {
		return 0, fmt.Errorf("empty status")
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	if mm := reParenNumber.FindStringSubmatch(s); len(mm) == 2 {
		return strconv.Atoi(mm[1])
	}
	parts := reDigits.FindAllString(s, -1)
	if len(parts) == 0 {
		return 0, fmt.Errorf("status parse: %q", raw)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func sanitizeSNMPValue(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}
	upper := strings.ToUpper(s)
	if i := strings.Index(upper, "STRING:"); i >= 0 {
		s = strings.TrimSpace(s[i+len("STRING:"):])
	} else if i := strings.Index(upper, "INTEGER:"); i >= 0 {
		s = strings.TrimSpace(s[i+len("INTEGER:"):])
	} else if i := strings.Index(upper, "HEX-STRING:"); i >= 0 {
		s = strings.TrimSpace(s[i+len("HEX-STRING:"):])
	}
	return strings.TrimSpace(strings.Trim(s, `"`))
}

// normalizeSnmpDisplayValue converte OctetString em hex (ex.: 2d:31:39:2e:35:31) para texto legível (-19.51).
func normalizeSnmpDisplayValue(raw string) string {
	s := sanitizeSNMPValue(raw)
	if s == "" {
		return s
	}
	if decoded, ok := decodeColonHexASCII(s); ok {
		return decoded
	}
	return s
}

func decodeColonHexASCII(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ":") {
		return "", false
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return "", false
	}
	var b strings.Builder
	for _, p := range parts {
		if len(p) != 2 {
			return "", false
		}
		n, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return "", false
		}
		if n < 32 || n > 126 {
			return "", false
		}
		b.WriteByte(byte(n))
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", false
	}
	if matched, _ := regexp.MatchString(`^-?\d+(\.\d+)?$`, out); matched {
		return out, true
	}
	if len(parts) == 6 && strings.ContainsAny(out, ".-") {
		return out, true
	}
	if len(parts) != 6 {
		return out, true
	}
	return "", false
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && v != "<nil>" {
			return v
		}
	}
	return ""
}

// ParsePonOnuSuffix interpreta sufixo .PON.ONU após a raiz da tabela SNMP.
func ParsePonOnuSuffix(baseOID, fullOID string) (pon, onu int, ok bool) {
	base := strings.TrimSuffix(strings.TrimSpace(probing.NormalizeSNMPOID(baseOID)), ".")
	full := strings.TrimSpace(probing.NormalizeSNMPOID(fullOID))
	if base == "" || full == "" || !strings.HasPrefix(full, base) {
		return 0, 0, false
	}
	suffix := strings.TrimPrefix(full, base)
	suffix = strings.TrimPrefix(suffix, ".")
	if suffix == "" {
		return 0, 0, false
	}
	parts := strings.Split(suffix, ".")
	if len(parts) >= 2 {
		pon, err1 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-2]))
		onu, err2 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
		if err1 != nil || err2 != nil || pon <= 0 || onu <= 0 {
			return 0, 0, false
		}
		return pon, onu, true
	}
	if len(parts) == 1 {
		onu, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || onu <= 0 {
			return 0, 0, false
		}
		if pon := ponSlotFromVsolPhaseWalkBase(base); pon > 0 {
			return pon, onu, true
		}
		return 0, onu, true
	}
	return 0, 0, false
}

func BuildPonsFromOnuRows(onuRows []map[string]any, ponByIfIndex map[int]ponIfRef) []map[string]any {
	type agg struct {
		total, online, offline int
		compact, name          string
		ifIndex                int
	}
	by := map[int]*agg{}
	for _, r := range onuRows {
		pon, _ := r["pon"].(int)
		if pon <= 0 {
			continue
		}
		a := by[pon]
		if a == nil {
			a = &agg{}
			by[pon] = a
		}
		if c := strings.TrimSpace(anyString(r["pon_compact"])); c != "" {
			a.compact = c
		}
		if n := strings.TrimSpace(anyString(r["pon_name"])); n != "" {
			a.name = n
		}
		if ix := intFromAny(r["if_index"]); ix > 0 {
			a.ifIndex = ix
		}
		a.total++
		if on, ok := r["online"].(bool); ok && on {
			a.online++
		} else if _, has := r["online"]; has {
			a.offline++
		}
	}
	keys := make([]int, 0, len(by))
	for p := range by {
		keys = append(keys, p)
	}
	sort.Ints(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, pon := range keys {
		a := by[pon]
		id := fmt.Sprintf("%02d", pon)
		name := "GPON0/" + id
		if a.compact != "" {
			id = a.compact
			name = "PON-" + a.compact
		} else if a.name != "" {
			name = a.name
		} else if ref, ok := ponByIfIndexForPort(ponByIfIndex, pon); ok {
			if ref.Compact != "" {
				id = ref.Compact
				name = ref.Name
				if name == "" {
					name = "PON-" + ref.Compact
				}
			}
			if a.ifIndex == 0 {
				a.ifIndex = ref.IfIndex
			}
		}
		off := a.offline
		if off < 0 {
			off = 0
		}
		if a.total > a.online+off {
			off = a.total - a.online
		}
		row := map[string]any{
			"id": id, "name": name, "pon": pon,
			"onu_total": a.total, "onu_online": a.online, "onu_offline": off,
			"status": "snmp_metrics", "source_slice": "onu_metrics_collect",
		}
		if a.ifIndex > 0 {
			row["if_index"] = a.ifIndex
		}
		if a.compact != "" {
			row["pon_compact"] = a.compact
		}
		out = append(out, row)
	}
	return out
}
