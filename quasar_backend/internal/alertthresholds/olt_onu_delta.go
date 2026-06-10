package alertthresholds

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/rs/zerolog"
)

const (
	alertTypeOltOnuDrop = "olt_onu_drop"
	alertTypeOltOnuRise = "olt_onu_rise"
)

// EvaluateOltOnuQuantityDeltaAlerts compara contagens onu_online por PON com o snapshot anterior
// e abre/atualiza alertas quando a variação (queda ou subida) ultrapassa limiares numérico e/ou percentual.
func EvaluateOltOnuQuantityDeltaAlerts(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	devDesc, host string,
	prevPons, curPons []map[string]any,
	source string,
) {
	if pool == nil || len(curPons) == 0 {
		return
	}
	thCnt, _, okCnt := LoadGlobalGteMetricForDevice(ctx, pool, "olt_onu_drop_count", "olt")
	thPct, _, okPct := LoadGlobalGteMetricForDevice(ctx, pool, "olt_onu_drop_percent", "olt")
	if !okCnt && !okPct {
		return
	}

	prevOn := map[string]float64{}
	for _, p := range prevPons {
		k := oltifderive.StablePonRowKey(p)
		if k == "" {
			continue
		}
		if n, ok := oltifderive.OnuOnlineFromRow(p); ok {
			prevOn[k] = n
		}
	}

	oltLabel := strings.TrimSpace(devDesc)
	if oltLabel == "" {
		oltLabel = host
	}

	for _, p := range curPons {
		k := oltifderive.StablePonRowKey(p)
		if k == "" {
			continue
		}
		curOn, curOK := oltifderive.OnuOnlineFromRow(p)
		prev, prevOK := prevOn[k]
		if !curOK || !prevOK || prev == curOn {
			if okCnt {
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_count:"+k)
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_count:"+k)
			}
			if okPct {
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_pct:"+k)
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_pct:"+k)
			}
			continue
		}

		prevSt := fmt.Sprintf("onu_online_%.0f", prev)
		currSt := fmt.Sprintf("onu_online_%.0f", curOn)

		if curOn < prev {
			delta := prev - curOn
			deltaPct := 0.0
			if prev > 0 {
				deltaPct = (delta / prev) * 100.0
			}
			if okCnt {
				evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
					alertTypeOltOnuDrop, "onu_drop_count:"+k, "Queda de ONUs online — PON",
					fmt.Sprintf("Queda de %.0f ONUs online na PON %s da OLT %s (%s).", delta, k, oltLabel, host),
					delta, deltaPct, prev, curOn, prevSt, currSt, thCnt, "drop_online_count", "drop_online_pct")
			} else {
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_count:"+k)
			}
			if okPct {
				evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
					alertTypeOltOnuDrop, "onu_drop_pct:"+k, "Queda percentual de ONUs online — PON",
					fmt.Sprintf("Queda de %.0f%% (%.0f ONUs) das ONUs online na PON %s da OLT %s (%s).", deltaPct, delta, k, oltLabel, host),
					delta, deltaPct, prev, curOn, prevSt, currSt, thPct, "drop_online_count", "drop_online_pct")
			} else {
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_pct:"+k)
			}
			closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_count:"+k)
			closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_pct:"+k)
			continue
		}

		delta := curOn - prev
		deltaPct := 0.0
		if prev > 0 {
			deltaPct = (delta / prev) * 100.0
		}
		if okCnt {
			evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
				alertTypeOltOnuRise, "onu_rise_count:"+k, "Subida de ONUs online — PON",
				fmt.Sprintf("Subida de %.0f ONUs online na PON %s da OLT %s (%s).", delta, k, oltLabel, host),
				delta, deltaPct, prev, curOn, prevSt, currSt, thCnt, "rise_online_count", "rise_online_pct")
		} else {
			closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_count:"+k)
		}
		if okPct && prev > 0 {
			evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
				alertTypeOltOnuRise, "onu_rise_pct:"+k, "Subida percentual de ONUs online — PON",
				fmt.Sprintf("Subida de %.0f%% (%.0f ONUs) das ONUs online na PON %s da OLT %s (%s).", deltaPct, delta, k, oltLabel, host),
				delta, deltaPct, prev, curOn, prevSt, currSt, thPct, "rise_online_count", "rise_online_pct")
		} else {
			closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_pct:"+k)
		}
		closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_count:"+k)
		closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_pct:"+k)
	}
}

func evalOltOnuDeltaSide(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	oltLabel, host, ponKey, source string,
	alertType, key, telegramTitle, message string,
	delta, deltaPct, prev, cur float64,
	prevSt, currSt string,
	th GteMetricThreshold,
	countMetaKey, pctMetaKey string,
) {
	metricVal := delta
	if strings.Contains(key, "_pct:") {
		metricVal = deltaPct
	}
	sev := severityGteMetric(metricVal, th)
	if sev == "ok" {
		closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertType, key)
		return
	}
	if alertignore.IsMuted(ctx, pool, deviceID, alertType, key) {
		return
	}
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":      source,
		"key":         key,
		"pon":         ponKey,
		countMetaKey:  delta,
		pctMetaKey:    deltaPct,
		"prev_online": prev,
		"curr_online": cur,
	}, prevSt, currSt, nil)
	created, aid, err := openOrUpdateOltOnuDeltaAlert(ctx, pool, deviceID, sev, alertType, message, host, oltLabel, meta)
	if err == nil && created && aid != uuid.Nil {
		alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, aid, strings.ToUpper(sev), telegramTitle, message)
	}
}

func openOrUpdateOltOnuDeltaAlert(
	ctx context.Context,
	pool *pgxpool.Pool,
	deviceID uuid.UUID,
	severity, alertType, message, ip, devName string,
	meta map[string]any,
) (created bool, alertID uuid.UUID, err error) {
	if meta == nil {
		meta = map[string]any{}
	}
	metaRaw, _ := json.Marshal(meta)
	err = pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, $2::text, $3::text, $4, NULLIF(trim($5), ''), NULLIF(trim($6), ''), COALESCE($7::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id=$1::uuid AND ai.alert_type=$3::text AND ai.closed_at IS NULL
			  AND (ai.meta->>'key')=($7::jsonb->>'key')
		)
		RETURNING id
	`, deviceID, severity, alertType, message, ip, devName, metaRaw).Scan(&alertID)
	if err == nil {
		return true, alertID, nil
	}
	if err != pgx.ErrNoRows {
		return false, uuid.Nil, err
	}
	_, err = pool.Exec(ctx, `
		UPDATE alert_instances SET
			severity=$3::text,
			message=$4,
			meta=COALESCE(meta, '{}'::jsonb) || $5::jsonb
		WHERE device_id=$1::uuid AND alert_type=$2::text AND closed_at IS NULL
		  AND (meta->>'key')=($5::jsonb->>'key')
	`, deviceID, alertType, severity, message, metaRaw)
	return false, uuid.Nil, err
}

func closeOltOnuDeltaAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType, key string) {
	metaPatch, _ := json.Marshal(map[string]any{"resolved": "normalized", "source": "olt_onu_delta", "key": key})
	var aid uuid.UUID
	var msg string
	err := pool.QueryRow(ctx, `
		UPDATE alert_instances SET
			closed_at=now(),
			meta=COALESCE(meta,'{}'::jsonb) || $4::jsonb
		WHERE device_id=$1::uuid AND alert_type=$2::text AND closed_at IS NULL AND (meta->>'key')=$3
		RETURNING id, message
	`, deviceID, alertType, key, metaPatch).Scan(&aid, &msg)
	if err != nil {
		return
	}
	head := alertnotify.ResolutionHeadlineForAlertType(alertType)
	alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, aid, head, msg)
}
