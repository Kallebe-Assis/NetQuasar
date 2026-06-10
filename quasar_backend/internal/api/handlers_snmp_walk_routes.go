package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
)

func (s *Server) snmpWalkDeviceRun(w http.ResponseWriter, r *http.Request) {
	s.setMonitoringActivity(r.Context(), "Executando SNMP walk de descoberta")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var ip *string
	var comm *string
	_ = s.DB().QueryRow(r.Context(), `SELECT host(ip)::text, snmp_community FROM devices WHERE id=$1`, id).Scan(&ip, &comm)
	host := ""
	if ip != nil {
		host = strings.TrimSpace(*ip)
	}
	c := ""
	if comm != nil {
		c = strings.TrimSpace(*comm)
	}
	if c == "" {
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&comm)
		if comm != nil {
			c = strings.TrimSpace(*comm)
		}
	}
	var jid uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO snmp_walk_jobs (device_id, host, community, scope, status) VALUES ($1,$2,$3,'mib2_walk','queued') RETURNING id
	`, id, host, c).Scan(&jid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	devID := id
	jobID := jid
	go func() {
		unlockSNMP := snmpdevicelock.Acquire(devID)
		defer unlockSNMP()
		pool := s.DB()
		if pool == nil {
			s.setMonitoringActivity(context.Background(), "")
			return
		}
		l := s.Log.With().Str("job", jobID.String()).Str("device", devID.String()).Logger()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='running' WHERE id=$1`, jobID)
		if err := snmpdiscovery.Run(ctx, pool, &l, devID); err != nil {
			b, _ := json.Marshal(map[string]any{"error": err.Error()})
			_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='failed', error_message=$2, result=$3::jsonb, finished_at=now() WHERE id=$1`, jobID, err.Error(), b)
			s.setMonitoringActivity(context.Background(), "")
			return
		}
		var rc int
		var tr bool
		_ = pool.QueryRow(ctx, `SELECT row_count, truncated FROM device_snmp_inventory WHERE device_id=$1`, devID).Scan(&rc, &tr)
		rb, _ := json.Marshal(map[string]any{
			"status":    "done",
			"row_count": rc,
			"truncated": tr,
			"note":      "Walk MIB-II (1.3.6.1.2.1) gravado em device_snmp_inventory; perfil em collect_profile.",
		})
		_, _ = pool.Exec(ctx, `UPDATE snmp_walk_jobs SET status='done', result=$2::jsonb, finished_at=now(), error_message=NULL WHERE id=$1`, jobID, rb)
		s.setMonitoringActivity(context.Background(), "")
	}()
	s.auditDeviceAction(r.Context(), r, id, "snmp_discover", map[string]any{
		"host":   host,
		"job_id": jid.String(),
		"scope":  "mib2_walk",
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jid, "status": "queued", "note": "Walk SNMP em execução em background; consulte GET /snmp-walk/jobs/{jobId} ou GET /devices/{id}/snmp-inventory."})
}

func (s *Server) snmpWalkJobGet(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "jobId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var st, sc, host, comm *string
	var res []byte
	var created, finished *time.Time
	err = s.DB().QueryRow(r.Context(), `
		SELECT status, scope, host, community, result::text, created_at, finished_at FROM snmp_walk_jobs WHERE id=$1
	`, id).Scan(&st, &sc, &host, &comm, &res, &created, &finished)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out := map[string]any{"id": id, "status": st, "scope": sc, "host": host, "created_at": created, "finished_at": finished}
	if len(res) > 0 {
		out["result"] = json.RawMessage(res)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) snmpWalkCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var res []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT result::text FROM snmp_walk_jobs
		WHERE device_id = $1 AND status = 'done' AND result IS NOT NULL
		ORDER BY COALESCE(finished_at, created_at) DESC LIMIT 1
	`, id).Scan(&res)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "candidates": []any{}, "note": "nenhum walk concluído para este equipamento"})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var parsed map[string]any
	_ = json.Unmarshal(res, &parsed)
	cands := []any{}
	if v, ok := parsed["snmp"]; ok && v != nil {
		cands = append(cands, map[string]any{"kind": "snmp_sample", "data": v})
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "candidates": cands})
}
