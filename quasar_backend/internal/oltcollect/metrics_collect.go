package oltcollect

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

var retryableMetrics = map[string]bool{
	MetricRxPower:     true,
	MetricTxPower:     true,
	MetricPonRxPower:  true,
	MetricPonTxPower:  true,
	MetricTemperature: true,
}

func isPonOnlyMetric(key string) bool {
	return key == MetricPonRxPower || key == MetricPonTxPower
}

// CollectOnuMetrics executa snmpwalk em cada tabela OID configurada e agrega por PON/ONU.
func CollectOnuMetrics(ctx context.Context, host, community string, metrics OnuMetricsConfig, totalBudget time.Duration) (
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
		case MetricRxPower, MetricTxPower, MetricPonRxPower, MetricPonTxPower, MetricTemperature:
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
				entry["note"] = firstNonEmptyString(fmt.Sprint(entry["note"]), "status sem correspondência IF-MIB (ifDescr/ifOperStatus)")
			}
		} else if isPonOnlyMetric(key) {
			for _, v := range vars {
				pon, ok := ParsePonFromSuffix(base, v.OID)
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
					if f, ok := parseOpticalDbm(val); ok {
						ponOptical[pon]["rx_dbm"] = f
					} else {
						ponOptical[pon]["rx_dbm_raw"] = val
					}
				case MetricPonTxPower:
					if f, ok := parseOpticalDbm(val); ok {
						ponOptical[pon]["tx_dbm"] = f
					} else {
						ponOptical[pon]["tx_dbm_raw"] = val
					}
				}
			}
		} else {
			for _, v := range vars {
				pon, onu, ok := ParsePonOnuSuffix(base, v.OID)
				if !ok {
					continue
				}
				matched++
				row := getRow(pon, onu)
				val := strings.TrimSpace(v.Value)
				switch key {
				case MetricSerial:
					row["serial"] = val
				case MetricModel:
					row["model"] = val
				case MetricRxPower:
					row["rx_pwr"] = val
				case MetricTxPower:
					row["tx_pwr"] = val
				case MetricTemperature:
					row["temp"] = val
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
	} else if len(ponCountsByPon) > 0 {
		online, offline = 0, 0
		for _, p := range pons {
			online += intFromAny(p["onu_online"])
			offline += intFromAny(p["onu_offline"])
		}
	}
	pons = BuildPonsFromOnuRows(onuRows)
	if len(ponCountsByPon) > 0 {
		pons = mergePonsWithCountMaps(pons, ponCountsByPon)
	}
	pons = mergePonOpticalIntoPons(pons, ponOptical)
	arr := make([]any, 0, len(onuRows))
	for _, r := range onuRows {
		arr = append(arr, r)
	}

	summary["vsol_onu_rows"] = arr
	summary["vsol_onu_count"] = len(onuRows)
	summary["vsol_onu_online"] = online
	summary["vsol_onu_offline"] = offline
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
		pon, ok := ParsePonFromSuffix(onBase, v.OID)
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
		pon, ok := ParsePonFromSuffix(offBase, v.OID)
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

func parseOpticalDbm(raw string) (float64, bool) {
	s := sanitizeSNMPValue(raw)
	if s == "" {
		return 0, false
	}
	normalizeDbm := func(f float64) float64 {
		if f > 100 || f < -100 {
			return f / 100
		}
		return f
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return normalizeDbm(f), true
	}
	if mm := reParenNumber.FindStringSubmatch(s); len(mm) == 2 {
		if f, err := strconv.ParseFloat(mm[1], 64); err == nil {
			return normalizeDbm(f), true
		}
	}
	parts := reDigits.FindAllString(s, -1)
	if len(parts) == 0 {
		return 0, false
	}
	if f, err := strconv.ParseFloat(parts[len(parts)-1], 64); err == nil {
		return normalizeDbm(f), true
	}
	return 0, false
}

func mergePonOpticalIntoPons(pons []map[string]any, optical map[int]map[string]any) []map[string]any {
	if len(optical) == 0 {
		return pons
	}
	byPon := map[int]map[string]any{}
	for _, p := range pons {
		pon := ponIndexFromRow(p)
		if pon <= 0 {
			continue
		}
		byPon[pon] = p
	}
	for pon, opt := range optical {
		row := byPon[pon]
		if row == nil {
			id := fmt.Sprintf("%02d", pon)
			row = map[string]any{
				"id": id, "name": "GPON0/" + id,
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

func ParsePonFromSuffix(baseOID, fullOID string) (pon int, ok bool) {
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
	segStr := suffix
	if i := strings.LastIndex(suffix, "."); i >= 0 {
		segStr = suffix[i+1:]
	}
	seg, err := strconv.Atoi(strings.TrimSpace(segStr))
	if err != nil || seg <= 0 {
		return 0, false
	}
	if seg <= 256 {
		return seg, true
	}
	if seg > 10000 {
		d := seg % 10
		if d > 0 && d <= 64 {
			return d, true
		}
		d = seg % 100
		if d > 0 && d <= 64 {
			return d, true
		}
	}
	return seg, true
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
		descrByIdx[idx] = strings.TrimSpace(v.Value)
	}
	statusByIdx := map[int]int{}
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
	}
	matched := 0
	for idx, descr := range descrByIdx {
		status, ok := statusByIdx[idx]
		if !ok {
			continue
		}
		pon, onu, ok := parsePonOnuFromIfDescr(descr)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%d.%d", pon, onu)
		out[key] = map[string]any{
			"onu_online_sta": status,
			"online":         StatusIsOnline(status, def),
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
	compact, onuIdx, parsed := oltifderive.PonCompactFromOnuIface(s, s)
	if !parsed {
		return 0, 0, false
	}
	if onuIdx <= 0 {
		return 0, 0, false
	}
	nums := reDigits.FindAllString(compact, -1)
	if len(nums) == 0 {
		return 0, 0, false
	}
	last := nums[len(nums)-1]
	ponNum, err := strconv.Atoi(strings.TrimLeft(last, "0"))
	if err != nil || ponNum <= 0 {
		if last == "0" {
			return 0, 0, false
		}
		ponNum, err = strconv.Atoi(last)
		if err != nil || ponNum <= 0 {
			return 0, 0, false
		}
	}
	return ponNum, onuIdx, true
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
	}
	return strings.TrimSpace(strings.Trim(s, `"`))
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
	if len(parts) < 2 {
		return 0, 0, false
	}
	pon, err1 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-2]))
	onu, err2 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err1 != nil || err2 != nil || pon <= 0 || onu <= 0 {
		return 0, 0, false
	}
	return pon, onu, true
}

func BuildPonsFromOnuRows(onuRows []map[string]any) []map[string]any {
	type agg struct{ total, online, offline int }
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
		off := a.offline
		if off < 0 {
			off = 0
		}
		if a.total > a.online+off {
			off = a.total - a.online
		}
		out = append(out, map[string]any{
			"id": id, "name": "GPON0/" + id,
			"onu_total": a.total, "onu_online": a.online, "onu_offline": off,
			"status": "snmp_metrics", "source_slice": "onu_metrics_collect",
		})
	}
	return out
}
