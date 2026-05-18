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
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/zteparse"
)

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
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) refreshOLTDevice(w http.ResponseWriter, r *http.Request) {
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
	_ = s.DB().QueryRow(r.Context(), `
		SELECT host(d.ip)::text, d.snmp_community,
			lower(coalesce(trim(d.brand), '')), lower(coalesce(trim(d.model), '')),
			coalesce(trim(d.description), '')
		FROM devices d WHERE d.id=$1
	`, id).Scan(&ip, &comm, &brand, &model, &devDesc)
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
	collTO := s.loadCollectionTimeouts(r.Context())
	oltRefreshTotal := collTO.OltRefreshTotal()
	snmpWalkTO := collTO.SnmpPerWalkTimeout(oltRefreshTotal)
	telnetTO := collTO.TelnetPhaseTimeout(oltRefreshTotal)
	oltCtx, oltCancel := context.WithTimeout(r.Context(), oltRefreshTotal)
	defer oltCancel()

	m := strings.ToLower(model)
	isVsol := strings.Contains(brand, "vsol") || strings.Contains(model, "vsol") ||
		strings.Contains(m, "v1600") || strings.Contains(m, "1600g")
	isZTE := strings.Contains(brand, "zte") || strings.Contains(model, "zte") || strings.Contains(m, "zxa10")
	isDatacom := strings.Contains(brand, "datacom") || strings.Contains(model, "datacom") || strings.Contains(m, "dm46")
	if isVsol && host != "" && c != "" {
		walk, trunc, note := probing.SNMPWalk(oltCtx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: c, RootOID: vsolparse.OIDGOnuAuthList,
			Version: "2c", Timeout: snmpWalkTO, Retries: 0, MaxRows: 48000,
		})
		walk2, trunc2, note2 := probing.SNMPWalk(oltCtx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: c, RootOID: vsolparse.OIDLegacyGOnuOptical,
			Version: "2c", Timeout: snmpWalkTO / 2, Retries: 0, MaxRows: 12000,
		})
		walk = append(walk, walk2...)
		trunc = trunc || trunc2
		if strings.TrimSpace(note2) != "" {
			if strings.TrimSpace(note) != "" {
				note = note + "; legado óptica: " + note2
			} else {
				note = "legado óptica: " + note2
			}
		}
		vSum, vPons, _ := vsolparse.FromSNMPWalk(walk)
		for k, v := range vSum {
			summary[k] = v
		}
		summary["vsol_walk_truncated"] = trunc
		if strings.TrimSpace(note) != "" {
			summary["vsol_walk_note"] = note
		}
		if len(vPons) > 0 {
			pons = make([]any, 0, len(vPons))
			for _, p := range vPons {
				pons = append(pons, p)
			}
		}
	}
	if isZTE && host != "" && c != "" {
		prof := loadVendorOIDProfile(r.Context(), s.DB(), "ZTE")
		zteSummary := map[string]any{
			"zte_profile": map[string]any{
				"onu_online_oid":  cleanSNMPOID(prof.OnuOnlineOID),
				"pon_status_oid":  cleanSNMPOID(prof.PonStatusOID),
				"transceiver_oid": cleanSNMPOID(prof.TransceiverOID),
				"snmp_base_oid":   cleanSNMPOID(prof.SNMPBaseOID),
			},
		}
		var notes []string
		onuRows, truncOnu, noteOnu := walkRootRows(oltCtx, host, c, prof.OnuOnlineOID)
		if len(onuRows) > 0 {
			zteSummary["zte_onu_online_table"] = onuRows
		} else {
			zteSummary["zte_onu_online_table"] = []any{}
		}
		ponRows, truncPon, notePon := walkRootRows(oltCtx, host, c, prof.PonStatusOID)
		if len(ponRows) > 0 {
			zteSummary["zte_pon_status_table"] = ponRows
		} else {
			zteSummary["zte_pon_status_table"] = []any{}
		}
		trxRows, truncTrx, noteTrx := walkRootRows(oltCtx, host, c, prof.TransceiverOID)
		if len(trxRows) > 0 {
			zteSummary["zte_transceiver_table"] = trxRows
		} else {
			zteSummary["zte_transceiver_table"] = []any{}
		}
		if strings.TrimSpace(noteOnu) != "" {
			notes = append(notes, "onu_online: "+strings.TrimSpace(noteOnu))
		}
		if strings.TrimSpace(notePon) != "" {
			notes = append(notes, "pon_status: "+strings.TrimSpace(notePon))
		}
		if strings.TrimSpace(noteTrx) != "" {
			notes = append(notes, "transceiver: "+strings.TrimSpace(noteTrx))
		}
		zteSummary["zte_walk_truncated"] = truncOnu || truncPon || truncTrx
		if len(notes) > 0 {
			zteSummary["zte_walk_note"] = strings.Join(notes, "; ")
		}
		for k, v := range zteSummary {
			summary[k] = v
		}
		var telUser, telPass, telEnable *string
		_ = s.DB().QueryRow(r.Context(), `SELECT telnet_user, telnet_password, telnet_enable FROM settings_connection_defaults WHERE id=1`).Scan(&telUser, &telPass, &telEnable)
		tu, tp, te := "", "", ""
		if telUser != nil {
			tu = strings.TrimSpace(*telUser)
		}
		if telPass != nil {
			tp = strings.TrimSpace(*telPass)
		}
		if telEnable != nil {
			te = strings.TrimSpace(*telEnable)
		}
		zteTelnetApplied := false
		if tu != "" && tp != "" {
			tel := probing.TelnetRunCommand(oltCtx, probing.TelnetRunParams{
				Host: host, Port: "23", Timeout: telnetTO,
				User: tu, Password: tp, Enable: te,
				Command:      "show gpon onu state",
				PreCommands:  []string{"terminal length 0", "terminal page-break disable", "scroll 512"},
				MaxReadBytes: 220000,
			})
			summary["zte_telnet_note"] = tel.Note
			summary["zte_telnet_raw_snippet"] = trimForSummary(tel.Output, 2400)
			if !tel.OK {
				summary["zte_telnet_error"] = tel.Error
			} else {
				rows := zteparse.ParseShowGponOnuState(tel.Output)
				summary["zte_telnet_onu_state_rows"] = rows
				summary["zte_telnet_onu_state_count"] = len(rows)
				if len(rows) > 0 {
					zteTelnetApplied = true
					statusByIf := buildZTEPonStatusByIfIndex(ponRows)
					ifNameByIndex := map[int]string{}
					var ifRaw []byte
					if err := s.DB().QueryRow(r.Context(), `SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1`, id).Scan(&ifRaw); err == nil && len(ifRaw) > 0 {
						ifRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(ifRaw))
						for _, r := range ifRows {
							lb := strings.TrimSpace(r.DisplayName)
							if lb == "" {
								lb = strings.TrimSpace(r.IfName)
							}
							if lb == "" {
								lb = strings.TrimSpace(r.Descr)
							}
							ifNameByIndex[r.IfIndex] = lb
						}
					}
					pons = make([]any, 0, len(rows))
					for _, pr := range rows {
						st := "unknown"
						for ifIdx, lb := range ifNameByIndex {
							if ztePonIDFromIfLabel(lb) == strings.TrimSpace(pr.Pon) {
								if x := strings.TrimSpace(statusByIf[ifIdx]); x != "" {
									st = x
								}
								break
							}
						}
						pons = append(pons, map[string]any{
							"id":           pr.Pon,
							"name":         "PON-" + pr.Pon,
							"onu_total":    pr.OnuTotal,
							"onu_online":   pr.OnuOnline,
							"onu_offline":  pr.OnuOffline,
							"status":       st,
							"source_slice": "zte_telnet_show_gpon_onu_state",
						})
					}
				}
			}
		}
		if !zteTelnetApplied {
			summary["zte_telnet_note"] = anyString(summary["zte_telnet_note"]) + " (sem parse válido; pons não serão inferidas por ifIndex)"
		}
	}
	if isDatacom && host != "" && c != "" {
		prof := loadVendorOIDProfile(r.Context(), s.DB(), "DATACOM")
		datacomSummary := map[string]any{
			"datacom_profile": map[string]any{
				"onu_online_oid":  cleanSNMPOID(prof.OnuOnlineOID),
				"pon_status_oid":  cleanSNMPOID(prof.PonStatusOID),
				"transceiver_oid": cleanSNMPOID(prof.TransceiverOID),
				"snmp_base_oid":   cleanSNMPOID(prof.SNMPBaseOID),
			},
		}
		var notes []string
		onuRows, truncOnu, noteOnu := walkRootRows(oltCtx, host, c, prof.OnuOnlineOID)
		if len(onuRows) > 0 {
			datacomSummary["datacom_onu_online_table"] = onuRows
			dRows := buildDatacomPonRowsFromTable(onuRows)
			if len(dRows) > 0 {
				pons = make([]any, 0, len(dRows))
				for _, pr := range dRows {
					pons = append(pons, pr)
				}
				datacomSummary["datacom_pon_rows_count"] = len(dRows)
			}
		} else {
			datacomSummary["datacom_onu_online_table"] = []any{}
		}
		ponRows, truncPon, notePon := walkRootRows(oltCtx, host, c, prof.PonStatusOID)
		if len(ponRows) > 0 {
			datacomSummary["datacom_pon_status_table"] = ponRows
		} else {
			datacomSummary["datacom_pon_status_table"] = []any{}
		}
		trxRows, truncTrx, noteTrx := walkRootRows(oltCtx, host, c, prof.TransceiverOID)
		if len(trxRows) > 0 {
			datacomSummary["datacom_transceiver_table"] = trxRows
		} else {
			datacomSummary["datacom_transceiver_table"] = []any{}
		}
		if strings.TrimSpace(noteOnu) != "" {
			notes = append(notes, "onu_online: "+strings.TrimSpace(noteOnu))
		}
		if strings.TrimSpace(notePon) != "" {
			notes = append(notes, "pon_status: "+strings.TrimSpace(notePon))
		}
		if strings.TrimSpace(noteTrx) != "" {
			notes = append(notes, "transceiver: "+strings.TrimSpace(noteTrx))
		}
		datacomSummary["datacom_walk_truncated"] = truncOnu || truncPon || truncTrx
		if len(notes) > 0 {
			datacomSummary["datacom_walk_note"] = strings.Join(notes, "; ")
		}
		for k, v := range datacomSummary {
			summary[k] = v
		}
	}

	// Tratamento de consistência: reforça dados de PON/ONU com derivação de IF-MIB (último snapshot de interfaces).
	if pool := s.DB(); pool != nil && !isZTE && !isDatacom {
		var ifRaw []byte
		if err := pool.QueryRow(r.Context(), `
			SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
		`, id).Scan(&ifRaw); err == nil && len(ifRaw) > 0 {
			ifRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(ifRaw))
			optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, walkJSONToSNMPVars(ifRaw))
			derivedPons, sumPatch := oltifderive.DeriveFromIfRows(ifRows, optMap)
			if len(derivedPons) > 0 {
				existing := make([]map[string]any, 0, len(pons))
				for _, p := range pons {
					if m, ok := p.(map[string]any); ok {
						existing = append(existing, m)
					}
				}
				merged := oltifderive.MergePonRowsForIfaceRefresh(existing, derivedPons)
				pons = make([]any, 0, len(merged))
				for _, p := range merged {
					pons = append(pons, p)
				}
				summary["if_mib_merge_applied"] = true
				for k, v := range sumPatch {
					summary[k] = v
				}
			}
		}
	}

	if pool := s.DB(); pool != nil {
		var prevSnapPons, prevSnapSum []byte
		_ = pool.QueryRow(r.Context(), `
			SELECT COALESCE(pons::text,'[]'), COALESCE(summary::text,'{}')
			FROM olt_snapshots WHERE device_id=$1
		`, id).Scan(&prevSnapPons, &prevSnapSum)
		prevMaps := oltifderive.PonsJSONToMaps(prevSnapPons)
		prevSumm := oltifderive.SummaryJSONBytesToMap(prevSnapSum)
		newMaps := oltifderive.PonsAnySliceToMaps(pons)
		stabMaps, stabPatch := oltifderive.StabilizePonSnapshotRows(prevMaps, newMaps, prevSumm)
		pons = oltifderive.PonsMapsToAny(stabMaps)
		for k, v := range stabPatch {
			summary[k] = v
		}

		thPct, okPct := loadGlobalMetricThreshold(r.Context(), pool, "olt_onu_drop_percent")
		if okPct {
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
