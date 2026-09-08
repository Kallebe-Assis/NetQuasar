package api

import (
	"net/http"
	"strconv"
	"time"
)

// dashboardPppoeSessions — aba "Sessões PPPoE" do Dashboard: totais/médias do inventário
// online/offline (bng_known_logins, alimentado pelo ciclo rápido de presença — ver
// bngcollect.CollectAndSyncOnlineLoginsFast) + eventos de conexão/desconexão no período. Fonte
// própria (não faz parte de /dashboard/analytics), mesmo padrão de "servidor"/"frota" no
// DashboardPage.tsx — cada view com os dados que fazem sentido para ela.
func (s *Server) dashboardPppoeSessions(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base de dados indisponível", nil)
		return
	}
	ctx := r.Context()
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 7
	}
	since := time.Now().UTC().AddDate(0, 0, -days)

	var totalKnown, totalOnline, totalOffline int64
	var avgOnlineSessionSec *float64
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE is_online), COUNT(*) FILTER (WHERE NOT is_online)
		FROM bng_known_logins
	`).Scan(&totalKnown, &totalOnline, &totalOffline)
	// Duração da sessão actual (não o online_time_sec do vendor, que só é actualizado pelo ciclo
	// completo lento e pode ficar desactualizado) — connected_at do evento aberto até agora.
	_ = pool.QueryRow(ctx, `
		SELECT AVG(EXTRACT(EPOCH FROM (now() - e.connected_at)))
		FROM bng_known_logins k
		JOIN bng_login_events e ON e.id = k.current_event_id
		WHERE k.is_online = true
	`).Scan(&avgOnlineSessionSec)

	type deviceRow struct {
		DeviceID   string `json:"device_id"`
		DeviceName string `json:"device_name"`
		Total      int64  `json:"total"`
		Online     int64  `json:"online"`
		Offline    int64  `json:"offline"`
	}
	var byDevice []deviceRow
	rows, err := pool.Query(ctx, `
		SELECT k.device_id::text, COALESCE(NULLIF(trim(d.description), ''), host(d.ip)::text, '(sem nome)'),
			COUNT(*), COUNT(*) FILTER (WHERE k.is_online)
		FROM bng_known_logins k
		LEFT JOIN devices d ON d.id = k.device_id
		GROUP BY k.device_id, d.description, d.ip
		ORDER BY 2
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dr deviceRow
			if rows.Scan(&dr.DeviceID, &dr.DeviceName, &dr.Total, &dr.Online) == nil {
				dr.Offline = dr.Total - dr.Online
				byDevice = append(byDevice, dr)
			}
		}
	}

	var connects, disconnects int64
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE connected_at >= $1), COUNT(*) FILTER (WHERE disconnected_at >= $1)
		FROM bng_login_events
		WHERE connected_at >= $1 OR disconnected_at >= $1
	`, since).Scan(&connects, &disconnects)

	out := map[string]any{
		"days": days,
		"totals": map[string]any{
			"known_logins": totalKnown,
			"online":       totalOnline,
			"offline":      totalOffline,
		},
		"by_device": byDevice,
		"events_period": map[string]any{
			"days":        days,
			"connects":    connects,
			"disconnects": disconnects,
		},
	}
	if avgOnlineSessionSec != nil {
		out["avg_online_session_sec"] = *avgOnlineSessionSec
	}
	writeJSON(w, http.StatusOK, out)
}
