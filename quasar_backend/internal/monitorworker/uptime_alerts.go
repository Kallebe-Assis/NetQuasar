package monitorworker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
	"github.com/rs/zerolog"
)

const alertTypeUptimeRestartLow = "uptime_restart_low"

// SnmpUptimeMinutes devolve minutos de uptime (sysUpTime em ticks ou texto VSOL).
func SnmpUptimeMinutes(sn probing.SNMPGetResult) (minutes float64, ok bool) {
	for _, v := range sn.Vars {
		oid := strings.Trim(strings.TrimSpace(v.OID), ".")
		val := strings.TrimSpace(v.Value)
		if val == "" || strings.Contains(strings.ToLower(val), "nosuch") {
			continue
		}
		if vsolparse.IsVsolUptimeOID(oid) {
			return vsolparse.UptimeMinutesFromValue(val)
		}
		if strings.HasSuffix(oid, "1.3.6.1.2.1.1.3.0") || oid == "1.3.6.1.2.1.1.3.0" {
			if m, ok := vsolparse.UptimeMinutesFromValue(val); ok {
				return m, true
			}
			ticks, err := parseUintFlexible(val)
			if err != nil {
				return 0, false
			}
			sec := float64(ticks) / 100.0
			return sec / 60.0, true
		}
	}
	return 0, false
}

func parseUintFlexible(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, part := range strings.Fields(s) {
		u, err := strconv.ParseUint(part, 10, 64)
		if err == nil {
			return u, nil
		}
	}
	return 0, fmt.Errorf("no integer")
}

// InsertUptimeRestartAlertIfNew cria alerta em aberto quando o uptime SNMP está abaixo do limiar configurado.
func InsertUptimeRestartAlertIfNew(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, description, ip string, thresholdMin int, observedMin float64) {
	if thresholdMin <= 0 || pool == nil {
		return
	}
	desc := strings.TrimSpace(description)
	addr := strings.TrimSpace(ip)
	msg := fmt.Sprintf("Equipamento com uptime baixo (%.0f min, limite %d min) — possível reinício.", observedMin, thresholdMin)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":                  "monitor_worker",
		"threshold_minutes":       thresholdMin,
		"observed_uptime_minutes": observedMin,
		"uptime_minutes":          observedMin,
		"value":                   observedMin,
		"value_text":              fmt.Sprintf("%.0f min", observedMin),
	}, "uptime_ok", "uptime_low", map[string]any{"uptime_minutes": observedMin})

	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: "warning", AlertType: alertTypeUptimeRestartLow,
		Message: msg, IP: addr, DeviceName: desc, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchDeviceOnly},
	}, &alertstore.NotifyCreate{Log: log, Level: "WARNING", Headline: "Possível reinício do equipamento"})
	if err != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alertstore uptime_restart_low")
		return
	}
	if res.Created {
		log.Warn().Str("device", deviceID.String()).Float64("uptime_min", observedMin).Int("threshold", thresholdMin).Msg("alerta: uptime abaixo do limiar")
	}
}

// ResolveUptimeRestartAlertsForDevices fecha alertas de uptime baixo quando o uptime já não está abaixo do limiar.
func ResolveUptimeRestartAlertsForDevices(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, recovered []uuid.UUID) {
	if len(recovered) == 0 {
		return
	}
	rows, err := pool.Query(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"uptime_above_threshold","source":"monitor_worker"}'::jsonb
		WHERE alert_type = $1
		  AND closed_at IS NULL
		  AND device_id = ANY($2::uuid[])
		RETURNING id, alert_type, message, device_name, ip
	`, alertTypeUptimeRestartLow, recovered)
	if err != nil {
		log.Error().Err(err).Msg("fechar alertas uptime_restart_low")
		return
	}
	defer rows.Close()
	var n int64
	for rows.Next() {
		var id uuid.UUID
		var atype, msg, dname, ip string
		if err := rows.Scan(&id, &atype, &msg, &dname, &ip); err != nil {
			log.Error().Err(err).Msg("scan uptime_restart_low resolvido")
			continue
		}
		n++
		detail := msg
		if strings.TrimSpace(dname) != "" || strings.TrimSpace(ip) != "" {
			detail = fmt.Sprintf("%s — %s (%s)", msg, strings.TrimSpace(dname), strings.TrimSpace(ip))
		}
		head := alertnotify.ResolutionHeadlineForAlertType(atype)
		alertnotify.SendResolutionTelegramAndPatchMeta(ctx, pool, log, id, head, detail)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("iter uptime_restart_low resolvido")
		return
	}
	if n > 0 {
		log.Info().Int64("closed", n).Msg("alertas uptime baixo resolvidos")
	}
}

// evaluateUptimeRestartAlert aplica o limiar de monitoring_intervals.uptime_restart_alert_minutes após cada coleta SNMP.
func evaluateUptimeRestartAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, description, ip string, observedMin float64) {
	if pool == nil || observedMin < 0 {
		return
	}
	var threshold int
	if err := pool.QueryRow(ctx, `SELECT COALESCE(uptime_restart_alert_minutes, 0) FROM monitoring_intervals WHERE id=1`).Scan(&threshold); err != nil || threshold <= 0 {
		return
	}
	if observedMin < float64(threshold) {
		InsertUptimeRestartAlertIfNew(ctx, pool, log, deviceID, description, ip, threshold, observedMin)
		return
	}
	ResolveUptimeRestartAlertsForDevices(ctx, pool, log, []uuid.UUID{deviceID})
}
