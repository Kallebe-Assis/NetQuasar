package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	databaseCleanupScanMinDays    = 1
	databaseCleanupExecuteMinDays = 7
)

type dbCleanupTableSpec struct {
	Table    string
	DateCol  string
	Label    string
	Category string
	// Guard "closed_only" — só linhas com closed_at preenchido (alertas).
	Guard string
}

var dbCleanupTables = []dbCleanupTableSpec{
	{Table: "telemetry_samples", DateCol: "collected_at", Label: "Telemetria SNMP", Category: "monitoramento"},
	{Table: "ping_history", DateCol: "checked_at", Label: "Histórico de ping", Category: "monitoramento"},
	{Table: "interface_snapshots", DateCol: "collected_at", Label: "Snapshots de interfaces", Category: "monitoramento"},
	{Table: "olt_onu_samples", DateCol: "recorded_at", Label: "Histórico agregado ONU (OLT)", Category: "olt"},
	{Table: "bng_stats_samples", DateCol: "collected_at", Label: "Totais BNG (monitoramento)", Category: "bng"},
	{Table: "bng_session_snapshots", DateCol: "captured_at", Label: "Snapshots sessões PPPoE BNG", Category: "bng"},
	{Table: "events", DateCol: "created_at", Label: "Eventos do sistema", Category: "sistema"},
	{Table: "snmp_walk_jobs", DateCol: "created_at", Label: "Jobs SNMP walk", Category: "sistema"},
	{Table: "onu_report_runs", DateCol: "started_at", Label: "Execuções relatório ONU", Category: "olt"},
	{Table: "automation_execution_log", DateCol: "started_at", Label: "Histórico de automações", Category: "sistema"},
	{Table: "integration_run_logs", DateCol: "created_at", Label: "Logs de integrações", Category: "sistema"},
	{Table: "settings_connection_audit", DateCol: "created_at", Label: "Auditoria de ligações BD", Category: "sistema"},
	{Table: "alert_instances", DateCol: "closed_at", Label: "Alertas encerrados", Category: "alertas", Guard: "closed_only"},
}

type dbCleanupScanRequest struct {
	OlderThanDays int      `json:"older_than_days"`
	Tables        []string `json:"tables,omitempty"`
}

func parseCleanupScanDays(body dbCleanupScanRequest) (int, error) {
	d := body.OlderThanDays
	if d <= 0 {
		d = 30
	}
	if d < databaseCleanupScanMinDays {
		return 0, fmt.Errorf("o mínimo para análise é %d dia(s)", databaseCleanupScanMinDays)
	}
	if d > 3650 {
		return 0, fmt.Errorf("máximo 3650 dias")
	}
	return d, nil
}

func parseCleanupExecuteDays(body dbCleanupExecuteRequest) (int, error) {
	d := body.OlderThanDays
	if d <= 0 {
		d = 30
	}
	if d < databaseCleanupExecuteMinDays {
		return 0, fmt.Errorf("o mínimo para eliminação é %d dias", databaseCleanupExecuteMinDays)
	}
	if d > 3650 {
		return 0, fmt.Errorf("máximo 3650 dias")
	}
	return d, nil
}

func filterCleanupSpecs(tables []string) []dbCleanupTableSpec {
	if len(tables) == 0 {
		return dbCleanupTables
	}
	allowed := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			allowed[t] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return dbCleanupTables
	}
	out := make([]dbCleanupTableSpec, 0, len(dbCleanupTables))
	for _, spec := range dbCleanupTables {
		if _, ok := allowed[strings.ToLower(spec.Table)]; ok {
			out = append(out, spec)
		}
	}
	return out
}

func cleanupWhereClause(spec dbCleanupTableSpec) string {
	switch spec.Guard {
	case "closed_only":
		return spec.DateCol + " IS NOT NULL AND " + spec.DateCol + " < $1"
	default:
		return spec.DateCol + " < $1"
	}
}

func (s *Server) tableExists(ctx context.Context, table string) (bool, error) {
	pool := s.DB()
	if pool == nil {
		return false, fmt.Errorf("base indisponível")
	}
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, table).Scan(&ok)
	return ok, err
}

func (s *Server) databaseCleanupOverview(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusInternalServerError, "DB", "base indisponível", nil)
		return
	}
	ctx := r.Context()

	var dbSize int64
	_ = pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&dbSize)

	items := make([]map[string]any, 0, len(dbCleanupTables))
	var totalRows int64
	for _, spec := range dbCleanupTables {
		item := map[string]any{
			"table":       spec.Table,
			"label":       spec.Label,
			"date_column": spec.DateCol,
			"category":    spec.Category,
		}
		exists, err := s.tableExists(ctx, spec.Table)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if !exists {
			item["exists"] = false
			item["total_count"] = 0
			items = append(items, item)
			continue
		}
		item["exists"] = true
		q := fmt.Sprintf(`
			SELECT COUNT(*)::bigint,
			       MIN(%s),
			       MAX(%s),
			       pg_total_relation_size('%s'::regclass)
			FROM %s
		`, spec.DateCol, spec.DateCol, spec.Table, spec.Table)
		var count, relSize int64
		var oldest, newest *time.Time
		if err := pool.QueryRow(ctx, q).Scan(&count, &oldest, &newest, &relSize); err != nil {
			item["error"] = err.Error()
			items = append(items, item)
			continue
		}
		totalRows += count
		item["total_count"] = count
		item["size_bytes"] = relSize
		if oldest != nil {
			item["oldest_at"] = oldest.UTC().Format(time.RFC3339)
		}
		if newest != nil {
			item["newest_at"] = newest.UTC().Format(time.RFC3339)
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"database_size_bytes": dbSize,
		"total_rows":          totalRows,
		"items":               items,
		"scanned_at":          time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) databaseCleanupScan(w http.ResponseWriter, r *http.Request) {
	var body dbCleanupScanRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	days, err := parseCleanupScanDays(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	specs := filterCleanupSpecs(body.Tables)
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	items, total, err := s.scanDatabaseCleanup(r.Context(), specs, cutoff)
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
		"tables":          body.Tables,
	})
}

func (s *Server) scanDatabaseCleanup(ctx context.Context, specs []dbCleanupTableSpec, cutoff time.Time) ([]map[string]any, int64, error) {
	pool := s.DB()
	if pool == nil {
		return nil, 0, fmt.Errorf("base indisponível")
	}
	var items []map[string]any
	var total int64
	for _, spec := range specs {
		item := map[string]any{
			"table":       spec.Table,
			"label":       spec.Label,
			"date_column": spec.DateCol,
			"category":    spec.Category,
		}
		exists, err := s.tableExists(ctx, spec.Table)
		if err != nil {
			return nil, 0, err
		}
		if !exists {
			item["exists"] = false
			item["count"] = 0
			items = append(items, item)
			continue
		}
		item["exists"] = true
		where := cleanupWhereClause(spec)
		q := fmt.Sprintf(`SELECT COUNT(*)::bigint, MIN(%s) FROM %s WHERE %s`, spec.DateCol, spec.Table, where)
		var n int64
		var oldest *time.Time
		if err := pool.QueryRow(ctx, q, cutoff).Scan(&n, &oldest); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", spec.Table, err)
		}
		total += n
		item["count"] = n
		if oldest != nil {
			item["oldest_eligible_at"] = oldest.UTC().Format(time.RFC3339)
		}
		items = append(items, item)
	}
	return items, total, nil
}

type dbCleanupExecuteRequest struct {
	OlderThanDays int      `json:"older_than_days"`
	Tables        []string `json:"tables,omitempty"`
	Confirm       bool     `json:"confirm"`
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
	days, err := parseCleanupExecuteDays(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	specs := filterCleanupSpecs(body.Tables)
	if len(specs) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nenhuma tabela seleccionada", nil)
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	items, total, err := s.scanDatabaseCleanup(r.Context(), specs, cutoff)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if total == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "deleted_total": 0, "items": items, "message": "Nenhum registo antigo para apagar nas tabelas seleccionadas.",
		})
		return
	}

	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusInternalServerError, "DB", "base indisponível", nil)
		return
	}
	deleted := make([]map[string]any, 0, len(specs))
	var deletedTotal int64
	for _, spec := range specs {
		exists, err := s.tableExists(r.Context(), spec.Table)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if !exists {
			continue
		}
		where := cleanupWhereClause(spec)
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
