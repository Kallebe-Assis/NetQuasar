package api

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snapshotwalk"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/netquasar/netquasar/quasar_backend/internal/switchcollect"
	"github.com/rs/zerolog"
)

type oltVendorOIDProfile struct {
	OnuOnlineOID   string
	PonStatusOID   string
	TransceiverOID string
	SNMPBaseOID    string
}

func trimForSummary(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func parseIfIndexFromSuffix(suffix string) int {
	s := strings.TrimSpace(suffix)
	if s == "" {
		return 0
	}
	part := s
	if i := strings.Index(part, "."); i >= 0 {
		part = part[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(part))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func statusLabelFromInt(v int) string {
	switch v {
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
		return strconv.Itoa(v)
	}
}

func enrichZTERows(rows []any, ifNameByIndex map[int]string, asOperStatus bool) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, it := range rows {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		for k, v := range m {
			row[k] = v
		}
		suffix, _ := row["suffix"].(string)
		ifIdx := parseIfIndexFromSuffix(suffix)
		if ifIdx > 0 {
			row["if_index"] = ifIdx
			if name := strings.TrimSpace(ifNameByIndex[ifIdx]); name != "" {
				row["if_name"] = name
			}
		}
		if asOperStatus {
			switch x := row["value_int"].(type) {
			case int:
				row["value_label"] = statusLabelFromInt(x)
			case float64:
				row["value_label"] = statusLabelFromInt(int(x))
			}
		}
		out = append(out, row)
	}
	return out
}

func parseIfAndOnuFromSuffix(suffix string) (ifIndex int, onuIndex int, ok bool) {
	s := strings.TrimSpace(suffix)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	ifx, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	onu, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || ifx <= 0 || onu <= 0 {
		return 0, 0, false
	}
	return ifx, onu, true
}

func buildZTEPonRowsFromTables(onuRows []map[string]any, ponRows []map[string]any) []map[string]any {
	type agg struct {
		ifIndex int
		total   int
		online  int
		offline int
		status  string
	}
	byIf := map[int]*agg{}
	for _, r := range ponRows {
		ifIdx := parseIfIndexFromSuffix(anyString(r["suffix"]))
		if ifIdx <= 0 {
			continue
		}
		st := strings.TrimSpace(anyString(r["value_label"]))
		if st == "" {
			if v, ok := r["value_int"].(int); ok {
				st = statusLabelFromInt(v)
			}
		}
		byIf[ifIdx] = &agg{ifIndex: ifIdx, status: st}
	}
	for _, r := range onuRows {
		ifIdx, _, ok := parseIfAndOnuFromSuffix(anyString(r["suffix"]))
		if !ok {
			continue
		}
		a := byIf[ifIdx]
		if a == nil {
			a = &agg{ifIndex: ifIdx}
			byIf[ifIdx] = a
		}
		a.total++
		if v, ok := r["value_int"].(int); ok && v == 1 {
			a.online++
		} else {
			a.offline++
		}
	}
	out := make([]map[string]any, 0, len(byIf))
	for _, a := range byIf {
		if a.offline == 0 && a.total > a.online {
			a.offline = a.total - a.online
		}
		row := map[string]any{
			"id":           strconv.Itoa(a.ifIndex),
			"name":         "PON ifIndex " + strconv.Itoa(a.ifIndex),
			"onu_total":    a.total,
			"onu_online":   a.online,
			"onu_offline":  a.offline,
			"status":       strings.TrimSpace(a.status),
			"source_slice": "zte_oid",
			"if_index":     a.ifIndex,
		}
		out = append(out, row)
	}
	return out
}

func buildZTEPonStatusByIfIndex(ponRows []map[string]any) map[int]string {
	out := map[int]string{}
	for _, r := range ponRows {
		ifIdx := parseIfIndexFromSuffix(anyString(r["suffix"]))
		if ifIdx <= 0 {
			continue
		}
		if lb := strings.TrimSpace(anyString(r["value_label"])); lb != "" {
			out[ifIdx] = lb
			continue
		}
		if v, ok := r["value_int"].(int); ok {
			out[ifIdx] = statusLabelFromInt(v)
		}
	}
	return out
}

func buildDatacomPonRowsFromTable(rows []map[string]any) []map[string]any {
	type agg struct {
		key     string
		name    string
		status  string
		total   int
		online  int
		offline int
	}
	byKey := map[string]*agg{}
	for _, r := range rows {
		sfx := strings.TrimSpace(anyString(r["suffix"]))
		if sfx == "" {
			continue
		}
		parts := strings.Split(sfx, ".")
		if len(parts) == 0 {
			continue
		}
		col, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		key := strings.Join(parts[1:], ".")
		if strings.TrimSpace(key) == "" {
			key = strings.TrimSpace(parts[0])
		}
		a := byKey[key]
		if a == nil {
			a = &agg{key: key}
			byKey[key] = a
		}
		switch col {
		case 1:
			// índice interno da PON na tabela Datacom
		case 2:
			a.name = strings.TrimSpace(anyString(r["value"]))
		case 3:
			if v, ok := r["value_int"].(int); ok {
				a.total = v
			}
		case 4:
			// ponIfNonProvisionedOnus — ignorado (não é offline)
		case 5:
			if v, ok := r["value_int"].(int); ok {
				a.online = v
			}
		case 6:
			if v, ok := r["value_int"].(int); ok {
				a.offline = v
			}
		}
	}
	out := make([]map[string]any, 0, len(byKey))
	for _, a := range byKey {
		if a.total <= 0 && (a.online > 0 || a.offline > 0) {
			a.total = a.online + a.offline
		}
		if a.offline == 0 && a.total > a.online {
			a.offline = a.total - a.online
		}
		name := strings.TrimSpace(a.name)
		if name == "" {
			name = "PON-" + a.key
		}
		out = append(out, map[string]any{
			"id":           a.key,
			"name":         name,
			"onu_total":    a.total,
			"onu_online":   a.online,
			"onu_offline":  a.offline,
			"status":       strings.TrimSpace(a.status),
			"source_slice": "datacom_snmp_table",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ni := strings.ToLower(strings.TrimSpace(anyString(out[i]["name"])))
		nj := strings.ToLower(strings.TrimSpace(anyString(out[j]["name"])))
		if ni == nj {
			return strings.TrimSpace(anyString(out[i]["id"])) < strings.TrimSpace(anyString(out[j]["id"]))
		}
		return ni < nj
	})
	return out
}

func ztePonIDFromIfLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if c := oltifderive.PonCompactFromPhy(s, s); c != "" {
		return c
	}
	return ""
}

func cleanSNMPOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}

func loadVendorOIDProfile(ctx context.Context, pool *pgxpool.Pool, brand, model string) oltVendorOIDProfile {
	if pool == nil {
		return oltVendorOIDProfile{}
	}
	brand = strings.TrimSpace(brand)
	model = strings.TrimSpace(model)
	var p oltVendorOIDProfile
	if brand != "" && model != "" {
		err := pool.QueryRow(ctx, `
			SELECT coalesce(onu_online_oid,''), coalesce(pon_status_oid,''), coalesce(transceiver_oid,''), coalesce(snmp_base_oid,'')
			FROM olt_vendor_models WHERE brand = $1 AND model = $2
		`, brand, model).Scan(&p.OnuOnlineOID, &p.PonStatusOID, &p.TransceiverOID, &p.SNMPBaseOID)
		if err == nil {
			return p
		}
	}
	if brand != "" {
		_ = pool.QueryRow(ctx, `
			SELECT coalesce(onu_online_oid,''), coalesce(pon_status_oid,''), coalesce(transceiver_oid,''), coalesce(snmp_base_oid,'')
			FROM olt_vendor_models WHERE brand = $1 AND model <> 'Padrão'
			ORDER BY model LIMIT 1
		`, brand).Scan(&p.OnuOnlineOID, &p.PonStatusOID, &p.TransceiverOID, &p.SNMPBaseOID)
		if p.OnuOnlineOID != "" || p.PonStatusOID != "" || p.TransceiverOID != "" || p.SNMPBaseOID != "" {
			return p
		}
		_ = pool.QueryRow(ctx, `
			SELECT coalesce(onu_online_oid,''), coalesce(pon_status_oid,''), coalesce(transceiver_oid,''), coalesce(snmp_base_oid,'')
			FROM olt_vendor_profiles WHERE upper(trim(brand)) = upper(trim($1))
		`, brand).Scan(&p.OnuOnlineOID, &p.PonStatusOID, &p.TransceiverOID, &p.SNMPBaseOID)
	}
	return p
}

func walkRootRows(ctx context.Context, host, community, root string) ([]map[string]any, bool, string) {
	root = cleanSNMPOID(root)
	if root == "" {
		return nil, false, ""
	}
	walk, trunc, note := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: root,
		Version: "2c", Timeout: 22 * time.Second, Retries: 0, MaxRows: 24000,
	})
	out := make([]map[string]any, 0, len(walk))
	for _, v := range walk {
		oid := cleanSNMPOID(v.OID)
		suffix := strings.TrimPrefix(oid, root)
		suffix = strings.TrimPrefix(suffix, ".")
		row := map[string]any{
			"oid":    oid,
			"suffix": suffix,
			"type":   v.Type,
			"value":  v.Value,
		}
		if n, err := strconv.Atoi(strings.TrimSpace(v.Value)); err == nil {
			row["value_int"] = n
		}
		out = append(out, row)
	}
	return out, trunc, note
}

func copyFloatPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func isLikelyMikrotikDevice(category, brand, model, description string) bool {
	hay := strings.ToLower(strings.TrimSpace(strings.Join([]string{category, brand, model, description}, " ")))
	if hay == "" {
		return false
	}
	if strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") {
		return true
	}
	// Cobertura para inventários com marca genérica, mas modelo MikroTik.
	return strings.Contains(hay, "ccr") || strings.Contains(hay, "crs") || strings.Contains(hay, "rb")
}

type ifaceTrafficRate struct {
	InBps  float64
	OutBps float64
}

type alertMetricThreshold struct {
	Operator string
	Warning  float64
	Critical float64
	HasWarn  bool
	HasCrit  bool
}

func loadGlobalMetricThreshold(ctx context.Context, pool *pgxpool.Pool, metricID string) (alertMetricThreshold, bool) {
	var out alertMetricThreshold
	if pool == nil || strings.TrimSpace(metricID) == "" {
		return out, false
	}
	var enabled bool
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT enabled, condition_json::text FROM alert_rules
		WHERE name = 'Limiar global de alertas' LIMIT 1
	`).Scan(&enabled, &raw)
	if err != nil || !enabled || len(raw) == 0 {
		return out, false
	}
	var root struct {
		Schema  string `json:"schema"`
		Metrics []struct {
			ID          string `json:"id"`
			Enabled     *bool  `json:"enabled"`
			Operator    string `json:"operator"`
			WarningMin  string `json:"warning_min"`
			CriticalMin string `json:"critical_min"`
		} `json:"metrics"`
	}
	if json.Unmarshal(raw, &root) != nil {
		return out, false
	}
	for _, m := range root.Metrics {
		if strings.TrimSpace(m.ID) != metricID {
			continue
		}
		if m.Enabled != nil && !*m.Enabled {
			return out, false
		}
		op := strings.ToLower(strings.TrimSpace(m.Operator))
		if op == "" {
			op = "gte"
		}
		out.Operator = op
		if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(m.WarningMin), ",", "."), 64); err == nil {
			out.Warning, out.HasWarn = f, true
		}
		if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(m.CriticalMin), ",", "."), 64); err == nil {
			out.Critical, out.HasCrit = f, true
		}
		return out, out.HasWarn || out.HasCrit
	}
	return out, false
}

func evalThresholdSeverity(v float64, t alertMetricThreshold) string {
	if t.Operator == "lte" {
		if t.HasCrit && v <= t.Critical {
			return "critical"
		}
		if t.HasWarn && v <= t.Warning {
			return "warning"
		}
		return "ok"
	}
	if t.HasCrit && v >= t.Critical {
		return "critical"
	}
	if t.HasWarn && v >= t.Warning {
		return "warning"
	}
	return "ok"
}

// openOrUpdateAlertWithMeta delega para alertstore (compatibilidade API).
func openOrUpdateAlertWithMeta(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, severity, alertType, message, ip, devName string, meta map[string]any) (created bool, alertID uuid.UUID, err error) {
	if meta == nil {
		meta = map[string]any{}
	}
	key, _ := meta["key"].(string)
	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: severity, AlertType: alertType,
		Message: message, IP: ip, DeviceName: devName, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
	}, nil)
	return res.Created, res.ID, err
}

func closeAlertByMetaKey(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType, key string) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertType,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{
			"resolved": "normalized", "source": "interface_snmp_refresh", "key": key,
		},
	})
}

func parseTrailingIndex(oid string) int {
	oid = strings.TrimSpace(strings.TrimPrefix(oid, "."))
	if oid == "" {
		return 0
	}
	p := strings.Split(oid, ".")
	if len(p) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(p[len(p)-1]))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parseIntCounter(v string) int64 {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if u, err := strconv.ParseUint(s, 10, 64); err == nil {
		if u > uint64(^uint64(0)>>1) {
			return int64(^uint64(0) >> 1)
		}
		return int64(u)
	}
	return 0
}

func walkCounterByIfIndex(ctx context.Context, host, community, rootOID string, timeout time.Duration, maxRows int) map[int]int64 {
	out := map[int]int64{}
	w, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: rootOID,
		Version: "2c", Timeout: timeout, Retries: 0, MaxRows: maxRows,
	})
	for _, v := range w {
		ix := parseTrailingIndex(v.OID)
		if ix <= 0 {
			continue
		}
		out[ix] = parseIntCounter(v.Value)
	}
	return out
}

func instantTrafficRatesByOID(ctx context.Context, host, community string, sampleGap time.Duration) (map[int]ifaceTrafficRate, float64) {
	in0 := walkCounterByIfIndex(ctx, host, community, "1.3.6.1.2.1.31.1.1.1.6", 14*time.Second, 20000)
	out0 := walkCounterByIfIndex(ctx, host, community, "1.3.6.1.2.1.31.1.1.1.10", 14*time.Second, 20000)
	if len(in0) == 0 && len(out0) == 0 {
		return nil, 0
	}
	start := time.Now()
	select {
	case <-ctx.Done():
		return nil, 0
	case <-time.After(sampleGap):
	}
	in1 := walkCounterByIfIndex(ctx, host, community, "1.3.6.1.2.1.31.1.1.1.6", 14*time.Second, 20000)
	out1 := walkCounterByIfIndex(ctx, host, community, "1.3.6.1.2.1.31.1.1.1.10", 14*time.Second, 20000)
	dt := time.Since(start).Seconds()
	if dt <= 0 {
		return nil, 0
	}
	rates := map[int]ifaceTrafficRate{}
	seen := map[int]struct{}{}
	for ix := range in1 {
		seen[ix] = struct{}{}
	}
	for ix := range out1 {
		seen[ix] = struct{}{}
	}
	for ix := range seen {
		aIn, okAIn := in0[ix]
		bIn, okBIn := in1[ix]
		aOut, okAOut := out0[ix]
		bOut, okBOut := out1[ix]
		if !okAIn || !okBIn || !okAOut || !okBOut {
			continue
		}
		if bIn < aIn || bOut < aOut {
			continue
		}
		rates[ix] = ifaceTrafficRate{
			InBps:  (float64(bIn-aIn) * 8.0) / dt,
			OutBps: (float64(bOut-aOut) * 8.0) / dt,
		}
	}
	return rates, dt
}

func computeInterfaceTrafficRates(currentRaw []byte, currentAt *time.Time, prevRaw []byte, prevAt *time.Time) (map[int]ifaceTrafficRate, float64) {
	if currentAt == nil || prevAt == nil || len(currentRaw) == 0 || len(prevRaw) == 0 {
		return nil, 0
	}
	dt := currentAt.Sub(*prevAt).Seconds()
	if dt <= 0 {
		return nil, 0
	}
	curRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(currentRaw))
	prevRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(prevRaw))
	if len(curRows) == 0 || len(prevRows) == 0 {
		return nil, 0
	}
	prevByIf := make(map[int]snmpifparse.IfRow, len(prevRows))
	for _, r := range prevRows {
		prevByIf[r.IfIndex] = r
	}
	out := map[int]ifaceTrafficRate{}
	for _, r := range curRows {
		p, ok := prevByIf[r.IfIndex]
		if !ok {
			continue
		}
		if r.InOctets < p.InOctets || r.OutOctets < p.OutOctets {
			// Reinício de contador ou wrap: ignora para evitar taxa negativa/espúria.
			continue
		}
		dIn := r.InOctets - p.InOctets
		dOut := r.OutOctets - p.OutOctets
		out[r.IfIndex] = ifaceTrafficRate{
			InBps:  (float64(dIn) * 8.0) / dt,
			OutBps: (float64(dOut) * 8.0) / dt,
		}
	}
	return out, dt
}

func loadLatestTwoInterfaceSnapshots(ctx context.Context, pool *pgxpool.Pool, devID uuid.UUID) (latestRaw []byte, latestAt *time.Time, prevRaw []byte, prevAt *time.Time, err error) {
	rows, err := pool.Query(ctx, `
		SELECT interfaces::text, collected_at
		FROM interface_snapshots
		WHERE device_id=$1
		ORDER BY collected_at DESC
		LIMIT 2
	`, devID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer rows.Close()
	var raws [][]byte
	var ats []time.Time
	for rows.Next() {
		var raw []byte
		var at time.Time
		if err := rows.Scan(&raw, &at); err != nil {
			return nil, nil, nil, nil, err
		}
		raws = append(raws, raw)
		ats = append(ats, at)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}
	if len(raws) == 0 {
		return nil, nil, nil, nil, nil
	}
	latestRaw = raws[0]
	lat := ats[0]
	latestAt = &lat
	if len(raws) > 1 {
		prevRaw = raws[1]
		p := ats[1]
		prevAt = &p
	}
	return latestRaw, latestAt, prevRaw, prevAt, nil
}

func upsertOltSnapshotAfterInterfaceRefresh(ctx context.Context, pool *pgxpool.Pool, devID uuid.UUID, derivedPons []map[string]any, sumPatch map[string]any) error {
	if pool == nil {
		return nil
	}
	sumT, ponsT := "{}", "[]"
	err := pool.QueryRow(ctx, `SELECT COALESCE(o.summary::text,'{}'), COALESCE(o.pons::text,'[]') FROM olt_snapshots o WHERE o.device_id=$1`, devID).Scan(&sumT, &ponsT)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	var existPons []map[string]any
	_ = json.Unmarshal([]byte(ponsT), &existPons)
	mergedPons := oltifderive.MergePonRowsForIfaceRefresh(existPons, derivedPons)
	oltifderive.ApplyPonOperStatusAll(mergedPons)
	newSum, err := oltifderive.MergeSummaryJSON([]byte(sumT), sumPatch)
	if err != nil {
		return err
	}
	pb, err := json.Marshal(mergedPons)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, $3::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET summary = $2::jsonb, pons = $3::jsonb, updated_at = now()
	`, devID, newSum, pb)
	return err
}

func walkJSONToSNMPVars(raw []byte) []probing.SNMPVar {
	return snapshotwalk.VarsFromJSON(raw)
}

func snapshotWalkTruncated(raw []byte) bool {
	var arr []struct {
		OID   string `json:"oid"`
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return false
	}
	for _, v := range arr {
		if strings.TrimSpace(v.OID) == "__netquasar.walk" && strings.TrimSpace(v.Value) == "truncated" {
			return true
		}
	}
	return false
}

func buildInterfaceMonitorPayload(ifaces []byte, collectedAt *time.Time, prevIfaces []byte, prevCollectedAt *time.Time) map[string]any {
	vars := walkJSONToSNMPVars(ifaces)
	rows := snmpifparse.BuildIfTable(vars)
	opt := snmpmikrotik.OpticalPowerByIfIndex(rows, vars)
	rateByIf, dtSec := computeInterfaceTrafficRates(ifaces, collectedAt, prevIfaces, prevCollectedAt)
	tab := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		op := opt[r.IfIndex]
		sfp := snmpmikrotik.IsSfpPort(r.DisplayName, r.Descr, op)
		row := map[string]any{
			"if_index":       r.IfIndex,
			"descr":          r.Descr,
			"if_name":        r.IfName,
			"display_name":   r.DisplayName,
			"speed_bps":      r.Speed,
			"admin_status":   snmpifparse.AdminStatusLabel(r.AdminStatus),
			"admin_status_n": r.AdminStatus,
			"oper_status":    snmpifparse.OperStatusLabel(r.OperStatus),
			"oper_status_n":  r.OperStatus,
			"in_octets":      r.InOctets,
			"out_octets":     r.OutOctets,
			"sfp":            sfp,
		}
		if op.TxDBm != nil {
			row["tx_dbm"] = *op.TxDBm
		}
		if op.RxDBm != nil {
			row["rx_dbm"] = *op.RxDBm
		}
		if rr, ok := rateByIf[r.IfIndex]; ok {
			row["in_bps"] = rr.InBps
			row["out_bps"] = rr.OutBps
		}
		tab = append(tab, row)
	}
	switchcollect.EnrichInterfaceVlans(tab, vars)
	var sensors []map[string]any
	for _, v := range vars {
		oid := strings.TrimSpace(v.OID)
		if strings.Contains(oid, "1.3.6.1.2.1.99.1.1.1.4.") {
			sensors = append(sensors, map[string]any{"oid": oid, "value": v.Value, "type": v.Type})
		}
	}
	out := map[string]any{"interface_table": tab, "optical_sensors": sensors}
	if dtSec > 0 {
		out["traffic_interval_seconds"] = dtSec
	}
	if prevCollectedAt != nil {
		out["traffic_prev_collected_at"] = prevCollectedAt.UTC().Format(time.RFC3339)
	}
	return out
}

