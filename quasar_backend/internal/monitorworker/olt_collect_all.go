package monitorworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
	"github.com/rs/zerolog"
)

// OltCollectSweepResult sumário de um ciclo completo de coleta ONU/PON.
type OltCollectSweepResult struct {
	TotalInDB   int
	Eligible    int
	Attempted   int
	OK          int
	Failed      int
	Skipped      int
	PrecheckSkip int
	Outcomes     []map[string]any
}

// OltCollectReadiness resultado da validação prévia por OLT.
type OltCollectReadiness struct {
	DeviceID    uuid.UUID
	Description string
	Host        string
	Brand       string
	Model       string
	Ready       bool
	Reason      string
}

// resolveOltsToCollect fonte única: lista explícita (pipeline) ou todas as OLTs da BD (mesmo critério da UI).
func resolveOltsToCollect(ctx context.Context, pool *pgxpool.Pool, opts SweepOpts) ([]pingableDeviceRow, error) {
	if len(opts.ScopedDevices) > 0 {
		return opts.ScopedDevices, nil
	}
	return loadOltDevicesForCollect(ctx, pool, opts.DeviceID)
}

// validateOltCollectReady verifica requisitos mínimos antes de tentar SNMP (mesma base do refresh manual).
func validateOltCollectReady(ctx context.Context, pool *pgxpool.Pool, row pingableDeviceRow, defCommunity *string) OltCollectReadiness {
	r := OltCollectReadiness{
		DeviceID:    row.id,
		Description: row.description,
		Host:        strings.TrimSpace(row.ip),
		Brand:       strings.TrimSpace(row.brand),
		Model:       strings.TrimSpace(row.model),
	}
	if r.Host == "" {
		r.Reason = "IP em falta"
		return r
	}
	if resolveSNMPCommunity(row, defCommunity) == "" {
		r.Reason = "community SNMP em falta (equipamento e Definições → Rede)"
		return r
	}
	if r.Brand == "" || r.Model == "" {
		r.Reason = "marca ou modelo OLT em falta"
		return r
	}
	if pool != nil {
		if _, err := oltcollect.LoadVendorProfile(ctx, pool, r.Brand, r.Model); err != nil {
			r.Reason = fmt.Sprintf("perfil OLT não encontrado para %s / %s", r.Brand, r.Model)
			return r
		}
	}
	r.Ready = true
	return r
}

func persistOltCollectStatus(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, source string, out OltCollectOutcome) {
	if pool == nil || deviceID == uuid.Nil {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "worker"
	}
	patch := map[string]any{
		"last_collect_at":     time.Now().UTC().Format(time.RFC3339Nano),
		"last_collect_ok":     out.OK,
		"last_collect_source": source,
		"olt_collection_mode": strings.TrimSpace(out.Mode),
		"last_collect_skipped": out.Skipped,
	}
	if r := strings.TrimSpace(out.Reason); r != "" {
		patch["last_collect_error"] = r
	} else if out.OK {
		patch["last_collect_error"] = nil
	}
	if out.OK && out.PonCount > 0 {
		patch["last_collect_pon_count"] = out.PonCount
	}
	sb, err := json.Marshal(patch)
	if err != nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, '[]'::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET
			summary = COALESCE(olt_snapshots.summary, '{}'::jsonb) || $2::jsonb
	`, deviceID, sb)
}

// RunOltCollectAll percorre TODAS as OLTs resolvidas, uma a uma, com validação prévia e registo por equipamento.
func RunOltCollectAll(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, mode string, opts SweepOpts) (OltCollectSweepResult, error) {
	var result OltCollectSweepResult
	if mode != ModeFull {
		return result, nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return result, err
	}
	src := strings.TrimSpace(opts.Source)
	if src == "" {
		src = "worker"
	}

	devices, err := resolveOltsToCollect(ctx, pool, opts)
	if err != nil {
		return result, err
	}
	result.TotalInDB = len(devices)
	if len(devices) == 0 {
		_, err = pool.Exec(ctx, `UPDATE monitoring_runtime SET last_olt_if_derived_cycle_at = now(), updated_at = now(), last_cycle_at = now() WHERE id=1`)
		return result, err
	}

	var defCommunity *string
	_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)

	type item struct {
		row   pingableDeviceRow
		ready OltCollectReadiness
	}
	queue := make([]item, 0, len(devices))
	for _, row := range devices {
		ready := validateOltCollectReady(ctx, pool, row, defCommunity)
		if !ready.Ready {
			result.PrecheckSkip++
			result.Skipped++
			persistOltCollectStatus(ctx, pool, row.id, src, OltCollectOutcome{
				Skipped: true,
				Reason:  ready.Reason,
				Mode:    "precheck",
			})
			if len(result.Outcomes) < 100 {
				result.Outcomes = append(result.Outcomes, map[string]any{
					"device_id": row.id.String(), "host": ready.Host, "description": row.description,
					"brand": ready.Brand, "ok": false, "skipped": true, "reason": ready.Reason, "phase": "precheck",
				})
			}
			if log != nil {
				log.Warn().
					Str("device_id", row.id.String()).
					Str("host", ready.Host).
					Str("reason", ready.Reason).
					Msg("OLT ignorada na coleta (pré-validação)")
			}
			continue
		}
		queue = append(queue, item{row: row, ready: ready})
	}
	result.Eligible = len(queue)

	for idx, it := range queue {
		row := it.row
		comm := resolveSNMPCommunity(row, defCommunity)
		label := monitoringDeviceLabel(row.description, row.ip)
		setActivity(ctx, pool, fmt.Sprintf("Coleta ONUs OLT [%d/%d] · %s", idx+1, result.Eligible, label))

		unlock := snmpdevicelock.Acquire(row.id)
		func() {
			defer unlock()
			result.Attempted++

			var out OltCollectOutcome
			if OltUsesIfDerivedPonSnapshots(row.category, row.brand, row.model) {
				sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.oltIfDerivedTimeout())
				defer scancel()
				invEmptyBefore, _ := snmpInventoryEmpty(sctx, pool, row.id)
				if invEmptyBefore {
					_, _ = snmpdiscovery.EnsureFreshInventory(sctx, pool, log, row.id, snmpdiscovery.DefaultInventoryMaxAge)
				}
				out = CollectOltPonAndEvaluate(sctx, pool, log, row.id, strings.TrimSpace(row.ip), comm, row.description, row.category, row.brand, row.model, row.maxPons)
			} else if manualOut, ok := tryOltManualRefresh(ctx, row.id, src); ok {
				out = manualOut
			} else {
				sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.oltIfDerivedTimeout())
				defer scancel()
				out = CollectOltVendorPeriodic(sctx, pool, log, row.id, strings.TrimSpace(row.ip), comm, row.description, row.brand, row.model, row.maxPons, "")
			}

			persistOltCollectStatus(ctx, pool, row.id, src, out)
			if out.OK {
				result.OK++
				NudgeMonitoringRuntimeRefresh(ctx, pool)
				if log != nil {
					log.Info().
						Str("device_id", row.id.String()).
						Str("host", strings.TrimSpace(row.ip)).
						Int("pon_count", out.PonCount).
						Int("progress", idx+1).
						Int("total", result.Eligible).
						Msg("OLT colectada com sucesso")
				}
			} else {
				if out.Skipped {
					result.Skipped++
				} else {
					result.Failed++
				}
				if log != nil {
					log.Warn().
						Str("device_id", row.id.String()).
						Str("host", strings.TrimSpace(row.ip)).
						Str("brand", strings.TrimSpace(row.brand)).
						Bool("skipped", out.Skipped).
						Str("reason", out.Reason).
						Msg("coleta ONU/PON OLT falhou")
				}
			}
			if len(result.Outcomes) < 100 {
				result.Outcomes = append(result.Outcomes, map[string]any{
					"device_id": row.id.String(), "host": strings.TrimSpace(row.ip),
					"description": row.description, "brand": strings.TrimSpace(row.brand),
					"ok": out.OK, "skipped": out.Skipped, "reason": out.Reason,
					"pon_count": out.PonCount, "mode": out.Mode, "phase": "collect",
				})
			}
		}()
	}

	if log != nil {
		log.Info().
			Int("total_in_db", result.TotalInDB).
			Int("eligible", result.Eligible).
			Int("attempted", result.Attempted).
			Int("ok", result.OK).
			Int("failed", result.Failed).
			Int("skipped", result.Skipped).
			Int("precheck_skip", result.PrecheckSkip).
			Str("source", src).
			Msg("ciclo ONU/PON OLT concluído (todas percorridas)")
		if result.TotalInDB > 0 && result.OK == 0 && result.Attempted == 0 {
			log.Error().Int("total", result.TotalInDB).Msg("nenhuma OLT elegível — verifique IP, SNMP, marca/modelo e perfis")
		}
	}

	appendWorkerAudit(ctx, pool, log, "monitoring_cycle", CycleSlugOltIfDerived, "run", map[string]any{
		"source":        src,
		"total_in_db":   result.TotalInDB,
		"eligible":      result.Eligible,
		"attempted":     result.Attempted,
		"ok":            result.OK,
		"failed":        result.Failed,
		"skipped":       result.Skipped,
		"precheck_skip": result.PrecheckSkip,
		"all_olts_mode": true,
		"devices":       result.Outcomes,
	})

	_, err = pool.Exec(ctx, `
		UPDATE monitoring_runtime SET last_olt_if_derived_cycle_at = now(), last_cycle_at = now(), updated_at = now()
		WHERE id = 1
	`)
	return result, err
}

// AuditOltCollectReadiness devolve estado de prontidão de todas as OLTs (útil para diagnóstico/API).
func AuditOltCollectReadiness(ctx context.Context, pool *pgxpool.Pool) ([]OltCollectReadiness, error) {
	devices, err := loadOltDevicesForCollect(ctx, pool, nil)
	if err != nil {
		return nil, err
	}
	var defCommunity *string
	if pool != nil {
		_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defCommunity)
	}
	out := make([]OltCollectReadiness, 0, len(devices))
	for _, row := range devices {
		out = append(out, validateOltCollectReady(ctx, pool, row, defCommunity))
	}
	return out, nil
}
