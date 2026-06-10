package monitorworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
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
	if alertignore.IsMuted(ctx, pool, deviceID, alertTypeUptimeRestartLow, "") {
		return
	}
	desc := strings.TrimSpace(description)
	addr := strings.TrimSpace(ip)
	msg := fmt.Sprintf("Equipamento reiniciou (Uptime < %d minutos).", thresholdMin)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":                  "monitor_worker",
		"threshold_minutes":       thresholdMin,
		"observed_uptime_minutes": observedMin,
	}, "uptime_ok", "uptime_low", map[string]any{"uptime_minutes": observedMin})
	metaRaw, jerr := json.Marshal(meta)
	if jerr != nil {
		metaRaw = []byte("{}")
	}
	var alertID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, 'warning', $2::text, $3, NULLIF(trim($4), ''), NULLIF(trim($5), ''),
			COALESCE($6::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid AND ai.alert_type = $2::text AND ai.closed_at IS NULL
		)
		RETURNING id
	`, deviceID, alertTypeUptimeRestartLow, msg, addr, desc, metaRaw).Scan(&alertID)
	if err != nil {
		if err == pgx.ErrNoRows {
			patch, _ := json.Marshal(map[string]any{
				"observed_uptime_minutes": observedMin,
				"uptime_minutes":          observedMin,
			})
			_, _ = pool.Exec(ctx, `
				UPDATE alert_instances SET
					meta = COALESCE(meta, '{}'::jsonb) || $3::jsonb
				WHERE device_id = $1::uuid AND alert_type = $2::text AND closed_at IS NULL
			`, deviceID, alertTypeUptimeRestartLow, patch)
			return
		}
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alert_instances insert uptime_restart_low")
		return
	}
	log.Warn().Str("device", deviceID.String()).Float64("uptime_min", observedMin).Int("threshold", thresholdMin).Msg("alerta: uptime abaixo do limiar (possível reinício)")
	alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, alertID, "WARNING", "Possível reinício do equipamento", msg)
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
