package alertverify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/rs/zerolog"
)

// VerifyResult resultado de uma verificação manual.
type VerifyResult struct {
	AlertID     uuid.UUID      `json:"alert_id"`
	StillActive bool           `json:"still_active"`
	Resolved    bool           `json:"resolved"`
	Summary     string         `json:"summary"`
	Collected   map[string]any `json:"collected"`
	UpdatedMeta map[string]any `json:"updated_meta,omitempty"`
}

type alertRow struct {
	ID         uuid.UUID
	DeviceID   uuid.UUID
	AlertType  string
	Message    string
	Severity   string
	IP         string
	DeviceName string
	Meta       map[string]any
	MetaKey    string
}

func loadOpenAlert(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID) (*alertRow, error) {
	var r alertRow
	var ip, dname *string
	var metaRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, device_id, alert_type, message, severity, ip, device_name, COALESCE(meta::text,'{}')
		FROM alert_instances WHERE id = $1 AND closed_at IS NULL
	`, alertID).Scan(&r.ID, &r.DeviceID, &r.AlertType, &r.Message, &r.Severity, &ip, &dname, &metaRaw)
	if err != nil {
		return nil, err
	}
	r.IP = ptrStr(ip)
	r.DeviceName = ptrStr(dname)
	_ = json.Unmarshal(metaRaw, &r.Meta)
	if r.Meta == nil {
		r.Meta = map[string]any{}
	}
	r.MetaKey = alertignore.MetaKeyFromAlert(r.AlertType, r.Meta)
	return &r, nil
}

// VerifyAlert reavalia um alerta aberto e actualiza message/meta ou fecha se normalizado.
func VerifyAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, alertID uuid.UUID) (VerifyResult, error) {
	out := VerifyResult{AlertID: alertID, Collected: map[string]any{}}
	row, err := loadOpenAlert(ctx, pool, alertID)
	if err != nil {
		return out, err
	}
	ignoreID := alertignore.FindActiveIgnoreID(ctx, pool, row.DeviceID, row.AlertType, row.MetaKey)

	resolved, summary, collected, patch := evaluateAlert(ctx, pool, log, row)
	out.Collected = collected
	out.Summary = summary
	out.StillActive = !resolved
	out.Resolved = resolved

	if resolved {
		metaClose, _ := json.Marshal(map[string]any{"resolved": "verify_normalized", "source": "alert_verify", "verify": collected})
		var closedID uuid.UUID
		var msg string
		err = pool.QueryRow(ctx, `
			UPDATE alert_instances SET closed_at = now(), meta = COALESCE(meta,'{}'::jsonb) || $2::jsonb
			WHERE id = $1::uuid AND closed_at IS NULL
			RETURNING id, message
		`, alertID, metaClose).Scan(&closedID, &msg)
		if err == nil {
			head := alertnotify.ResolutionHeadlineForAlertType(row.AlertType)
			alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, closedID, head, msg)
		}
	} else {
		if v, ok := collected["dbm"]; ok {
			if patch == nil {
				patch = map[string]any{}
			}
			patch["dbm"] = v
			patch["value"] = v
		}
		if len(patch) > 0 {
			patch["source"] = "alert_verify"
			patch["verify"] = collected
			metaRaw, _ := json.Marshal(patch)
			newMsg := buildUpdatedMessage(row, collected)
			_, _ = pool.Exec(ctx, `
				UPDATE alert_instances SET message = $3, meta = COALESCE(meta,'{}'::jsonb) || $2::jsonb
				WHERE id = $1::uuid AND closed_at IS NULL
			`, alertID, metaRaw, newMsg)
			out.UpdatedMeta = patch
		}
	}

	verifyStore := map[string]any{"summary": summary, "resolved": resolved, "collected": collected, "at": time.Now().UTC().Format(time.RFC3339)}
	alertignore.PatchVerifyResult(ctx, pool, ignoreID, row.DeviceID, row.AlertType, row.MetaKey, verifyStore)
	return out, nil
}

func evaluateAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, row *alertRow) (resolved bool, summary string, collected map[string]any, patch map[string]any) {
	collected = map[string]any{}
	patch = map[string]any{}
	switch row.AlertType {
	case "ping_unreachable":
		return verifyPing(ctx, pool, row)
	case "latency_high":
		return verifyLatency(ctx, pool, row)
	case "telemetry_threshold":
		return verifyTelemetry(ctx, pool, row)
	case "uptime_restart_low":
		return verifyUptime(ctx, pool, row)
	case "olt_onu_drop", "olt_onu_rise":
		return verifyOltOnuDelta(ctx, pool, row)
	case "olt_onu_rx", "olt_onu_tx":
		return verifyOltOnuOptical(ctx, pool, row)
	case "mikrotik_sfp_tx", "mikrotik_sfp_rx":
		return verifySfp(ctx, pool, log, row)
	default:
		_ = log
		collected["note"] = "verificação genérica via cache"
		var reachOK *bool
		var lat *int64
		_ = pool.QueryRow(ctx, `SELECT reach_ok, latency_ms FROM device_probe_cache WHERE device_id=$1`, row.DeviceID).Scan(&reachOK, &lat)
		if reachOK != nil {
			collected["reach_ok"] = *reachOK
		}
		if lat != nil {
			collected["latency_ms"] = *lat
		}
		return false, "Dados actualizados do cache de monitorização.", collected, patch
	}
}

func verifyPing(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	var host string
	_ = pool.QueryRow(ctx, `SELECT host(ip)::text FROM devices WHERE id=$1`, row.DeviceID).Scan(&host)
	host = strings.TrimSpace(host)
	if host == "" {
		return false, "Equipamento sem IP — não foi possível pingar.", collected, nil
	}
	var pingTimeoutMs int
	_ = pool.QueryRow(ctx, `SELECT ping_timeout_ms FROM monitoring_intervals WHERE id=1`).Scan(&pingTimeoutMs)
	if pingTimeoutMs < 1000 {
		pingTimeoutMs = 3000
	}
	to := time.Duration(pingTimeoutMs) * time.Millisecond
	perCtx, cancel := context.WithTimeout(ctx, to+time.Second)
	defer cancel()
	probe := probing.HostReachability(perCtx, host, "443", to*2/3, to/3, probing.ClampICMPPayloadBytes(56))
	collected["probe"] = probe
	ok, _ := probe["ok"].(bool)
	if ok {
		return true, "Equipamento respondeu ao ping/TCP — alerta pode ser encerrado.", collected, nil
	}
	patch := map[string]any{"reachability": probe}
	return false, "Equipamento continua sem resposta ICMP/TCP.", collected, patch
}

func verifyLatency(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	var lat *int64
	var reachOK bool
	_ = pool.QueryRow(ctx, `SELECT latency_ms, reach_ok FROM device_probe_cache WHERE device_id=$1`, row.DeviceID).Scan(&lat, &reachOK)
	if lat != nil {
		collected["latency_ms"] = *lat
	}
	collected["reach_ok"] = reachOK
	var devCat string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(lower(trim(category)),'') FROM devices WHERE id=$1`, row.DeviceID).Scan(&devCat)
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", devCat)
	if !ok {
		return false, fmt.Sprintf("Latência actual: %v ms (sem limiar global).", collected["latency_ms"]), collected, map[string]any{"curr_latency_ms": lat}
	}
	if lat == nil {
		return false, "Sem leitura de latência no cache.", collected, nil
	}
	sev := severityGte(float64(*lat), th)
	if sev == "ok" {
		return true, fmt.Sprintf("Latência %.0f ms dentro do limiar.", float64(*lat)), collected, map[string]any{"curr_latency_ms": *lat}
	}
	return false, fmt.Sprintf("Latência %.0f ms — ainda acima do limiar (%s).", float64(*lat), sev), collected, map[string]any{"curr_latency_ms": *lat}
}

func verifyTelemetry(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	metricID, _ := row.Meta["metric_id"].(string)
	if metricID == "" {
		metricID = strings.TrimPrefix(row.MetaKey, "telemetry:")
	}
	var val *float64
	_ = pool.QueryRow(ctx, `
		SELECT value FROM telemetry_samples
		WHERE device_id=$1 AND metric_id=$2
		ORDER BY sampled_at DESC LIMIT 1
	`, row.DeviceID, metricID).Scan(&val)
	if val != nil {
		collected["value"] = *val
		collected["metric_id"] = metricID
	}
	var devCat string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(lower(trim(category)),'') FROM devices WHERE id=$1`, row.DeviceID).Scan(&devCat)
	th, label, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, metricID, devCat)
	if !ok || val == nil {
		return false, "Sem amostra recente de telemetria.", collected, map[string]any{"value": val, "metric_id": metricID}
	}
	sev := severityGte(*val, th)
	if sev == "ok" {
		return true, fmt.Sprintf("%s: %.2f — dentro do limiar.", label, *val), collected, map[string]any{"value": *val, "metric_id": metricID}
	}
	return false, fmt.Sprintf("%s: %.2f — ainda em %s.", label, *val, sev), collected, map[string]any{"value": *val, "metric_id": metricID}
}

func verifyUptime(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	var mins *float64
	_ = pool.QueryRow(ctx, `
		SELECT value FROM telemetry_samples
		WHERE device_id=$1 AND metric_id='uptime_minutes'
		ORDER BY sampled_at DESC LIMIT 1
	`, row.DeviceID).Scan(&mins)
	if mins != nil {
		collected["uptime_minutes"] = *mins
	}
	var devCat string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(lower(trim(category)),'') FROM devices WHERE id=$1`, row.DeviceID).Scan(&devCat)
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "uptime_minutes", devCat)
	if !ok || mins == nil {
		return false, "Sem leitura de uptime.", collected, nil
	}
	sev := severityGte(*mins, th)
	if sev == "ok" {
		return true, fmt.Sprintf("Uptime %.0f min — acima do limiar mínimo.", *mins), collected, map[string]any{"uptime_minutes": *mins}
	}
	return false, fmt.Sprintf("Uptime %.0f min — ainda abaixo do esperado.", *mins), collected, map[string]any{"uptime_minutes": *mins}
}

func verifyOltOnuDelta(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	pon := strings.TrimSpace(fmt.Sprint(row.Meta["pon"]))
	var ponsRaw []byte
	_ = pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1`, row.DeviceID).Scan(&ponsRaw)
	collected["pon"] = pon
	collected["snapshot_at"] = time.Now().UTC().Format(time.RFC3339)
	var arr []map[string]any
	_ = json.Unmarshal(ponsRaw, &arr)
	for _, p := range arr {
		k := strings.TrimSpace(fmt.Sprint(p["id"]))
		if k == "" {
			k = strings.TrimSpace(fmt.Sprint(p["pon_compact"]))
		}
		if k != pon && pon != "" {
			continue
		}
		collected["onu_online"] = p["onu_online"]
		collected["onu_offline"] = p["onu_offline"]
		break
	}
	prev, _ := row.Meta["prev_online"].(float64)
	cur, _ := row.Meta["curr_online"].(float64)
	if v, ok := collected["onu_online"].(float64); ok {
		cur = v
	} else if v, ok := collected["onu_online"].(json.Number); ok {
		f, _ := v.Float64()
		cur = f
	}
	if row.AlertType == "olt_onu_drop" && cur >= prev {
		return true, fmt.Sprintf("PON %s: %.0f ONUs online (normalizado vs %.0f).", pon, cur, prev), collected, map[string]any{"curr_online": cur, "prev_online": prev}
	}
	if row.AlertType == "olt_onu_rise" && cur <= prev {
		return true, fmt.Sprintf("PON %s: variação estabilizada (%.0f online).", pon, cur), collected, map[string]any{"curr_online": cur}
	}
	return false, fmt.Sprintf("PON %s: %.0f ONUs online na última coleta OLT.", pon, cur), collected, map[string]any{"curr_online": cur, "onu_online": collected["onu_online"]}
}

func verifyOltOnuOptical(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	if v, ok := row.Meta["value"].(float64); ok {
		collected["previous_dbm"] = v
	}
	pon := strings.TrimSpace(fmt.Sprint(row.Meta["pon"]))
	var ponsRaw []byte
	_ = pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1`, row.DeviceID).Scan(&ponsRaw)
	var arr []any
	_ = json.Unmarshal(ponsRaw, &arr)
	for _, x := range arr {
		p, ok := x.(map[string]any)
		if !ok {
			continue
		}
		k := strings.TrimSpace(fmt.Sprint(p["id"]))
		if pon != "" && k != pon {
			continue
		}
		if row.AlertType == "olt_onu_rx" {
			collected["dbm"] = p["rx_dbm"]
		} else {
			collected["dbm"] = p["tx_dbm"]
		}
		break
	}
	metricID := "olt_onu_rx_dbm"
	if row.AlertType == "olt_onu_tx" {
		metricID = "olt_onu_tx_dbm"
	}
	th, label, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, metricID, "olt")
	dbm := toFloat(collected["dbm"])
	if !ok || dbm == nil {
		return false, "Sem leitura óptica recente na OLT.", collected, map[string]any{"dbm": dbm}
	}
	sev := severityGte(*dbm, th)
	if sev == "ok" {
		return true, fmt.Sprintf("%s PON %s: %.2f dBm — dentro do limiar.", label, pon, *dbm), collected, map[string]any{"dbm": *dbm, "value": *dbm}
	}
	return false, fmt.Sprintf("%s PON %s: %.2f dBm — ainda abaixo do limiar.", label, pon, *dbm), collected, map[string]any{"dbm": *dbm, "value": *dbm}
}

func verifySfp(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	wantRx := row.AlertType == "mikrotik_sfp_rx"
	ifIndex := metaIfIndex(row.Meta)
	dbm, snapAt := loadSfpDbmFromSnapshot(ctx, pool, row.DeviceID, ifIndex, wantRx)
	if dbm == nil && ifIndex > 0 {
		host, comm, cat, brand, model, desc := loadDeviceSnmpFields(ctx, pool, row.DeviceID)
		if host != "" && comm != "" {
			monitorworker.CollectInterfaceSnapshotWorker(ctx, pool, log, row.DeviceID, host, comm, cat, brand, model, desc)
			collected["snapshot_refreshed"] = true
			dbm, snapAt = loadSfpDbmFromSnapshot(ctx, pool, row.DeviceID, ifIndex, wantRx)
		}
	}
	if snapAt != "" {
		collected["snapshot_at"] = snapAt
	}
	if dbm == nil {
		if old := toFloat(row.Meta["dbm"]); old != nil {
			dbm = old
			collected["dbm"] = *dbm
			collected["note"] = "sem leitura nova — valor do alerta"
		} else {
			return false, "Sem leitura óptica SFP (snapshot ou coleta).", collected, nil
		}
	} else {
		collected["dbm"] = *dbm
	}
	metricID := "mikrotik_sfp_rx_dbm"
	if !wantRx {
		metricID = "mikrotik_sfp_tx_dbm"
	}
	var devCat string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(lower(trim(category)),'') FROM devices WHERE id=$1`, row.DeviceID).Scan(&devCat)
	th, label, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, metricID, devCat)
	if !ok {
		return false, fmt.Sprintf("Potência actual %.2f dBm (sem limiar global).", *dbm), collected, map[string]any{"dbm": *dbm, "value": *dbm}
	}
	sev := severityGte(*dbm, th)
	patch := map[string]any{"dbm": *dbm, "value": *dbm}
	if sev == "ok" {
		return true, fmt.Sprintf("%s: %.2f dBm — dentro do limiar.", label, *dbm), collected, patch
	}
	return false, fmt.Sprintf("%s: %.2f dBm — ainda fora do limiar.", label, *dbm), collected, patch
}

func metaIfIndex(meta map[string]any) int {
	if meta == nil {
		return 0
	}
	switch x := meta["if_index"].(type) {
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case int:
		return x
	case int64:
		return int(x)
	}
	return 0
}

func loadDeviceSnmpFields(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (host, community, category, brand, model, description string) {
	var ip, devComm, defComm *string
	_ = pool.QueryRow(ctx, `
		SELECT host(d.ip)::text, d.snmp_community,
			COALESCE(trim(d.category),''), COALESCE(trim(d.brand),''), COALESCE(trim(d.model),''),
			COALESCE(trim(d.description),'')
		FROM devices d WHERE d.id=$1
	`, deviceID).Scan(&ip, &devComm, &category, &brand, &model, &description)
	if ip != nil {
		host = strings.TrimSpace(*ip)
	}
	if devComm != nil && strings.TrimSpace(*devComm) != "" {
		community = strings.TrimSpace(*devComm)
	} else {
		_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
		if defComm != nil {
			community = strings.TrimSpace(*defComm)
		}
	}
	return host, community, category, brand, model, description
}

func loadSfpDbmFromSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, ifIndex int, wantRx bool) (*float64, string) {
	if pool == nil || ifIndex <= 0 {
		return nil, ""
	}
	var raw []byte
	var at time.Time
	err := pool.QueryRow(ctx, `
		SELECT interfaces::text, collected_at FROM interface_snapshots
		WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
	`, deviceID).Scan(&raw, &at)
	if err != nil || len(raw) == 0 {
		return nil, ""
	}
	vars := snmpVarsFromSnapshotJSON(raw)
	if len(vars) == 0 {
		return nil, ""
	}
	ifRows := snmpifparse.BuildIfTable(vars)
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, vars)
	if op, ok := optMap[ifIndex]; ok {
		if wantRx && op.RxDBm != nil {
			v := *op.RxDBm
			return &v, at.UTC().Format(time.RFC3339)
		}
		if !wantRx && op.TxDBm != nil {
			v := *op.TxDBm
			return &v, at.UTC().Format(time.RFC3339)
		}
	}
	return nil, at.UTC().Format(time.RFC3339)
}

func snmpVarsFromSnapshotJSON(raw []byte) []probing.SNMPVar {
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	out := make([]probing.SNMPVar, 0, len(arr))
	for _, m := range arr {
		oid, _ := m["oid"].(string)
		if strings.TrimSpace(oid) == "" || strings.HasPrefix(oid, "__") {
			continue
		}
		val := fmt.Sprint(m["value"])
		typ, _ := m["type"].(string)
		out = append(out, probing.SNMPVar{OID: oid, Value: val, Type: typ})
	}
	return out
}

func severityGte(v float64, th alertthresholds.GteMetricThreshold) string {
	if th.Operator == "lte" {
		if th.HasCrit && v <= th.Critical {
			return "critical"
		}
		if th.HasWarn && v <= th.Warning {
			return "warning"
		}
		return "ok"
	}
	if th.HasCrit && v >= th.Critical {
		return "critical"
	}
	if th.HasWarn && v >= th.Warning {
		return "warning"
	}
	return "ok"
}

func toFloat(v any) *float64 {
	switch x := v.(type) {
	case float64:
		return &x
	case json.Number:
		f, err := x.Float64()
		if err == nil {
			return &f
		}
	}
	return nil
}

func buildUpdatedMessage(row *alertRow, collected map[string]any) string {
	if row.AlertType == "ping_unreachable" {
		return row.Message
	}
	if v, ok := collected["latency_ms"]; ok {
		return fmt.Sprintf("%s (%s): latência actual %v ms.", row.DeviceName, row.IP, v)
	}
	if v, ok := collected["dbm"]; ok {
		if f := toFloat(v); f != nil {
			return fmt.Sprintf("%s (%s): potência actual %.2f dBm.", row.DeviceName, row.IP, *f)
		}
	}
	if v, ok := collected["value"]; ok {
		return fmt.Sprintf("%s (%s): valor actual %v.", row.DeviceName, row.IP, v)
	}
	if v, ok := collected["onu_online"]; ok {
		pon := collected["pon"]
		if f := toFloat(v); f != nil {
			return fmt.Sprintf("%s (%s): PON %v — %.0f ONUs online na última coleta.", row.DeviceName, row.IP, pon, *f)
		}
	}
	return row.Message
}

// VerifyAllOpen verifica todos os alertas abertos (excepto ignorados) e devolve contagens.
func VerifyAllOpen(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, limit int) (verified, resolved int, err error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := pool.Query(ctx, `
		SELECT a.id FROM alert_instances a
		WHERE a.closed_at IS NULL
		`+alertignore.SQLActiveIgnoreNotExists+`
		ORDER BY a.active_since DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		res, err := VerifyAlert(ctx, pool, log, id)
		if err != nil {
			continue
		}
		verified++
		if res.Resolved {
			resolved++
		}
	}
	return verified, resolved, nil
}

// RevalidatePingAlerts fecha ping_unreachable quando probe OK (extraído de alertsRevalidate).
func RevalidatePingAlerts(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) (int, error) {
	rows, err := pool.Query(ctx, `
		UPDATE alert_instances a SET
			closed_at = now(),
			meta = COALESCE(a.meta, '{}'::jsonb) || CASE
				WHEN COALESCE(c.reach_ok, false) THEN '{"resolved":"revalidate_probe_ok","source":"alerts_verify"}'::jsonb
				ELSE '{"resolved":"revalidate_device_not_monitored","source":"alerts_verify"}'::jsonb
			END
		FROM devices d
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE a.device_id = d.id
		AND a.closed_at IS NULL
		AND a.alert_type = 'ping_unreachable'
		AND (
			COALESCE(c.reach_ok, false)
			OR `+monitorworker.SQLDeviceEligibleForPingAlertsNotMet+`
		)
		RETURNING a.id, a.alert_type, a.message, (COALESCE(c.reach_ok, false)) AS probe_ok
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id uuid.UUID
		var atype, msg string
		var probeOK bool
		if err := rows.Scan(&id, &atype, &msg, &probeOK); err != nil {
			continue
		}
		n++
		if probeOK {
			head := alertnotify.ResolutionHeadlineForAlertType(atype)
			alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, id, head, msg)
		}
	}
	return n, rows.Err()
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}
