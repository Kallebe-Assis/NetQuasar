package alertthresholds

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
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
			dropMsg := fmt.Sprintf("PON %s — queda de %.0f ONUs online (%.0f%% de %.0f) — OLT %s (%s).", k, delta, deltaPct, prev, oltLabel, host)
			if okCnt {
				evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
					alertTypeOltOnuDrop, "onu_drop_count:"+k, "Queda de ONUs online — PON",
					dropMsg,
					delta, deltaPct, prev, curOn, prevSt, currSt, thCnt, "drop_online_count", "drop_online_pct")
			} else {
				closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuDrop, "onu_drop_count:"+k)
			}
			if okPct {
				evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
					alertTypeOltOnuDrop, "onu_drop_pct:"+k, "Queda percentual de ONUs online — PON",
					dropMsg,
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
		riseMsg := fmt.Sprintf("PON %s — subida de %.0f ONUs online (%.0f%% de %.0f) — OLT %s (%s).", k, delta, deltaPct, prev, oltLabel, host)
		if okCnt {
			evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
				alertTypeOltOnuRise, "onu_rise_count:"+k, "Subida de ONUs online — PON",
				riseMsg,
				delta, deltaPct, prev, curOn, prevSt, currSt, thCnt, "rise_online_count", "rise_online_pct")
		} else {
			closeOltOnuDeltaAlert(ctx, pool, log, deviceID, alertTypeOltOnuRise, "onu_rise_count:"+k)
		}
		if okPct && prev > 0 {
			evalOltOnuDeltaSide(ctx, pool, log, deviceID, oltLabel, host, k, source,
				alertTypeOltOnuRise, "onu_rise_pct:"+k, "Subida percentual de ONUs online — PON",
				riseMsg,
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
	key, _ := meta["key"].(string)
	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: severity, AlertType: alertType,
		Message: message, IP: ip, DeviceName: devName, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
	}, nil)
	return res.Created, res.ID, err
}

func closeOltOnuDeltaAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, alertType, key string) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertType,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{
			"resolved": "normalized", "source": "olt_onu_delta", "key": key,
		},
	})
}
