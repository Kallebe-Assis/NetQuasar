package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/rs/zerolog"
)

const (
	oltSnmpFirstTimeout    = 24 * time.Second
	oltSnmpFirstRetries    = 0
	oltSnmpRetryTimeout    = 36 * time.Second
	oltSnmpRetryRetries    = 1
	oltPonRetryPause       = 4 * time.Second
	oltBulkZeroRetryMinSum = 12.0 // soma de onu_online no snapshot anterior
)

// OltSnmpWalkPhase métricas de uma árvore IF-MIB (diagnóstico de lentidão).
type OltSnmpWalkPhase struct {
	OID       string
	RowCount  int
	Duration  time.Duration
	Err       string
	Truncated bool
}

// OltIfMibWalkBundle resultado agregado dos dois walks SNMP usados para derivar ONUs/PON.
type OltIfMibWalkBundle struct {
	Vars      []probing.SNMPVar
	Truncated bool
	Err       string
	Phases    []OltSnmpWalkPhase
}

func walkOltIfMibTables(ctx context.Context, host, community string, timeout time.Duration, retries int) OltIfMibWalkBundle {
	type step struct {
		oid string
		max int
	}
	steps := []step{
		{"1.3.6.1.2.1.2.2.1", 14000},
		{"1.3.6.1.2.1.31.1.1.1", 20000},
	}
	all := make([]probing.SNMPVar, 0, 8000)
	var phases []OltSnmpWalkPhase
	truncAny := false
	var parts []string
	for _, st := range steps {
		t0 := time.Now()
		walk, tr, e := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: st.oid,
			Version: "2c", Timeout: timeout, Retries: retries, MaxRows: st.max,
		})
		phases = append(phases, OltSnmpWalkPhase{
			OID: st.oid, RowCount: len(walk), Duration: time.Since(t0),
			Err: e, Truncated: tr,
		})
		truncAny = truncAny || tr
		rootTag := "if_mib"
		if strings.Contains(st.oid, "31") {
			rootTag = "ifx"
		}
		if strings.TrimSpace(e) != "" {
			parts = append(parts, rootTag+":"+e)
		}
		all = append(all, walk...)
	}
	errMsg := strings.Join(parts, "; ")
	return OltIfMibWalkBundle{Vars: all, Truncated: truncAny, Err: errMsg, Phases: phases}
}

func sumOnuOnlineInPonRows(pons []map[string]any) float64 {
	var s float64
	for _, p := range pons {
		if n, ok := oltifderive.OnuOnlineFromRow(p); ok {
			s += n
		}
	}
	return s
}

func applyMaxPonsLimitMapRows(pons []map[string]any, maxPons *int) []map[string]any {
	if maxPons == nil || *maxPons <= 0 {
		return pons
	}
	return oltifderive.FilterPonRowsByMaxSlots(pons, *maxPons)
}

// shouldSecondOltIfWalk relê IF-MIB quando o walk falhou, truncou ou a agregação parece perda em massa vs. snapshot anterior.
func shouldSecondOltIfWalk(prevPons []map[string]any, derivedPons []map[string]any, truncated bool, walkErr string) bool {
	if strings.TrimSpace(walkErr) != "" {
		return true
	}
	if truncated {
		return true
	}
	prevSum := sumOnuOnlineInPonRows(prevPons)
	curSum := sumOnuOnlineInPonRows(derivedPons)
	if prevSum >= oltBulkZeroRetryMinSum && curSum <= 0.5 {
		return true
	}
	if prevSum >= 30 && curSum > 0.5 && curSum <= prevSum*0.05 {
		return true
	}
	return false
}

func logOltPonSnmpWalk(log *zerolog.Logger, deviceID uuid.UUID, host, pass string, b OltIfMibWalkBundle) {
	if log == nil {
		return
	}
	e := log.Info().
		Str("component", "olt_pon_collect").
		Str("pass", pass).
		Str("device_id", deviceID.String()).
		Str("host", host).
		Bool("snmp_truncated", b.Truncated).
		Str("snmp_walk_err", b.Err).
		Int("snmp_pdu_total", len(b.Vars))
	if len(b.Phases) > 0 {
		p := b.Phases[0]
		e = e.Int("if_mib_rows", p.RowCount).Int64("if_mib_walk_ms", p.Duration.Milliseconds()).Bool("if_mib_trunc", p.Truncated)
		if p.Err != "" {
			e = e.Str("if_mib_walk_err", p.Err)
		}
	}
	if len(b.Phases) > 1 {
		p := b.Phases[1]
		e = e.Int("ifx_rows", p.RowCount).Int64("ifx_walk_ms", p.Duration.Milliseconds()).Bool("ifx_trunc", p.Truncated)
		if p.Err != "" {
			e = e.Str("ifx_walk_err", p.Err)
		}
	}
	e.Msg("OLT PON: walk SNMP (IF-MIB + IF-MIB-X)")
}

// CollectOltPonAndEvaluate executa coleta periódica de ONUs por PON (derive IF-MIB) e avalia alarmes.
// Omitido para fabricantes onde o derive IF-MIB é incorrecto/incompleto (ex.: ZTE, Datacom — usar refresh OLT/API).
func CollectOltPonAndEvaluate(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, host, community, devDesc, category, brand, model string, maxPons *int) {
	if pool == nil || strings.TrimSpace(host) == "" || strings.TrimSpace(community) == "" {
		return
	}
	if !OltUsesIfDerivedPonSnapshots(category, brand, model) {
		return
	}
	if !alertthresholds.OltOnuQuantityAlertsEnabled(ctx, pool) {
		return
	}

	tAll := time.Now()

	var prevPonsRaw []byte
	var prevSumRaw []byte
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(pons::text,'[]'), COALESCE(summary::text,'{}')
		FROM olt_snapshots WHERE device_id=$1
	`, deviceID).Scan(&prevPonsRaw, &prevSumRaw)
	prevMaps := oltifderive.PonsJSONToMaps(prevPonsRaw)
	prevMaps = applyMaxPonsLimitMapRows(prevMaps, maxPons)
	prevSumm := oltifderive.SummaryJSONBytesToMap(prevSumRaw)

	truncated, walkErr := false, ""
	var vars []probing.SNMPVar
	b1 := walkOltIfMibTables(ctx, host, community, oltSnmpFirstTimeout, oltSnmpFirstRetries)
	vars, truncated, walkErr = b1.Vars, b1.Truncated, b1.Err
	logOltPonSnmpWalk(log, deviceID, host, "1_primary", b1)
	if len(vars) == 0 {
		if sumOnuOnlineInPonRows(prevMaps) < oltBulkZeroRetryMinSum {
			return
		}
		select {
		case <-time.After(oltPonRetryPause):
		case <-ctx.Done():
			return
		}
		b1b := walkOltIfMibTables(ctx, host, community, oltSnmpRetryTimeout, oltSnmpRetryRetries)
		vars, truncated, walkErr = b1b.Vars, b1b.Truncated, b1b.Err
		logOltPonSnmpWalk(log, deviceID, host, "1b_empty_retry", b1b)
		if len(vars) == 0 {
			return
		}
	}

	var pons []map[string]any
	var sumPatch map[string]any
	secondCollectDone := false
deriveLoop:
	for {
		td := time.Now()
		ifRows := snmpifparse.BuildIfTable(vars)
		optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, vars)
		pons, sumPatch = oltifderive.DeriveFromIfRows(ifRows, optMap)
		pons = applyMaxPonsLimitMapRows(pons, maxPons)
		if log != nil {
			log.Info().
				Str("component", "olt_pon_collect").
				Str("device_id", deviceID.String()).
				Str("host", host).
				Int64("derive_ms", time.Since(td).Milliseconds()).
				Int("pon_segments", len(pons)).
				Float64("derived_onu_online_sum", sumOnuOnlineInPonRows(pons)).
				Msg("OLT PON: derive IF-MIB → PON")
		}
		if len(pons) == 0 {
			return
		}
		if !secondCollectDone && shouldSecondOltIfWalk(prevMaps, pons, truncated, walkErr) {
			secondCollectDone = true
			if log != nil {
				prevS := sumOnuOnlineInPonRows(prevMaps)
				curS := sumOnuOnlineInPonRows(pons)
				log.Info().
					Str("component", "olt_pon_collect").
					Str("device_id", deviceID.String()).
					Str("host", host).
					Bool("snmp_truncated", truncated).
					Str("snmp_walk_err", walkErr).
					Float64("prev_online_sum", prevS).
					Float64("derived_online_sum_before_retry", curS).
					Msg("OLT PON: segunda coleta SNMP (leitura suspeita vs snapshot)")
			}
			select {
			case <-time.After(oltPonRetryPause):
			case <-ctx.Done():
				break deriveLoop
			}
			b2 := walkOltIfMibTables(ctx, host, community, oltSnmpRetryTimeout, oltSnmpRetryRetries)
			if len(b2.Vars) > 0 {
				logOltPonSnmpWalk(log, deviceID, host, "2_suspect_retry", b2)
				vars, truncated, walkErr = b2.Vars, b2.Truncated, b2.Err
				continue
			}
		}
		break
	}

	incomplete := truncated || len(pons) < len(prevMaps)
	stabMaps, stabPatch := oltifderive.StabilizePonSnapshotRows(prevMaps, pons, prevSumm, incomplete)
	pons = stabMaps
	pons = applyMaxPonsLimitMapRows(pons, maxPons)

	oltifderive.ApplyPonOperStatusAll(pons)
	alertthresholds.EvaluateOltOnuQuantityDeltaAlerts(ctx, pool, log, deviceID, devDesc, host, prevMaps, pons, "monitor_worker")
	alertthresholds.EvaluateOltOnuOpticalFromPons(ctx, pool, log, deviceID, devDesc, host, pons)

	summary := map[string]any{
		"if_mib_derived_at":    time.Now().UTC().Format(time.RFC3339),
		"if_mib_merge_applied": true,
		"derived_from_worker":  true,
	}
	for k, v := range sumPatch {
		summary[k] = v
	}
	for k, v := range stabPatch {
		summary[k] = v
	}
	sb, _ := json.Marshal(summary)
	pb, _ := json.Marshal(pons)
	_, _ = pool.Exec(ctx, `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, $3::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET
			summary = COALESCE(olt_snapshots.summary, '{}'::jsonb) || $2::jsonb,
			pons = $3::jsonb,
			updated_at = now()
	`, deviceID, sb, pb)

	if log != nil {
		log.Info().
			Str("component", "olt_pon_collect").
			Str("device_id", deviceID.String()).
			Str("host", host).
			Int64("total_collect_ms", time.Since(tAll).Milliseconds()).
			Int("pon_rows_stored", len(pons)).
			Float64("onu_online_sum_stored", sumOnuOnlineInPonRows(pons)).
			Msg("OLT PON: ciclo concluído (snapshot gravado)")
	}
}
