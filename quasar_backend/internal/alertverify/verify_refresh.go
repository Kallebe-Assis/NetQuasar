package alertverify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/bngcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
	"github.com/rs/zerolog"
)

func alertStillOpen(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID) bool {
	var open bool
	_ = pool.QueryRow(ctx, `SELECT closed_at IS NULL FROM alert_instances WHERE id=$1`, alertID).Scan(&open)
	return open
}

func refreshTelemetryForDevice(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) error {
	host, comm, _, _, _, _ := loadDeviceSnmpFields(ctx, pool, deviceID)
	if host == "" || comm == "" {
		return fmt.Errorf("host ou community SNMP em falta")
	}
	_, err := telemetryengine.CollectAndStore(ctx, pool, deviceID, host, comm)
	return err
}

func refreshInterfaceSnapshot(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID) {
	host, comm, cat, brand, model, desc := loadDeviceSnmpFields(ctx, pool, deviceID)
	if host == "" || comm == "" {
		return
	}
	monitorworker.CollectInterfaceSnapshotWorker(ctx, pool, log, deviceID, host, comm, cat, brand, model, desc)
}

func refreshOltCollect(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, mode string) {
	host, comm, _, brand, model, desc := loadDeviceSnmpFields(ctx, pool, deviceID)
	if host == "" || comm == "" || brand == "" || model == "" {
		return
	}
	var maxPons *int
	var mp *int
	_ = pool.QueryRow(ctx, `SELECT max_pons FROM devices WHERE id=$1`, deviceID).Scan(&mp)
	if mp != nil && *mp > 0 {
		maxPons = mp
	}
	_ = monitorworker.CollectOltVendorPeriodic(ctx, pool, log, deviceID, host, comm, desc, brand, model, maxPons, mode)
}

func refreshBngCollect(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) error {
	host, comm, _, _, _, _ := loadDeviceSnmpFields(ctx, pool, deviceID)
	if host == "" || comm == "" {
		return fmt.Errorf("host ou community SNMP em falta")
	}
	timeout := 120 * time.Second
	var toMs int
	_ = pool.QueryRow(ctx, `SELECT COALESCE(bng_timeout_ms, telemetry_timeout_ms, 120000) FROM monitoring_intervals WHERE id=1`).Scan(&toMs)
	if toMs >= 5000 {
		timeout = time.Duration(toMs) * time.Millisecond
	}
	_, err := bngcollect.CollectAndStorePeriodicMode(ctx, pool, deviceID, host, comm, timeout, "monitoring")
	return err
}

func verifyLatencyLive(ctx context.Context, pool *pgxpool.Pool, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{}
	var host string
	_ = pool.QueryRow(ctx, `SELECT host(ip)::text FROM devices WHERE id=$1`, row.DeviceID).Scan(&host)
	host = strings.TrimSpace(host)
	if host == "" {
		return false, "Equipamento sem IP — não foi possível medir latência.", collected, nil
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
	collected["reach_ok"] = ok
	latVal, hasLat := probe["latency_ms"]
	var lat int64
	switch x := latVal.(type) {
	case int64:
		lat = x
	case int:
		lat = int64(x)
	case float64:
		lat = int64(x)
	}
	if hasLat {
		collected["latency_ms"] = lat
	}
	if !ok || !hasLat {
		return false, "Sem resposta ICMP/TCP para medir latência.", collected, map[string]any{"reachability": probe}
	}
	var devCat string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(lower(trim(category)),'') FROM devices WHERE id=$1`, row.DeviceID).Scan(&devCat)
	th, label, thOK := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "latency_ms", devCat)
	if thOK {
		sev := alertthresholds.EvalMetricSeverity(float64(lat), th)
		if sev == "ok" {
			return true, fmt.Sprintf("%s: %d ms — dentro do limiar.", label, lat), collected, map[string]any{"curr_latency_ms": lat}
		}
		return false, fmt.Sprintf("%s: %d ms — ainda em %s.", label, lat, sev), collected, map[string]any{"curr_latency_ms": lat}
	}
	const calmMax int64 = 160
	const warnMin int64 = 280
	if lat < calmMax {
		return true, fmt.Sprintf("Latência %d ms — normalizada (heurística < %d ms).", lat, calmMax), collected, map[string]any{"curr_latency_ms": lat}
	}
	if lat >= warnMin {
		return false, fmt.Sprintf("Latência %d ms — ainda acima do limiar heurístico (%d ms).", lat, warnMin), collected, map[string]any{"curr_latency_ms": lat}
	}
	return false, fmt.Sprintf("Latência %d ms — entre limiares heurísticos.", lat), collected, map[string]any{"curr_latency_ms": lat}
}

func verifyInterfaceDown(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, row *alertRow) (bool, string, map[string]any, map[string]any) {
	collected := map[string]any{"refreshed": true}
	ifIndex := metaIfIndex(row.Meta)
	collected["if_index"] = ifIndex
	label := strings.TrimSpace(fmt.Sprint(row.Meta["display_name"]))
	if label == "" || label == "<nil>" {
		label = strings.TrimSpace(fmt.Sprint(row.Meta["if_name"]))
	}
	if label == "" || label == "<nil>" {
		if ifIndex > 0 {
			label = fmt.Sprintf("if%d", ifIndex)
		} else {
			label = "interface"
		}
	}
	collected["display_name"] = label

	refreshInterfaceSnapshot(ctx, pool, log, row.DeviceID)

	if !alertStillOpen(ctx, pool, row.ID) {
		return true, fmt.Sprintf("Interface %s voltou a operação UP.", label), collected, nil
	}

	oper := loadIfaceOperFromLatestSnapshot(ctx, pool, row.DeviceID, ifIndex)
	if oper != "" {
		collected["oper_status"] = oper
	}
	if strings.EqualFold(oper, "up") {
		return true, fmt.Sprintf("Interface %s está UP na coleta actual.", label), collected, map[string]any{"oper_status": oper}
	}
	if oper == "" {
		return false, fmt.Sprintf("Interface %s — sem estado operacional na coleta actual.", label), collected, nil
	}
	return false, fmt.Sprintf("Interface %s continua %s.", label, strings.ToUpper(oper)), collected, map[string]any{"oper_status": oper}
}

func loadIfaceOperFromLatestSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, ifIndex int) string {
	if ifIndex <= 0 {
		return ""
	}
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT interfaces::text FROM interface_snapshots
		WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
	`, deviceID).Scan(&raw)
	if err != nil || len(raw) == 0 {
		return ""
	}
	vars := snmpVarsFromSnapshotJSON(raw)
	rows := snmpifparse.BuildIfTable(vars)
	for _, r := range rows {
		if r.IfIndex == ifIndex {
			return snmpifparse.OperStatusLabel(r.OperStatus)
		}
	}
	return ""
}
