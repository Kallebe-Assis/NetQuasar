package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/rs/zerolog"
)

// CollectOltVendorPeriodic coleta ONUs/PON via perfil do fabricante (VSOL, ZTE, Datacom, etc.).
func CollectOltVendorPeriodic(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	host, community, devDesc, brand, model string,
	maxPons *int,
	onuCollectMode string,
) OltCollectOutcome {
	out := OltCollectOutcome{Mode: "vendor_profile"}
	if pool == nil || strings.TrimSpace(host) == "" || strings.TrimSpace(community) == "" {
		out.Reason = "host ou community SNMP em falta"
		return out
	}
	brand = strings.TrimSpace(brand)
	model = strings.TrimSpace(model)
	if brand == "" || model == "" {
		out.Reason = "marca ou modelo OLT em falta"
		return out
	}
	profile, err := oltcollect.LoadVendorProfile(ctx, pool, brand, model)
	if err != nil {
		out.Reason = "perfil do fabricante não encontrado"
		if log != nil {
			log.Debug().Err(err).Str("device", deviceID.String()).Str("brand", brand).Str("model", model).
				Msg("OLT periódica: perfil não encontrado")
		}
		return out
	}
	if strings.TrimSpace(onuCollectMode) != "" {
		profile.OnuMetrics = oltcollect.FilterOnuMetricsByMode(profile.OnuMetrics, onuCollectMode)
	}
	periodicSteps := periodicCollectionSteps(profile, brand, onuCollectMode)
	steps := oltcollect.StepsForScope(periodicSteps, oltcollect.ScopeFull)
	if len(steps) == 0 {
		out.Reason = "perfil sem passos de coleta ONU activos"
		return out
	}

	var prevPonsRaw []byte
	_ = pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1`, deviceID).Scan(&prevPonsRaw)
	prevMaps := oltifderive.PonsJSONToMaps(prevPonsRaw)

	maxPonsVal := 0
	if maxPons != nil && *maxPons > 0 {
		maxPonsVal = *maxPons
	}

	cfg, _ := loadClampMonitoringIntervals(ctx, pool)
	telnetTO := cfg.oltOnuTelnetTimeout()
	budget := cfg.oltIfDerivedTimeout()
	totalBudget := budget
	includeTelnet := oltcollect.IncludesTelnetOnuCollectMode(onuCollectMode) &&
		(profile.OnuReport.MonitorEnabled() || profile.PonTelnet.MonitorEnabled())
	if includeTelnet {
		totalBudget = budget + telnetTO
	}
	if budget < 120*time.Second {
		budget = 120 * time.Second
	}
	if totalBudget < budget {
		totalBudget = budget
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), totalBudget)
	defer cancel()

	modeNorm := NormalizeOltOnuMode(onuCollectMode)
	summary := map[string]any{
		"source":              "monitor_worker_olt_periodic",
		"olt_collection_mode": "profile_periodic",
		"olt_onu_mode":        modeNorm,
		"updated_at":          time.Now().UTC().Format(time.RFC3339),
	}

	execSt := &oltWorkerExecState{
		Pool: pool, DeviceID: deviceID,
		Host: host, Community: community,
		Brand: brand, Model: model, DevDesc: devDesc,
		MaxPons: maxPonsVal, Profile: profile,
		Summary: summary, TelnetTO: telnetTO,
		StepsOverride: periodicSteps,
	}
	if err := runOltProfileStepsWorker(sctx, execSt, oltcollect.ScopeFull); err != nil {
		out.Reason = strings.TrimSpace(err.Error())
		if log != nil {
			log.Warn().Err(err).Str("device", deviceID.String()).Msg("OLT periódica: perfil")
		}
	}

	pons := execSt.Pons
	if len(pons) == 0 {
		if dl, ok := sctx.Deadline(); ok {
			left := time.Until(dl) - 2*time.Second
			if left > 25*time.Second {
				if fbPons, fbSum := tryPeriodicOltCollectFallback(sctx, pool, deviceID, host, community, brand, profile, left, summary); len(fbPons) > 0 {
					pons = fbPons
					for k, v := range fbSum {
						execSt.Summary[k] = v
					}
				}
			}
		}
	}
	if len(pons) == 0 {
		out.Reason = "coleta por perfil não produziu segmentos PON"
		if r := strings.TrimSpace(oltWorkerAnyString(execSt.Summary["olt_profile_error"])); r != "" {
			out.Reason = r
		}
		if note := strings.TrimSpace(oltWorkerAnyString(execSt.Summary["onu_metrics_note"])); note != "" {
			out.Reason = note
		}
		return out
	}
	pons = applyMaxPonsLimitMapRows(pons, maxPons)
	incomplete := oltcollect.IsOltSnapshotIncomplete(execSt.Summary)
	if incomplete && len(prevMaps) > 0 {
		var carryPatch map[string]any
		pons, carryPatch = oltifderive.PreservePonCountsOnIncomplete(prevMaps, pons)
		for k, v := range carryPatch {
			execSt.Summary[k] = v
		}
		execSt.Summary["onu_delta_alerts_skipped"] = "coleta SNMP incompleta ou truncada"
	}
	if oltcollect.IsPartialOnuCollectMode(onuCollectMode) && len(prevMaps) > 0 {
		pons = oltifderive.OverlayPartialPonSnapshot(prevMaps, pons, modeNorm)
		oltifderive.StripPartialSummaryKeys(execSt.Summary, modeNorm)
	}
	oltifderive.ApplyPonOperStatusAll(pons)
	if !incomplete && !oltcollect.IsPartialOnuCollectMode(onuCollectMode) {
		alertthresholds.EvaluateOltOnuQuantityDeltaAlerts(sctx, pool, log, deviceID, devDesc, host, prevMaps, pons, "monitor_worker")
	} else if NormalizeOltOnuMode(onuCollectMode) == "onu_counts" || NormalizeOltOnuMode(onuCollectMode) == "status_only" {
		alertthresholds.EvaluateOltOnuQuantityDeltaAlerts(sctx, pool, log, deviceID, devDesc, host, prevMaps, pons, "monitor_worker")
	}
	if !oltcollect.IsPartialOnuCollectMode(onuCollectMode) {
		alertthresholds.EvaluateOltOnuOpticalFromPons(sctx, pool, log, deviceID, devDesc, host, pons)
	}

	summary = execSt.Summary
	summary["if_mib_derived_at"] = time.Now().UTC().Format(time.RFC3339)
	sb, _ := json.Marshal(summary)
	pb, _ := json.Marshal(pons)
	_, _ = pool.Exec(sctx, `
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
			Int("pon_segments", len(pons)).
			Float64("onu_online_sum", sumOnuOnlineInPonRows(pons)).
			Msg("OLT periódica: snapshot actualizado (perfil fabricante)")
	}
	out.OK = true
	out.PonCount = len(pons)
	return out
}
