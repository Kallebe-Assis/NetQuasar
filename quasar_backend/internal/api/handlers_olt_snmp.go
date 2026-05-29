package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
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
			COALESCE(o.summary::text, '{}'), COALESCE(o.pons::text, '[]'), o.updated_at
		FROM devices d
		LEFT JOIN commercial_localities l ON l.id = d.locality_id
		LEFT JOIN olt_snapshots o ON o.device_id = d.id
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
		var sum, pons string
		var snapAt *time.Time
		if err := rows.Scan(&id, &desc, &ip, &brand, &model, &locID, &locName, &sum, &pons, &snapAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		item := map[string]any{
			"id": id, "description": desc, "ip": ip, "brand": brand, "model": model,
			"locality_id": locID, "locality_name": locName,
			"summary": json.RawMessage(sum), "pons": json.RawMessage(pons),
		}
		item["computed"] = oltparse.SnapshotComputed([]byte(sum), []byte(pons))
		if snapAt != nil {
			item["olt_snapshot_at"] = snapAt.UTC().Format(time.RFC3339)
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
	summary := map[string]any{
		"source":     "olt_refresh",
		"status":     "updated",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	pons := []any{}

	var ip *string
	var comm *string
	var brand, model, devDesc string
	var maxPons *int
	_ = s.DB().QueryRow(r.Context(), `
		SELECT host(d.ip)::text, d.snmp_community,
			coalesce(trim(d.brand), ''), coalesce(trim(d.model), ''),
			coalesce(trim(d.description), ''), d.max_pons
		FROM devices d WHERE d.id=$1
	`, id).Scan(&ip, &comm, &brand, &model, &devDesc, &maxPons)
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
	if host == "" || c == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "IP e community SNMP obrigatórios para refresh OLT", nil)
		return
	}
	if strings.TrimSpace(model) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "modelo OLT obrigatório — seleccione em Equipamentos e configure o perfil em Definições", nil)
		return
	}

	profile, profErr := loadOltCollectionProfile(r.Context(), s.DB(), brand, model)
	if profErr != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION",
			fmt.Sprintf("perfil OLT não encontrado para %s / %s — cadastre em Definições → OLT vendors", brand, model), nil)
		return
	}

	collTO := s.loadCollectionTimeouts(r.Context())
	oltRefreshTotal := collTO.OltRefreshTotal()
	if oltcollect.IsSimpleOnuCollect(profile.Steps) || strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope"))) == oltcollect.ScopeOnu {
		oltRefreshTotal = 100 * time.Second
	}
	telnetTO := collTO.TelnetPhaseTimeout(oltRefreshTotal)
	oltCtx, oltCancel := context.WithTimeout(r.Context(), oltRefreshTotal)
	defer oltCancel()

	fullTelemetry := strings.TrimSpace(r.URL.Query().Get("telemetry")) == "1"
	scope := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = oltcollect.ScopeFull
	}
	if scope == "fast" {
		scope = oltcollect.ScopeOnu
	}
	refreshT0 := time.Now()
	maxPonsVal := 0
	if maxPons != nil && *maxPons > 0 {
		maxPonsVal = *maxPons
	}
	execSt := &oltCollectExecState{
		DeviceID: id, Host: host, Community: c,
		Brand: brand, Model: model, DevDesc: devDesc,
		MaxPons: maxPonsVal,
		Profile: profile, Summary: summary, Pons: pons,
		FullTelemetry: fullTelemetry, TelnetTO: telnetTO,
		Scope: scope,
	}
	if err := s.executeOltProfile(oltCtx, execSt); err != nil {
		summary["olt_profile_exec_error"] = err.Error()
	}
	if oltCtx.Err() != nil {
		summary["olt_refresh_timeout"] = true
		summary["olt_refresh_timeout_reason"] = oltCtx.Err().Error()
		if _, ok := summary["olt_refresh_cancelled"]; !ok {
			summary["olt_refresh_cancelled"] = oltCtx.Err().Error()
		}
	}
	pons = execSt.Pons
	pons = applyMaxPonsLimitAnyRows(pons, maxPons)
	for k, v := range execSt.Summary {
		summary[k] = v
	}
	summary["olt_refresh_elapsed_ms"] = time.Since(refreshT0).Milliseconds()

	if pool := s.DB(); pool != nil {
		thPct, okPct := loadGlobalMetricThreshold(r.Context(), pool, "olt_onu_drop_percent")
		if okPct {
			var prevSnapPons []byte
			_ = pool.QueryRow(r.Context(), `SELECT COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1`, id).Scan(&prevSnapPons)
			prevMaps := oltifderive.PonsJSONToMaps(prevSnapPons)
			prevOn := map[string]float64{}
			for _, p := range prevMaps {
				k := oltifderive.StablePonRowKey(p)
				if k == "" {
					continue
				}
				if v, ok := oltifderive.OnuOnlineFromRow(p); ok {
					prevOn[k] = v
				}
			}
			for _, anyP := range pons {
				p, okMap := anyP.(map[string]any)
				if !okMap {
					continue
				}
				k := oltifderive.StablePonRowKey(p)
				if k == "" {
					continue
				}
				curOn, curOK := oltifderive.OnuOnlineFromRow(p)
				prev, prevOK := prevOn[k]
				if !curOK || !prevOK || prev <= curOn {
					continue
				}
				drop := prev - curOn
				dropPct := 0.0
				if prev > 0 {
					dropPct = (drop / prev) * 100.0
				}
				prevSt := fmt.Sprintf("onu_online_%.0f", prev)
				currSt := fmt.Sprintf("onu_online_%.0f", curOn)
				// Critério único: queda percentual configurada em alert_rules (>= limiar).
				closeAlertByMetaKey(r.Context(), pool, &s.Log, id, "olt_onu_drop", "onu_drop_count:"+k)
				sevPct := evalThresholdSeverity(dropPct, thPct)
				keyPct := "onu_drop_pct:" + k
				oltLabel := strings.TrimSpace(devDesc)
				if oltLabel == "" {
					oltLabel = host
				}
				msgPct := fmt.Sprintf("Queda de %.0f%% (%.0f ONUs) das ONUs online na PON %s da OLT %s (%s).", dropPct, drop, k, oltLabel, host)
				if sevPct != "ok" {
					metaPct := alertnotify.WithStatusTransition(map[string]any{
						"source":            "olt_refresh",
						"pon":               k,
						"drop_online_count": drop,
						"drop_online_pct":   dropPct,
						"prev_online":       prev,
						"curr_online":       curOn,
						"key":               keyPct,
					}, prevSt, currSt, nil)
					created, aid, err := openOrUpdateAlertWithMeta(r.Context(), pool, id, sevPct, "olt_onu_drop", msgPct, host, oltLabel, metaPct)
					if err == nil && created && aid != uuid.Nil {
						alertnotify.SendMonitoringTelegramAndPatchMeta(r.Context(), pool, &s.Log, aid, strings.ToUpper(sevPct), "Queda percentual de ONUs online — PON", msgPct)
					}
				} else {
					closeAlertByMetaKey(r.Context(), pool, &s.Log, id, "olt_onu_drop", keyPct)
				}
			}
		}
	}

	sb, _ := json.Marshal(summary)
	pb, _ := json.Marshal(pons)
	_, err = s.DB().Exec(r.Context(), `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, $3::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET summary = excluded.summary, pons = excluded.pons, updated_at = now()
	`, id, sb, pb)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if pool := s.DB(); pool != nil {
		recordOLTOnuSample(r.Context(), pool, id, sb, pb)
	}
	comp := oltparse.SnapshotComputed(sb, pb)
	s.appendAuditLog(r.Context(), "device", id.String(), "refresh_olt", actorFromRequest(r), nil, map[string]any{
		"timeout_ms": oltRefreshTotal.Milliseconds(), "computed": comp,
	})
	s.getOLTDevice(w, r)
}
