package alertthresholds

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/rs/zerolog"
)

const alertTypePonDown = "pon_down"

// EvaluatePonDownAlerts abre/fecha alerta pon_down quando a PON passa de ON → OFF
// no mesmo critério da ficha da OLT (≥1 ONU online = UP) e resolve no regresso.
func EvaluatePonDownAlerts(
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
	src := strings.TrimSpace(source)
	if src == "" {
		src = "monitor_worker"
	}
	oltLabel := strings.TrimSpace(devDesc)
	if oltLabel == "" {
		oltLabel = host
	}

	prevUp := map[string]bool{}
	prevKnown := map[string]bool{}
	for _, p := range prevPons {
		k := oltifderive.StablePonRowKey(p)
		if k == "" {
			continue
		}
		up, ok := oltifderive.PonLooksUpOnOltPage(p)
		if !ok {
			continue
		}
		prevKnown[k] = true
		prevUp[k] = up
	}

	for _, p := range curPons {
		k := oltifderive.StablePonRowKey(p)
		if k == "" {
			continue
		}
		curUp, curOK := oltifderive.PonLooksUpOnOltPage(p)
		if !curOK {
			continue
		}
		metaKey := "pon_down:" + k
		if !prevKnown[k] {
			// Sem histórico: não abrir no 1.º snapshot; fechar se já estiver UP.
			if curUp {
				closePonDownAlertAliases(ctx, pool, log, deviceID, k)
			}
			continue
		}
		wasUp := prevUp[k]
		if wasUp && !curUp {
			msg := fmt.Sprintf("PON %s — status DOWN (inactiva) — OLT %s (%s).", k, oltLabel, host)
			meta := alertnotify.WithStatusTransition(map[string]any{
				"source":    src,
				"pon":       k,
				"key":       metaKey,
				"metric_id": "pon_oper_status",
			}, "pon_up", "pon_down", nil)
			if name := strings.TrimSpace(fmt.Sprint(p["name"])); name != "" && name != k {
				meta["pon_name"] = name
			}
			res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
				DeviceID: deviceID, Severity: "critical", AlertType: alertTypePonDown,
				Message: msg, IP: host, DeviceName: oltLabel, Meta: meta,
				Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: metaKey},
			}, &alertstore.NotifyCreate{
				Log: log, Level: "CRITICAL", Headline: "PON inactiva (DOWN)",
			})
			if err != nil && log != nil {
				log.Error().Err(err).Str("device", deviceID.String()).Str("pon", k).Msg("pon_down")
			} else if res.Created && log != nil {
				log.Warn().Str("device", deviceID.String()).Str("pon", k).Msg("alerta criado: PON DOWN")
			}
			continue
		}
		if curUp {
			closePonDownAlertAliases(ctx, pool, log, deviceID, k)
		}
	}
}

func closePonDownAlertAliases(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, ponKey string) {
	seen := map[string]struct{}{}
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		closePonDownAlert(ctx, pool, log, deviceID, "pon_down:"+k)
	}
	add(ponKey)
	norm := oltifderive.PonIdentityNorm(ponKey)
	add(norm)
	if n, err := strconv.Atoi(norm); err == nil && n > 0 {
		add(oltifderive.VsolMibPonCompactID(n))
		add(strconv.Itoa(n))
	}
}

func closePonDownAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, metaKey string) {
	_, _, err := alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID:  deviceID,
		AlertType: alertTypePonDown,
		Match:     alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: metaKey},
		Resolved: map[string]any{
			"resolved": "pon_up",
			"source":   "monitor_worker",
		},
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Str("key", metaKey).Msg("fechar pon_down")
	}
}
