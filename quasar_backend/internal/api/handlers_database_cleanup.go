package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const databaseCleanupMinDays = 60

type dbCleanupTableSpec struct {
	Table   string
	DateCol string
	Label   string
}

var dbCleanupTables = []dbCleanupTableSpec{
	{Table: "telemetry_samples", DateCol: "collected_at", Label: "Telemetria SNMP"},
	{Table: "ping_history", DateCol: "checked_at", Label: "Histórico de ping"},
	{Table: "interface_snapshots", DateCol: "collected_at", Label: "Snapshots de interfaces"},
	{Table: "olt_onu_samples", DateCol: "recorded_at", Label: "Histórico agregado ONU (OLT)"},
	{Table: "bng_stats_samples", DateCol: "collected_at", Label: "Totais BNG (monitoramento)"},
	{Table: "bng_session_snapshots", DateCol: "captured_at", Label: "Snapshots sessões PPPoE BNG"},
	{Table: "events", DateCol: "created_at", Label: "Eventos do sistema"},
	{Table: "snmp_walk_jobs", DateCol: "created_at", Label: "Jobs SNMP walk"},
	{Table: "onu_report_runs", DateCol: "started_at", Label: "Execuções relatório ONU"},
	{Table: "automation_execution_log", DateCol: "started_at", Label: "Histórico de automações"},
	{Table: "integration_run_logs", DateCol: "created_at", Label: "Logs de integrações"},
	{Table: "alert_instances", DateCol: "closed_at", Label: "Alertas encerrados (closed_at)"},
}

type dbCleanupScanRequest struct {
	OlderThanDays int `json:"older_than_days"`
}

func parseCleanupDays(body dbCleanupScanRequest) (int, error) {
	d := body.OlderThanDays
	if d <= 0 {
		d = databaseCleanupMinDays
	}
	if d < databaseCleanupMinDays {
		return 0, fmt.Errorf("o mínimo é %d dias", databaseCleanupMinDays)
	}
	if d > 3650 {
		return 0, fmt.Errorf("máximo 3650 dias")
	}
	return d, nil
}

func (s *Server) databaseCleanupScan(w http.ResponseWriter, r *http.Request) {
	var body dbCleanupScanRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	days, err := parseCleanupDays(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	items, total, err := s.scanDatabaseCleanup(r.Context(), cutoff)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"older_than_days": days,
		"cutoff_at":       cutoff.Format(time.RFC3339),
		"items":           items,
		"total_rows":      total,
	})
	actor := s.actorFromRequest(r)
	s.appendAuditLog(r.Context(), "database", "cleanup", "scan_old_data", actor, nil, map[string]any{
		"older_than_days": days,
		"cutoff_at":       cutoff.Format(time.RFC3339),
		"total_rows":      total,
	})
}

func (s *Server) scanDatabaseCleanup(ctx context.Context, cutoff time.Time) ([]map[string]any, int64, error) {
	pool := s.DB()
	if pool == nil {
		return nil, 0, fmt.Errorf("base indisponível")
	}
	var items []map[string]any
	var total int64
	for _, spec := range dbCleanupTables {
		where := spec.DateCol + " < $1"
		if spec.Table == "alert_instances" {
			where = "closed_at IS NOT NULL AND closed_at < $1"
		}
		q := fmt.Sprintf(`SELECT COUNT(*)::bigint FROM %s WHERE %s`, spec.Table, where)
		var n int64
		if err := pool.QueryRow(ctx, q, cutoff).Scan(&n); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", spec.Table, err)
		}
		total += n
		items = append(items, map[string]any{
			"table": spec.Table, "label": spec.Label, "date_column": spec.DateCol, "count": n,
		})
	}
	return items, total, nil
}

type dbCleanupExecuteRequest struct {
	OlderThanDays int  `json:"older_than_days"`
	Confirm       bool `json:"confirm"`
}

func (s *Server) databaseCleanupExecute(w http.ResponseWriter, r *http.Request) {
	var body dbCleanupExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "confirmação obrigatória (confirm=true)", nil)
		return
	}
	days, err := parseCleanupDays(dbCleanupScanRequest{OlderThanDays: body.OlderThanDays})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	items, total, err := s.scanDatabaseCleanup(r.Context(), cutoff)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if total == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "deleted_total": 0, "items": items, "message": "Nenhum registo antigo para apagar.",
		})
		return
	}

	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusInternalServerError, "DB", "base indisponível", nil)
		return
	}
	deleted := make([]map[string]any, 0, len(dbCleanupTables))
	var deletedTotal int64
	for _, spec := range dbCleanupTables {
		where := spec.DateCol + " < $1"
		if spec.Table == "alert_instances" {
			where = "closed_at IS NOT NULL AND closed_at < $1"
		}
		q := fmt.Sprintf(`DELETE FROM %s WHERE %s`, spec.Table, where)
		tag, err := pool.Exec(r.Context(), q, cutoff)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", fmt.Sprintf("falha ao apagar %s: %v", spec.Table, err), nil)
			return
		}
		n := tag.RowsAffected()
		deletedTotal += n
		deleted = append(deleted, map[string]any{"table": spec.Table, "label": spec.Label, "deleted": n})
	}

	actor := s.actorFromRequest(r)
	s.appendAuditLog(r.Context(), "database", "cleanup", "purge_old_data", actor, nil, map[string]any{
		"older_than_days": days,
		"cutoff_at":       cutoff.Format(time.RFC3339),
		"deleted_total":   deletedTotal,
		"tables":          deleted,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"deleted_total": deletedTotal,
		"items":         deleted,
		"cutoff_at":     cutoff.Format(time.RFC3339),
	})
}
