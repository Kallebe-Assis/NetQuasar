package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorview"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

func (s *Server) pingLatest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var checkedAt time.Time
	var ok bool
	var lat *int64
	var method *string
	var source string
	var detail []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT checked_at, ok, latency_ms, method, source, detail::text
		FROM ping_history WHERE device_id=$1 ORDER BY checked_at DESC LIMIT 1
	`, id).Scan(&checkedAt, &ok, &lat, &method, &source, &detail)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "note": "sem histórico de ping ainda"})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out := map[string]any{"device_id": id, "checked_at": checkedAt, "ok": ok, "source": source}
	if lat != nil {
		out["latency_ms"] = *lat
	}
	if method != nil {
		out["method"] = *method
	}
	if len(detail) > 0 {
		out["detail"] = json.RawMessage(detail)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pingHistory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	q := `SELECT id, device_id, checked_at, ok, latency_ms, method, source FROM ping_history WHERE 1=1`
	args := []any{}
	n := 1
	if deviceID != "" {
		did, err := uuid.Parse(deviceID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_QUERY", "device_id UUID inválido", nil)
			return
		}
		q += ` AND device_id = $` + strconv.Itoa(n)
		args = append(args, did)
		n++
	}
	if from != "" {
		q += ` AND checked_at >= $` + strconv.Itoa(n)
		args = append(args, from)
		n++
	}
	if to != "" {
		q += ` AND checked_at <= $` + strconv.Itoa(n)
		args = append(args, to)
		n++
	}
	q += ` ORDER BY checked_at DESC LIMIT ` + strconv.Itoa(limit)

	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id int64
		var did uuid.UUID
		var t time.Time
		var ok bool
		var lat *int64
		var method, source *string
		if err := rows.Scan(&id, &did, &t, &ok, &lat, &method, &source); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		row := map[string]any{"id": id, "device_id": did, "checked_at": t, "ok": ok}
		if lat != nil {
			row["latency_ms"] = *lat
		}
		if method != nil {
			row["method"] = *method
		}
		if source != nil {
			row["source"] = *source
		}
		list = append(list, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": list})
}

func (s *Server) telemetryCollect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var ip *string
	var devComm *string
	var devDesc, devCat, devBrand, devModel string
	err = s.DB().QueryRow(r.Context(), `
		SELECT host(ip)::text, snmp_community,
			coalesce(description,''), coalesce(category,''), coalesce(brand,''), coalesce(model,'')
		FROM devices WHERE id=$1
	`, id).Scan(&ip, &devComm, &devDesc, &devCat, &devBrand, &devModel)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ip == nil || strings.TrimSpace(*ip) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "equipamento sem IP para SNMP", nil)
		return
	}
	var defComm *string
	_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
	comm := ""
	if devComm != nil && strings.TrimSpace(*devComm) != "" {
		comm = strings.TrimSpace(*devComm)
	} else if defComm != nil {
		comm = strings.TrimSpace(*defComm)
	}
	if comm == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "defina snmp_community no equipamento ou em settings_connection_defaults", nil)
		return
	}
	unlockSNMP := snmpdevicelock.Acquire(id)
	defer unlockSNMP()
	// Coleta manual pode precisar atualizar inventário SNMP antes de ler métricas.
	// Alguns equipamentos levam >25s para completar esse ciclo.
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if _, err := snmpdiscovery.EnsureFreshInventory(ctx, s.DB(), &s.Log, id, snmpdiscovery.DefaultInventoryMaxAge); err != nil {
		code := "SNMP_DISCOVERY"
		msg := err.Error()
		low := strings.ToLower(msg)
		if strings.Contains(low, "context deadline exceeded") || strings.Contains(low, "deadline exceeded") {
			code = "SNMP_TIMEOUT"
			msg = "tempo limite da coleta SNMP excedido; tente novamente ou valide conectividade/OIDs"
		}
		writeErr(w, http.StatusInternalServerError, code, msg, nil)
		return
	}
	col, err := telemetryengine.CollectAndStore(ctx, s.DB(), id, strings.TrimSpace(*ip), comm)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if col.Metrics != nil {
		if mb, mErr := json.Marshal(col.Metrics); mErr == nil {
			monitorview.PatchProbeKPIs(ctx, s.DB(), id, mb, time.Now())
		}
	}
	monitorworker.RunPostTelemetryAlertEval(ctx, s.DB(), &s.Log, id, devDesc, strings.TrimSpace(*ip), comm, devCat, devBrand, devModel, col)
	monitorworker.NudgeMonitoringRuntimeRefresh(ctx, s.DB())
	s.auditDeviceAction(ctx, r, id, "collect_telemetry", map[string]any{
		"description": devDesc,
		"ip":          strings.TrimSpace(*ip),
		"ok":          col.OK,
	})
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "ok": col.OK, "metrics": col.Metrics})
}

func (s *Server) telemetryLatest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var collected time.Time
	var metrics []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT collected_at, metrics::text FROM telemetry_samples WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
	`, id).Scan(&collected, &metrics)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "note": "sem telemetria persistida"})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "collected_at": collected, "metrics": json.RawMessage(metrics)})
}

func (s *Server) telemetryHistory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "query device_id obrigatório", nil)
		return
	}
	did, err := uuid.Parse(deviceID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	q := `SELECT id, collected_at, metrics::text FROM telemetry_samples WHERE device_id=$1`
	args := []any{did}
	n := 2
	if from != "" {
		q += ` AND collected_at >= $` + strconv.Itoa(n)
		args = append(args, from)
		n++
	}
	if to != "" {
		q += ` AND collected_at <= $` + strconv.Itoa(n)
		args = append(args, to)
		n++
	}
	q += ` ORDER BY collected_at DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id int64
		var ts time.Time
		var m []byte
		if err := rows.Scan(&id, &ts, &m); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{"id": id, "collected_at": ts, "metrics": json.RawMessage(m)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": list})
}
