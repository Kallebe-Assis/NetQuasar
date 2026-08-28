package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/bgpcollect"
)

// handlers_bgp_report.go — leitura da coleta BGP (telemetry_samples), mirror directo de
// telemetryLatest/telemetryHistory (handlers_ping_telemetry.go) mas com o pivot de
// bgpcollect.BuildReportFromStoredMetrics já aplicado, pronto para a tela BGP.

// listBGPDevices lista equipamentos com coleta BGP activa — mirror de bngListDevices.
func (s *Server) listBGPDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, coalesce(d.description,''), coalesce(host(d.ip)::text,''),
			coalesce(d.brand,''), coalesce(d.model,'')
		FROM devices d
		WHERE coalesce(d.bgp_enabled, false) = true
		ORDER BY d.description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, ip, brand, model string
		if err := rows.Scan(&id, &desc, &ip, &brand, &model); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{
			"id": id, "description": desc, "ip": ip, "brand": brand, "model": model,
		})
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}

// resolveBGPDevice devolve IP (o marcado for_bgp em device_ips, senão o IP primário — mesma
// prioridade de loadBgpDevicesForCollect no ciclo periódico) e a community SNMP efectiva
// (do equipamento, senão o padrão global).
func (s *Server) resolveBGPDevice(ctx context.Context, id uuid.UUID) (ip string, community string, err error) {
	var devComm *string
	err = s.DB().QueryRow(ctx, `
		SELECT host(COALESCE(dip.ip, d.ip))::text, d.snmp_community
		FROM devices d
		LEFT JOIN device_ips dip ON dip.device_id = d.id AND dip.for_bgp = true
		WHERE d.id = $1 AND coalesce(d.bgp_enabled, false) = true
	`, id).Scan(&ip, &devComm)
	if err != nil {
		return "", "", err
	}
	if devComm != nil && strings.TrimSpace(*devComm) != "" {
		return strings.TrimSpace(ip), strings.TrimSpace(*devComm), nil
	}
	var defComm *string
	_ = s.DB().QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
	comm := ""
	if defComm != nil {
		comm = strings.TrimSpace(*defComm)
	}
	return strings.TrimSpace(ip), comm, nil
}

// bgpDeviceCollect faz uma coleta SNMP BGP sob demanda (botão "Atualizar" da tela BGP) — mirror
// de bngDeviceCollect, usa o perfil is_default de bgp_snmp_profiles e grava em telemetry_samples,
// o mesmo destino do ciclo periódico (RunBgpSweep).
func (s *Server) bgpDeviceCollect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	ip, comm, err := s.resolveBGPDevice(r.Context(), id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BGP não encontrado (ou BGP não está activo nele)", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if strings.TrimSpace(ip) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "equipamento sem IP para SNMP", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "defina snmp_community no equipamento ou em Definições → Rede e SNMP", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	profile := bgpcollect.LoadDefaultProfile(ctx, s.DB())
	out, err := bgpcollect.CollectAndStoreForDevice(ctx, s.DB(), id, ip, comm, 30*time.Second, profile)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COLLECT", err.Error(), nil)
		return
	}
	if out.CollectedCount == 0 {
		msg := out.Message
		if msg == "" {
			msg = "nenhuma métrica activa respondeu — confira o perfil SNMP em Configurações → BGP e a community"
		}
		writeErr(w, http.StatusUnprocessableEntity, "COLLECT_EMPTY", msg, map[string]any{"collection": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "device_id": id, "collection": out})
}

func (s *Server) bgpDeviceReport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var collected time.Time
	var metrics []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT collected_at, metrics::text FROM telemetry_samples
		WHERE device_id=$1 AND metrics ? 'bgp_collection'
		ORDER BY collected_at DESC LIMIT 1
	`, id).Scan(&collected, &metrics)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "note": "sem coleta BGP persistida"})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	rep := bgpcollect.BuildReportFromStoredMetrics(metrics)
	rep.CollectedAt = collected.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "collected_at": rep.CollectedAt, "report": rep})
}

func (s *Server) bgpDeviceHistory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := s.DB().Query(r.Context(), `
		SELECT collected_at, metrics::text FROM telemetry_samples
		WHERE device_id=$1 AND metrics ? 'bgp_collection' AND collected_at >= $2
		ORDER BY collected_at DESC LIMIT $3
	`, id, since, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var samples []map[string]any
	for rows.Next() {
		var collected time.Time
		var metrics []byte
		if err := rows.Scan(&collected, &metrics); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		rep := bgpcollect.BuildReportFromStoredMetrics(metrics)
		rep.CollectedAt = collected.UTC().Format(time.RFC3339)
		samples = append(samples, map[string]any{"collected_at": rep.CollectedAt, "report": rep})
	}
	if samples == nil {
		samples = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "samples": samples})
}
