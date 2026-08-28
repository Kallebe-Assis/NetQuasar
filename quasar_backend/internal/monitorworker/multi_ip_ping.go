package monitorworker

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// multi_ip_ping.go — sonda os IPs extra monitorados de um equipamento (device_ips) durante o
// mesmo ciclo de latência (RunLatencySweep), e combina o resultado com o do IP primário
// conforme devices.offline_alert_logic ("any"/"all") para decidir se o alerta ping_unreachable
// deve abrir. device_probe_cache/ping_history do IP primário não mudam em nada — este ficheiro
// só lê/escreve device_ip_probe_state, uma tabela nova e paralela.

type monitoredExtraIP struct {
	id uuid.UUID
	ip string
}

func loadMonitoredExtraIPs(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) ([]monitoredExtraIP, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, host(ip)::text FROM device_ips WHERE device_id=$1 AND monitored=true
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []monitoredExtraIP
	for rows.Next() {
		var m monitoredExtraIP
		if err := rows.Scan(&m.id, &m.ip); err != nil {
			return nil, err
		}
		if strings.TrimSpace(m.ip) != "" {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

// probeExtraIPConfirmedOffline sonda um IP extra, actualiza o seu fail_streak em
// device_ip_probe_state (mesma lógica de pingOfflineConfirmed do IP primário) e devolve se
// está confirmado offline (streak >= limiar), não a falha isolada de um único ciclo.
func probeExtraIPConfirmedOffline(ctx context.Context, pool *pgxpool.Pool, m monitoredExtraIP, icmpPart, tcpPart time.Duration, icmpPayload, threshold int) bool {
	var prevStreak int
	_ = pool.QueryRow(ctx, `SELECT COALESCE(fail_streak, 0) FROM device_ip_probe_state WHERE device_ip_id=$1`, m.id).Scan(&prevStreak)

	pctx, cancel := context.WithTimeout(ctx, icmpPart+tcpPart+300*time.Millisecond)
	probe := probing.HostReachability(pctx, m.ip, "443", icmpPart, tcpPart, icmpPayload)
	cancel()
	ok, _ := probe["ok"].(bool)
	var lat int64
	switch v := probe["latency_ms"].(type) {
	case int64:
		lat = v
	case float64:
		lat = int64(v)
	case int:
		lat = int64(v)
	}

	streakAfter := 0
	if !ok {
		streakAfter = prevStreak + 1
	}
	_, _ = pool.Exec(ctx, `
		INSERT INTO device_ip_probe_state (device_ip_id, ok, fail_streak, latency_ms, checked_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (device_ip_id) DO UPDATE SET
			ok = EXCLUDED.ok, fail_streak = EXCLUDED.fail_streak, latency_ms = EXCLUDED.latency_ms, checked_at = EXCLUDED.checked_at
	`, m.id, ok, streakAfter, lat)

	return pingOfflineConfirmed(ok, streakAfter, threshold)
}

// combinedOfflineConfirmed decide se o equipamento deve ser considerado offline dado o estado
// (já confirmado por streak) do IP primário e dos IPs extra monitorados:
//   - "all": só offline quando TODOS (primário + extras) estiverem confirmados offline —
//     "só alarmar quando os 2 IPs não responderem".
//   - "any" (padrão): offline se qualquer um estiver confirmado offline — "quando um só não
//     responder já alarma". Com 0 IPs extra (o caso de quase todo equipamento hoje), o
//     resultado é sempre igual ao do IP primário sozinho — comportamento inalterado.
func combinedOfflineConfirmed(primaryConfirmedOffline bool, extrasConfirmedOffline []bool, logic string) bool {
	if len(extrasConfirmedOffline) == 0 {
		return primaryConfirmedOffline
	}
	if strings.EqualFold(strings.TrimSpace(logic), "all") {
		if !primaryConfirmedOffline {
			return false
		}
		for _, c := range extrasConfirmedOffline {
			if !c {
				return false
			}
		}
		return true
	}
	// any
	if primaryConfirmedOffline {
		return true
	}
	for _, c := range extrasConfirmedOffline {
		if c {
			return true
		}
	}
	return false
}
