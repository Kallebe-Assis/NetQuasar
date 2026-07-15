package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

func applyMaxPonsLimitAnyRows(pons []any, maxPons *int) []any {
	if maxPons == nil || *maxPons <= 0 {
		return pons
	}
	return oltifderive.FilterPonAnyRows(pons, *maxPons)
}

func (s *Server) listOLTDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.description, host(d.ip)::text, d.brand, d.model, d.locality_id, l.name,
			d.max_pons, COALESCE(d.pon_descriptions::text, '{}'), COALESCE(d.pon_vlans::text, '{}'),
			COALESCE(o.summary::text, '{}'), COALESCE(o.pons::text, '[]'), o.updated_at,
			c.snmp_health_status, c.snmp_health_reason, c.snmp_health_checked_at
		FROM devices d
		LEFT JOIN commercial_localities l ON l.id = d.locality_id
		LEFT JOIN olt_snapshots o ON o.device_id = d.id
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
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
		var desc, ip, brand, model, locName *string
		var locID *uuid.UUID
		var maxPons *int
		var ponDesc, ponVlans, sum, pons string
		var snapAt *time.Time
		var hStatus, hReason *string
		var hAt *time.Time
		if err := rows.Scan(&id, &desc, &ip, &brand, &model, &locID, &locName, &maxPons, &ponDesc, &ponVlans, &sum, &pons, &snapAt, &hStatus, &hReason, &hAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		item := map[string]any{
			"id": id, "description": desc, "ip": ip, "brand": brand, "model": model,
			"locality_id": locID, "locality_name": locName,
			"max_pons": maxPons,
			"pon_descriptions": normalizePonDescriptionsJSON(json.RawMessage(ponDesc)),
			"pon_vlans":        normalizePonVlansJSON(json.RawMessage(ponVlans)),
			"summary": json.RawMessage(sum), "pons": json.RawMessage(pons),
		}
		item["computed"] = oltparse.SnapshotComputed([]byte(sum), []byte(pons))
		if snapAt != nil {
			item["olt_snapshot_at"] = snapAt.UTC().Format(time.RFC3339)
		}
		var summaryMap map[string]any
		_ = json.Unmarshal([]byte(sum), &summaryMap)
		oltStatus, oltReason := oltcollect.DeriveSnmpHealthFromSummary(summaryMap)
		if oltStatus != "unknown" {
			item["snmp_health_status"] = oltStatus
			if oltReason != "" {
				item["snmp_health_reason"] = oltReason
			}
		} else {
			item["snmp_health_status"] = hStatus
			item["snmp_health_reason"] = hReason
		}
		if hAt != nil {
			item["snmp_health_checked_at"] = hAt.UTC().Format(time.RFC3339)
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"olts": list})
}

func (s *Server) getOLTDevice(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var desc, cat string
	var ip *string
	err = s.DB().QueryRow(r.Context(), `SELECT description, category, host(ip)::text FROM devices WHERE id=$1`, id).Scan(&desc, &cat, &ip)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if strings.ToLower(cat) != "olt" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "equipamento não é OLT", nil)
		return
	}
	var sum, pons []byte
	var snapAt *time.Time
	snapErr := s.DB().QueryRow(r.Context(), `SELECT summary::text, pons::text, updated_at FROM olt_snapshots WHERE device_id=$1`, id).Scan(&sum, &pons, &snapAt)
	if snapErr == pgx.ErrNoRows {
		sum = []byte("{}")
		pons = []byte("[]")
		snapAt = nil
	} else if snapErr != nil {
		writeErr(w, http.StatusInternalServerError, "DB", snapErr.Error(), nil)
		return
	}
	out := map[string]any{
		"id": id, "description": desc, "ip": ip,
		"summary":         json.RawMessage(sum),
		"pons":            json.RawMessage(pons),
		"computed":        oltparse.SnapshotComputed(sum, pons),
		"pons_table":      oltparse.PonRows(pons),
		"interface_table": []any{},
		"optical_sensors": []any{},
		"vsol_onu_table":  []any{},
	}
	var sumObj map[string]any
	if json.Unmarshal(sum, &sumObj) == nil && sumObj != nil {
		if z, ok := sumObj["zte_onu_online_table"]; ok {
			out["zte_onu_online_table"] = z
		} else {
			out["zte_onu_online_table"] = []any{}
		}
		if z, ok := sumObj["zte_pon_status_table"]; ok {
			out["zte_pon_status_table"] = z
		} else {
			out["zte_pon_status_table"] = []any{}
		}
		if z, ok := sumObj["zte_transceiver_table"]; ok {
			out["zte_transceiver_table"] = z
		} else {
			out["zte_transceiver_table"] = []any{}
		}
	} else {
		out["zte_onu_online_table"] = []any{}
		out["zte_pon_status_table"] = []any{}
		out["zte_transceiver_table"] = []any{}
	}
	if snapAt != nil {
		out["olt_snapshot_at"] = snapAt.UTC().Format(time.RFC3339)
	}
	if arr := vsolparse.VsolOnuRowsFromSummaryBlob(sum); len(arr) > 0 {
		out["vsol_onu_table"] = arr
	} else {
		out["vsol_onu_table"] = []any{}
	}
	ifRaw, ifCollected, ifPrevRaw, ifPrevCollected, ifErr := loadLatestTwoInterfaceSnapshots(r.Context(), s.DB(), id)
	if ifErr == nil && len(ifRaw) > 0 {
		for k, v := range buildInterfaceMonitorPayload(ifRaw, ifCollected, ifPrevRaw, ifPrevCollected) {
			out[k] = v
		}
		ifTab := snmpifparse.BuildIfTable(walkJSONToSNMPVars(ifRaw))
		ifNameByIndex := make(map[int]string, len(ifTab))
		for _, r := range ifTab {
			lb := strings.TrimSpace(r.DisplayName)
			if lb == "" {
				lb = strings.TrimSpace(r.IfName)
			}
			if lb == "" {
				lb = strings.TrimSpace(r.Descr)
			}
			ifNameByIndex[r.IfIndex] = lb
		}
		if src, ok := out["zte_onu_online_table"].([]any); ok {
			out["zte_onu_online_table"] = enrichZTERows(src, ifNameByIndex, false)
		}
		if src, ok := out["zte_pon_status_table"].([]any); ok {
			out["zte_pon_status_table"] = enrichZTERows(src, ifNameByIndex, true)
		}
		if src, ok := out["zte_transceiver_table"].([]any); ok {
			out["zte_transceiver_table"] = enrichZTERows(src, ifNameByIndex, false)
		}
	}
	if ifCollected != nil {
		out["interface_collected_at"] = ifCollected.UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(strings.ToLower(cat)) == "olt" {
		if tab, ok := out["interface_table"].([]map[string]any); ok {
			oltifderive.AnnotateInterfaceTable(tab)
		}
	}
	if sumObj != nil {
		out["collection_log"] = buildOltCollectionLog(sumObj)
		if dbg, ok := sumObj["snmp_debug"].(map[string]any); ok {
			out["snmp_debug"] = dbg
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) refreshOLTDevice(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 3*time.Minute)
	s.setMonitoringActivity(r.Context(), "Coletando PONs OLT")
	defer s.setMonitoringActivity(r.Context(), "")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	unlockSNMP := snmpdevicelock.Acquire(id)
	defer unlockSNMP()

	fullTelemetry := strings.TrimSpace(r.URL.Query().Get("telemetry")) == "1"
	scope := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope")))
	res, err := s.refreshOLTDeviceCore(r.Context(), id, OltRefreshCoreOpts{
		Source:        "olt_refresh",
		Scope:         scope,
		FullTelemetry: fullTelemetry,
	})
	if err != nil {
		if strings.Contains(err.Error(), "community SNMP") || strings.Contains(err.Error(), "modelo OLT") {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
			return
		}
		if strings.Contains(err.Error(), "perfil OLT não encontrado") {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "device", id.String(), "refresh_olt", s.actorFromRequest(r), nil, map[string]any{
		"timeout_ms": res.TimeoutMs, "pon_count": res.PonCount, "ok": res.OK,
	})
	s.getOLTDevice(w, r)
}
