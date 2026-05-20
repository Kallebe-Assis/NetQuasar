package monitorworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertcorrelation"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/rs/zerolog"
)

const alertTypePingUnreachable = "ping_unreachable"

// repairMissingPingUnreachableAlerts cria alertas em aberto para equipamentos já offline no cache
// (streak >= limiar) sem instância aberta — cobre falhas anteriores ao fix do limiar consecutivo.
func repairMissingPingUnreachableAlerts(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, threshold int) {
	if pool == nil || threshold < 1 {
		return
	}
	rows, err := pool.Query(ctx, `
		SELECT d.id, COALESCE(TRIM(d.description), ''), host(d.ip)::text, COALESCE(c.detail::text, '{}')
		FROM device_probe_cache c
		JOIN devices d ON d.id = c.device_id
		WHERE c.reach_ok = false
		  AND COALESCE(c.ping_fail_streak, 0) >= $1
		  AND ` + SQLDeviceEligibleForPingAlerts + `
		  AND NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = c.device_id
			  AND ai.alert_type = $2
			  AND ai.closed_at IS NULL
		  )
	`, threshold, alertTypePingUnreachable)
	if err != nil {
		if log != nil {
			log.Error().Err(err).Msg("repair ping_unreachable: query")
		}
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var desc, ip, detailText string
		if err := rows.Scan(&id, &desc, &ip, &detailText); err != nil {
			continue
		}
		probe := map[string]any{"ok": false}
		if strings.TrimSpace(detailText) != "" {
			var root map[string]any
			if json.Unmarshal([]byte(detailText), &root) == nil {
				if rv, ok := root["reachability"].(map[string]any); ok {
					probe = rv
				}
			}
		}
		insertPingUnreachableIfNew(ctx, pool, log, id, desc, ip, probe, "repair_streak")
	}
}

// InsertPingUnreachableIfNew grava alerta ping_unreachable em aberto (se ainda não existir) e notifica Telegram (monitorização) quando novo.
// Verifica elegibilidade do equipamento (ex.: ping manual em inativo).
func InsertPingUnreachableIfNew(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, description, ip string, probe map[string]any, metaSource string) {
	eligible, err := DeviceEligibleForPingAlerts(ctx, pool, deviceID)
	if err != nil {
		if log != nil {
			log.Error().Err(err).Str("device", deviceID.String()).Msg("ping_unreachable: elegibilidade")
		}
		return
	}
	if !eligible {
		return
	}
	insertPingUnreachableIfNew(ctx, pool, log, deviceID, description, ip, probe, metaSource)
}

// InsertPingUnreachableIfNewForMonitoredDevice skips the eligibility query — use only when the device
// was already loaded via loadPingableDevices or equivalent filter.
func InsertPingUnreachableIfNewForMonitoredDevice(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, description, ip string, probe map[string]any, metaSource string) {
	insertPingUnreachableIfNew(ctx, pool, log, deviceID, description, ip, probe, metaSource)
}

func insertPingUnreachableIfNew(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, description, ip string, probe map[string]any, metaSource string) {
	metaSource = strings.TrimSpace(metaSource)
	if metaSource == "" {
		metaSource = "monitor_worker"
	}
	desc := strings.TrimSpace(description)
	addr := strings.TrimSpace(ip)
	msg := fmt.Sprintf("%s (%s): sem resposta ICMP/TCP dentro do tempo de espera configurado.", descOr(desc, "?"), addrOr(addr, "?"))

	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":       metaSource,
		"reachability": probe,
	}, "reachable", "unreachable", nil)
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		metaRaw = []byte("{}")
	}

	var alertID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO alert_instances (device_id, severity, alert_type, message, ip, device_name, meta)
		SELECT $1::uuid, 'critical', $2::text, $3, NULLIF(trim($4), ''), NULLIF(trim($5), ''),
			COALESCE($6::jsonb, '{}'::jsonb)
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_instances ai
			WHERE ai.device_id = $1::uuid AND ai.alert_type = $2::text AND ai.closed_at IS NULL
		)
		RETURNING id
	`, deviceID, alertTypePingUnreachable, msg, addr, desc, metaRaw).Scan(&alertID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return
		}
		log.Error().Err(err).Str("device", deviceID.String()).Msg("alert_instances insert ping_unreachable")
		return
	}
	log.Warn().Str("device", deviceID.String()).Str("host", addr).Msg("alerta criado: equipamento inalcançável (ICMP/TCP) — mudança de estado")
	alertcorrelation.Reconcile(ctx, pool, log)
	alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, pool, log, alertID, "CRITICAL", "Equipamento offline", msg)
}

func resolvePingUnreachableForDevices(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, recovered []uuid.UUID) {
	if len(recovered) == 0 {
		return
	}
	rows, err := pool.Query(ctx, `
		UPDATE alert_instances SET
			closed_at = now(),
			meta = COALESCE(meta, '{}'::jsonb) || '{"resolved":"icmp_or_tcp_ok","source":"monitor_worker"}'::jsonb
		WHERE alert_type = $1
		  AND closed_at IS NULL
		  AND device_id = ANY($2::uuid[])
		RETURNING id, alert_type, message, device_name, ip
	`, alertTypePingUnreachable, recovered)
	if err != nil {
		log.Error().Err(err).Msg("fechar alertas ping_unreachable")
		return
	}
	defer rows.Close()
	var n int64
	for rows.Next() {
		var id uuid.UUID
		var atype, msg, dname, ip string
		if err := rows.Scan(&id, &atype, &msg, &dname, &ip); err != nil {
			log.Error().Err(err).Msg("scan ping_unreachable resolvido")
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
		log.Error().Err(err).Msg("iter ping_unreachable resolvido")
		return
	}
	if n > 0 {
		log.Info().Int64("closed", n).Msg("alertas ping recuperados — equipamento volta a responder")
	}
}

func descOr(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return v
}

func addrOr(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return v
}
