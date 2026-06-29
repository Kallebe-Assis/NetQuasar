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
	"github.com/rs/zerolog"
)

const alertTypeBngSubscriberDrop = "bng_subscriber_drop"

type bngSampleVals struct {
	total, pppoe, ipv4, ipv6, dual *int
}

type bngDropMetricSpec struct {
	metricID string
	field    string
	metaKey  string
	title    string
}

var bngDropMetricSpecs = []bngDropMetricSpec{
	{metricID: "bng_pppoe_drop_count", field: "pppoe_online", metaKey: "bng_drop:pppoe", title: "PPPoE online"},
	{metricID: "bng_ipv4_drop_count", field: "ipv4_online", metaKey: "bng_drop:ipv4", title: "IPv4 online"},
	{metricID: "bng_ipv6_drop_count", field: "ipv6_online", metaKey: "bng_drop:ipv6", title: "IPv6 online"},
	{metricID: "bng_total_drop_count", field: "total_online", metaKey: "bng_drop:total", title: "Total online"},
	{metricID: "bng_dual_stack_drop_count", field: "dual_stack_online", metaKey: "bng_drop:dual_stack", title: "Dual-stack"},
}

func (s bngSampleVals) fieldVal(field string) *int {
	switch field {
	case "total_online":
		return s.total
	case "pppoe_online":
		return s.pppoe
	case "ipv4_online":
		return s.ipv4
	case "ipv6_online":
		return s.ipv6
	case "dual_stack_online":
		return s.dual
	default:
		return nil
	}
}

// BngSubscriberDropAlertsEnabled indica se existe algum limiar activo de queda BNG entre coletas.
func BngSubscriberDropAlertsEnabled(ctx context.Context, pool *pgxpool.Pool) bool {
	if pool == nil {
		return false
	}
	for _, spec := range bngDropMetricSpecs {
		_, _, ok := LoadGlobalGteMetricForDevice(ctx, pool, spec.metricID, "bng")
		if ok {
			return true
		}
	}
	return false
}

// EvaluateBngSubscriberDropAlerts compara a última coleta SNMP com a anterior e abre/fecha alertas
// quando a queda absoluta ultrapassa os limiares configurados (warning/critical).
func EvaluateBngSubscriberDropAlerts(
	ctx context.Context,
	pool *pgxpool.Pool,
	log *zerolog.Logger,
	deviceID uuid.UUID,
	devDesc, host, source string,
) {
	if pool == nil {
		return
	}
	var bngEnabled bool
	if err := pool.QueryRow(ctx, `SELECT coalesce(bng_enabled, false) FROM devices WHERE id=$1`, deviceID).Scan(&bngEnabled); err != nil || !bngEnabled {
		return
	}

	prev, cur, ok := loadLastTwoBngSamples(ctx, pool, deviceID)
	if !ok {
		return
	}

	bngLabel := strings.TrimSpace(devDesc)
	if bngLabel == "" {
		bngLabel = strings.TrimSpace(host)
	}
	if bngLabel == "" {
		bngLabel = "BNG"
	}
	host = strings.TrimSpace(host)

	for _, spec := range bngDropMetricSpecs {
		th, metricLabel, enabled := LoadGlobalGteMetricForDevice(ctx, pool, spec.metricID, "bng")
		if !enabled {
			closeBngSubscriberDropAlert(ctx, pool, log, deviceID, spec.metaKey)
			continue
		}
		if strings.TrimSpace(metricLabel) == "" {
			metricLabel = spec.title
		}

		prevV := prev.fieldVal(spec.field)
		curV := cur.fieldVal(spec.field)
		if prevV == nil || curV == nil || *curV >= *prevV {
			closeBngSubscriberDropAlert(ctx, pool, log, deviceID, spec.metaKey)
			continue
		}

		delta := float64(*prevV - *curV)
		sev := severityGteMetric(delta, th)
		if sev == "ok" {
			closeBngSubscriberDropAlert(ctx, pool, log, deviceID, spec.metaKey)
			continue
		}
		if alertignore.IsMuted(ctx, pool, deviceID, alertTypeBngSubscriberDrop, spec.metaKey) {
			continue
		}

		prevSt := fmt.Sprintf("%s_%d", spec.field, *prevV)
		currSt := fmt.Sprintf("%s_%d", spec.field, *curV)
		msg := fmt.Sprintf(
			"BNG %s (%s) — queda de %.0f %s (%.0f → %.0f) entre coletas SNMP.",
			bngLabel, addrOrEmpty(host, "?"), delta, metricLabel, float64(*prevV), float64(*curV),
		)
		meta := alertnotify.WithStatusTransition(map[string]any{
			"source":           source,
			"key":              spec.metaKey,
			"metric_id":        spec.metricID,
			"subscriber_field": spec.field,
			"drop_count":       delta,
			"prev_online":      *prevV,
			"curr_online":      *curV,
		}, prevSt, currSt, nil)

		created, aid, err := openOrUpdateBngSubscriberDropAlert(ctx, pool, deviceID, sev, msg, host, bngLabel, meta)
		if err == nil && created && aid != uuid.Nil {
			headline := fmt.Sprintf("Queda de logins BNG — %s", spec.title)
			alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, aid, strings.ToUpper(sev), headline, msg)
		}
	}
}

func loadLastTwoBngSamples(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (prev, cur bngSampleVals, ok bool) {
	rows, err := pool.Query(ctx, `
		SELECT total_online, pppoe_online, ipv4_online, ipv6_online, dual_stack_online
		FROM bng_stats_samples
		WHERE device_id = $1
		ORDER BY collected_at DESC
		LIMIT 2
	`, deviceID)
	if err != nil {
		return bngSampleVals{}, bngSampleVals{}, false
	}
	defer rows.Close()

	var samples []bngSampleVals
	for rows.Next() {
		var s bngSampleVals
		if rows.Scan(&s.total, &s.pppoe, &s.ipv4, &s.ipv6, &s.dual) == nil {
			samples = append(samples, s)
		}
	}
	if len(samples) < 2 {
		return bngSampleVals{}, bngSampleVals{}, false
	}
	return samples[1], samples[0], true
}

func openOrUpdateBngSubscriberDropAlert(
	ctx context.Context,
	pool *pgxpool.Pool,
	deviceID uuid.UUID,
	severity, message, ip, devName string,
	meta map[string]any,
) (created bool, alertID uuid.UUID, err error) {
	if meta == nil {
		meta = map[string]any{}
	}
	key, _ := meta["key"].(string)
	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: severity, AlertType: alertTypeBngSubscriberDrop,
		Message: message, IP: ip, DeviceName: devName, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
	}, nil)
	return res.Created, res.ID, err
}

func closeBngSubscriberDropAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, key string) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertTypeBngSubscriberDrop,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{
			"resolved": "normalized", "source": "bng_subscriber_delta", "key": key,
		},
	})
}
