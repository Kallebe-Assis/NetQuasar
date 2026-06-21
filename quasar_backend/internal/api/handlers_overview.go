package api

import (
	"net/http"
)

func (s *Server) overviewSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var devices, pops int64
	var clients int64
	var monRunning bool
	_ = s.DB().QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&devices)
	_ = s.DB().QueryRow(ctx, `SELECT COUNT(*) FROM pops`).Scan(&pops)
	_ = s.DB().QueryRow(ctx, `
		SELECT COALESCE(SUM(client_count), 0)::bigint FROM commercial_monthly_records
		WHERE year_month = to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM')
	`).Scan(&clients)
	_ = s.DB().QueryRow(ctx, `SELECT is_running FROM monitoring_runtime WHERE id=1`).Scan(&monRunning)

	writeJSON(w, http.StatusOK, map[string]any{
		"devices":                devices,
		"pops":                   pops,
		"commercial_clients_sum": clients,
		"monitoring_running":     monRunning,
	})
}
