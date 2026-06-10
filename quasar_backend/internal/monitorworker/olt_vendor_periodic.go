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
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
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
) {
	if pool == nil || strings.TrimSpace(host) == "" || strings.TrimSpace(community) == "" {
		return
	}
	brand = strings.TrimSpace(brand)
	model = strings.TrimSpace(model)
	if brand == "" || model == "" {
		return
	}
	profile, err := oltcollect.LoadVendorProfile(ctx, pool, brand, model)
	if err != nil {
		if log != nil {
			log.Debug().Err(err).Str("device", deviceID.String()).Str("brand", brand).Str("model", model).
				Msg("OLT periódica: perfil não encontrado")
		}
		return
	}
	steps := oltcollect.StepsForScope(oltcollect.EnabledSteps(profile.Steps), oltcollect.ScopeOnu)
	if len(steps) == 0 {
		return
	}

	var prevPonsRaw []byte
	_ = pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1`, deviceID).Scan(&prevPonsRaw)
	prevMaps := oltifderive.PonsJSONToMaps(prevPonsRaw)

	maxPonsVal := 0
	if maxPons != nil && *maxPons > 0 {
		maxPonsVal = *maxPons
	}

	cfg, _ := loadClampMonitoringIntervals(ctx, pool)
	budget := cfg.oltIfDerivedTimeout()
	if budget <= 0 {
		budget = 180 * time.Second
	}
	sctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	summary := map[string]any{
		"source":              "monitor_worker_olt_periodic",
		"olt_collection_mode": "profile_periodic",
		"updated_at":          time.Now().UTC().Format(time.RFC3339),
	}
	var pons []map[string]any

	for _, step := range steps {
		switch step.Method {
		case oltcollect.MethodOnuMetricsCollect:
			if !profile.OnuMetrics.HasAnyEnabled() {
				continue
			}
			sum, p, _, err := oltcollect.CollectOnuMetrics(sctx, host, community, profile.OnuMetrics, budget, maxPonsVal)
			if err != nil {
				if log != nil {
					log.Warn().Err(err).Str("device", deviceID.String()).Msg("OLT periódica: onu_metrics_collect")
				}
				return
			}
			for k, v := range sum {
				summary[k] = v
			}
			pons = p
		case oltcollect.MethodOnuSNMPWalk:
			oid := strings.TrimSpace(profile.ResolveWalkOID(step))
			if oid == "" && strings.EqualFold(brand, "vsol") {
				oid = vsolparse.DefaultVSOLOnuWalkOID
			}
			if oid == "" {
				continue
			}
			sum, ponsWalk, _, _, _, err := vsolparse.WalkOnuTable(sctx, host, community, oid, budget)
			if err != nil {
				if log != nil {
					log.Warn().Err(err).Str("device", deviceID.String()).Msg("OLT periódica: onu_snmp_walk")
				}
				return
			}
			for k, v := range sum {
				summary[k] = v
			}
			pons = ponsWalk
		default:
			continue
		}
		if len(pons) > 0 {
			break
		}
	}

	if len(pons) == 0 {
		return
	}
	pons = applyMaxPonsLimitMapRows(pons, maxPons)
	oltifderive.ApplyPonOperStatusAll(pons)
	alertthresholds.EvaluateOltOnuQuantityDeltaAlerts(sctx, pool, log, deviceID, devDesc, host, prevMaps, pons, "monitor_worker")
	alertthresholds.EvaluateOltOnuOpticalFromPons(sctx, pool, log, deviceID, devDesc, host, pons)

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
}
