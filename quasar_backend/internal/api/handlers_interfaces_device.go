package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

func (s *Server) listDeviceInterfaces(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ifaces, collected, prevIfaces, prevCollected, err := loadLatestTwoInterfaceSnapshots(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(ifaces) == 0 || collected == nil {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "interfaces": []any{}, "note": "execute POST .../refresh para coletar via SNMP"})
		return
	}
	// Proteção contra snapshots parciais antigos: escolhe o mais completo entre os últimos.
	cRows, qErr := s.DB().Query(r.Context(), `
		SELECT interfaces::text, collected_at
		FROM interface_snapshots
		WHERE device_id=$1
		ORDER BY collected_at DESC
		LIMIT 20
	`, id)
	if qErr == nil {
		defer cRows.Close()
		type cand struct {
			raw []byte
			at  time.Time
		}
		var cands []cand
		for cRows.Next() {
			var raw []byte
			var at time.Time
			if err := cRows.Scan(&raw, &at); err != nil {
				break
			}
			cands = append(cands, cand{raw: raw, at: at})
		}
		bestIdx := 0
		bestN := -1
		for i, c := range cands {
			p := buildInterfaceMonitorPayload(c.raw, &c.at, nil, nil)
			n := 0
			if tab, ok := p["interface_table"].([]map[string]any); ok {
				n = len(tab)
			}
			if n > bestN {
				bestN = n
				bestIdx = i
			}
		}
		if len(cands) > 0 && bestN > 1 {
			ifaces = cands[bestIdx].raw
			at := cands[bestIdx].at
			collected = &at
			prevIfaces = nil
			prevCollected = nil
			if bestIdx+1 < len(cands) {
				prevIfaces = cands[bestIdx+1].raw
				pAt := cands[bestIdx+1].at
				prevCollected = &pAt
			}
		}
	}
	useRaw := ifaces
	useAt := collected
	usePrevRaw := prevIfaces
	usePrevAt := prevCollected
	payload := buildInterfaceMonitorPayload(useRaw, useAt, usePrevRaw, usePrevAt)
	latestRows := 0
	if tab, ok := payload["interface_table"].([]map[string]any); ok {
		latestRows = len(tab)
	}
	if latestRows <= 1 && len(prevIfaces) > 0 && prevCollected != nil {
		prevPayload := buildInterfaceMonitorPayload(prevIfaces, prevCollected, nil, nil)
		prevRows := 0
		if tab, ok := prevPayload["interface_table"].([]map[string]any); ok {
			prevRows = len(tab)
		}
		if prevRows > latestRows {
			useRaw = prevIfaces
			useAt = prevCollected
			usePrevRaw = nil
			usePrevAt = nil
			payload = prevPayload
		}
	}
	out := map[string]any{"device_id": id, "collected_at": useAt.UTC().Format(time.RFC3339), "interfaces": json.RawMessage(useRaw)}
	if useAt != collected {
		out["note"] = "último snapshot estava parcial; exibindo snapshot anterior mais completo"
	}
	// Se não houver tráfego calculado por delta de snapshots, tenta taxa instantânea por dupla amostragem SNMP.
	if tab, ok := payload["interface_table"].([]map[string]any); ok && len(tab) > 0 {
		hasRates := false
		for _, row := range tab {
			if _, ok := row["in_bps"]; ok {
				hasRates = true
				break
			}
		}
		if !hasRates {
			var ip, devComm string
			_ = s.DB().QueryRow(r.Context(), `
				SELECT coalesce(host(ip)::text,''), coalesce(snmp_community,'')
				FROM devices WHERE id=$1
			`, id).Scan(&ip, &devComm)
			comm := strings.TrimSpace(devComm)
			if comm == "" {
				var defComm string
				_ = s.DB().QueryRow(r.Context(), `SELECT coalesce(snmp_community,'') FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
				comm = strings.TrimSpace(defComm)
			}
			host := strings.TrimSpace(ip)
			if host != "" && comm != "" {
				ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
				rates, dtSec := instantTrafficRatesByOID(ctx, host, comm, 2*time.Second)
				cancel()
				if len(rates) > 0 {
					for _, row := range tab {
						ifIdx, ok := row["if_index"].(int)
						if !ok || ifIdx <= 0 {
							continue
						}
						if rr, ok := rates[ifIdx]; ok {
							row["in_bps"] = rr.InBps
							row["out_bps"] = rr.OutBps
						}
					}
					payload["traffic_interval_seconds"] = dtSec
					payload["traffic_source"] = "snmp_oid_dual_sample"
				}
			}
		}
	}
	for k, v := range payload {
		out[k] = v
	}
	if tab, ok := out["interface_table"].([]map[string]any); ok {
		out["interface_count"] = len(tab)
	}
	out["walk_truncated"] = snapshotWalkTruncated(useRaw)
	if note, ok := out["note"].(string); ok && strings.Contains(strings.ToLower(note), "truncad") {
		out["walk_truncated"] = true
	}
	if pool := s.DB(); pool != nil {
		var devCat string
		_ = pool.QueryRow(r.Context(), `SELECT coalesce(lower(trim(category)),'') FROM devices WHERE id=$1`, id).Scan(&devCat)
		if devCat == "olt" {
			if tab, ok := out["interface_table"].([]map[string]any); ok {
				oltifderive.AnnotateInterfaceTable(tab)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) refreshDeviceInterfaces(w http.ResponseWriter, r *http.Request) {
	s.setMonitoringActivity(r.Context(), "Coletando interfaces via SNMP")
	defer s.setMonitoringActivity(r.Context(), "")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var ip *string
	var comm *string
	var devDesc, devCat, devBrand, devModel string
	err = s.DB().QueryRow(r.Context(), `
		SELECT host(ip)::text, snmp_community,
			coalesce(description,''), coalesce(lower(trim(category)),''), coalesce(lower(trim(brand)),''), coalesce(lower(trim(model)), '')
		FROM devices WHERE id=$1
	`, id).Scan(&ip, &comm, &devDesc, &devCat, &devBrand, &devModel)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ip == nil || strings.TrimSpace(*ip) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "IP obrigatório", nil)
		return
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
	if c == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community SNMP obrigatória", nil)
		return
	}
	unlockSNMP := snmpdevicelock.Acquire(id)
	defer unlockSNMP()
	ifRefreshTO := s.loadCollectionTimeouts(r.Context()).InterfaceRefreshTotal()
	ctx, cancel := context.WithTimeout(r.Context(), ifRefreshTO)
	defer cancel()
	host := strings.TrimSpace(*ip)
	isMikrotik := isLikelyMikrotikDevice(devCat, devBrand, devModel, devDesc)
	walkRes := collectInterfaceSNMPWalks(ctx, host, c, ifRefreshTO, isMikrotik)
	merged := walkRes.Merged
	note := walkRes.Note
	arr := make([]map[string]any, 0, len(merged)+1)
	for _, v := range merged {
		arr = append(arr, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
	}
	if walkRes.Truncated {
		arr = append(arr, map[string]any{"oid": "__netquasar.walk", "value": "truncated", "type": "meta"})
	}
	b, _ := json.Marshal(arr)
	latestBeforeInsertRaw, latestBeforeInsertAt, _, _, loadPrevErr := loadLatestTwoInterfaceSnapshots(r.Context(), s.DB(), id)
	if loadPrevErr != nil {
		writeErr(w, http.StatusInternalServerError, "DB", loadPrevErr.Error(), nil)
		return
	}
	var collectedAt time.Time
	err = s.DB().QueryRow(r.Context(), `INSERT INTO interface_snapshots (device_id, interfaces) VALUES ($1, $2::jsonb) RETURNING collected_at`, id, b).Scan(&collectedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	ok := len(merged) > 0
	payload := buildInterfaceMonitorPayload(b, &collectedAt, latestBeforeInsertRaw, latestBeforeInsertAt)
	if isMikrotik {
		if tab, ok := payload["interface_table"].([]map[string]any); ok {
			needInstant := true
			for _, row := range tab {
				if _, hasIn := row["in_bps"]; hasIn {
					needInstant = false
					break
				}
			}
			if needInstant {
				if instRates, instDt := instantTrafficRatesByOID(ctx, host, c, 2*time.Second); len(instRates) > 0 {
					for _, row := range tab {
						ifIdx, ok := row["if_index"].(int)
						if !ok || ifIdx <= 0 {
							continue
						}
						if rr, ok := instRates[ifIdx]; ok {
							row["in_bps"] = rr.InBps
							row["out_bps"] = rr.OutBps
						}
					}
					payload["traffic_interval_seconds"] = instDt
					payload["traffic_source"] = "snmp_oid_dual_sample"
				}
			}
		}
	}
	if strings.TrimSpace(devCat) == "olt" {
		if tab, ok := payload["interface_table"].([]map[string]any); ok {
			oltifderive.AnnotateInterfaceTable(tab)
		}
		if !strings.Contains(strings.ToLower(strings.TrimSpace(devBrand)), "zte") {
			parsedVars := walkJSONToSNMPVars(b)
			ifRows := snmpifparse.BuildIfTable(parsedVars)
			optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, parsedVars)
			derivedPons, sumPatch := oltifderive.DeriveFromIfRows(ifRows, optMap)
			sumPatch["if_mib_derived_at"] = time.Now().UTC().Format(time.RFC3339)
			if err := upsertOltSnapshotAfterInterfaceRefresh(ctx, s.DB(), id, derivedPons, sumPatch); err != nil {
				s.Log.Warn().Err(err).Str("device", id.String()).Msg("olt_snapshots IF-MIB merge")
			}
		}
	}
	if isMikrotik && s.DB() != nil {
		parsedVars := walkJSONToSNMPVars(b)
		ifRows := snmpifparse.BuildIfTable(parsedVars)
		optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, parsedVars)
		sfpEval := make([]alertthresholds.SfpInterfaceRow, 0, len(ifRows))
		for _, r := range ifRows {
			op := optMap[r.IfIndex]
			disp := strings.TrimSpace(r.DisplayName)
			if disp == "" {
				disp = fmt.Sprintf("if%d", r.IfIndex)
			}
			sfpEval = append(sfpEval, alertthresholds.SfpInterfaceRow{
				IfIndex:     r.IfIndex,
				DisplayName: disp,
				Sfp:         snmpmikrotik.IsSfpPort(r.DisplayName, r.Descr, op),
				TxDBm:       copyFloatPtr(op.TxDBm),
				RxDBm:       copyFloatPtr(op.RxDBm),
			})
		}
		alertthresholds.EvaluateMikrotikSFPAfterSnapshot(ctx, s.DB(), &s.Log, id, devDesc, host, sfpEval)
	}
	if s.DB() != nil && latestBeforeInsertRaw != nil {
		prevRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(latestBeforeInsertRaw))
		currRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(b))
		prevBy := map[int]snmpifparse.IfRow{}
		for _, r := range prevRows {
			prevBy[r.IfIndex] = r
		}
		if th, ok := loadGlobalMetricThreshold(ctx, s.DB(), "iface_down_count"); ok {
			for _, r := range currRows {
				p, hasPrev := prevBy[r.IfIndex]
				if !hasPrev {
					continue
				}
				prevUp := snmpifparse.OperStatusLabel(p.OperStatus) == "up"
				currUp := snmpifparse.OperStatusLabel(r.OperStatus) == "up"
				key := fmt.Sprintf("ifdown:%d", r.IfIndex)
				if prevUp && !currUp {
					sev := evalThresholdSeverity(1, th)
					if sev != "ok" {
						name := strings.TrimSpace(r.DisplayName)
						if name == "" {
							name = fmt.Sprintf("if%d", r.IfIndex)
						}
						msg := fmt.Sprintf("%s (%s): interface %s mudou de UP para DOWN.", strings.TrimSpace(devDesc), host, name)
						meta := alertnotify.WithStatusTransition(map[string]any{
							"source":       "interface_snmp_refresh",
							"if_index":     r.IfIndex,
							"display_name": name,
							"key":          key,
						}, "interface_up", "interface_down", nil)
						created, aid, err := openOrUpdateAlertWithMeta(ctx, s.DB(), id, sev, "interface_down_transition", msg, host, devDesc, meta)
						if err == nil && created && aid != uuid.Nil {
							alertnotify.SendMonitoringTelegramAndPatchMeta(ctx, s.DB(), &s.Log, aid, strings.ToUpper(sev), "Interface DOWN (mudança de estado)", msg)
						}
					}
				}
				if currUp {
					closeAlertByMetaKey(ctx, s.DB(), &s.Log, id, "interface_down_transition", key)
				}
			}
		}
	}
	ifaceCount := 0
	if tab, ok := payload["interface_table"].([]map[string]any); ok {
		ifaceCount = len(tab)
	}
	s.appendAuditLog(r.Context(), "device", id.String(), "refresh_interfaces", actorFromRequest(r), nil, map[string]any{
		"ok": ok, "timeout_ms": ifRefreshTO.Milliseconds(), "snmp_rows": len(merged), "interface_count": ifaceCount,
		"walk_truncated": walkRes.Truncated,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":        id,
		"collected_at":     collectedAt.UTC().Format(time.RFC3339),
		"ok":               ok,
		"walk_note":        note,
		"walk_truncated":   walkRes.Truncated,
		"interface_count":  ifaceCount,
		"snmp_rows":        len(merged),
		"interfaces":       json.RawMessage(b),
		"interface_table":  payload["interface_table"],
		"optical_sensors":  payload["optical_sensors"],
	})
}

func (s *Server) realtimeDeviceInterfaces(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var ip *string
	var comm *string
	var devDesc, devCat, devBrand, devModel string
	err = s.DB().QueryRow(r.Context(), `
		SELECT host(ip)::text, snmp_community,
			coalesce(description,''), coalesce(lower(trim(category)),''), coalesce(lower(trim(brand)),''), coalesce(lower(trim(model)), '')
		FROM devices WHERE id=$1
	`, id).Scan(&ip, &comm, &devDesc, &devCat, &devBrand, &devModel)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ip == nil || strings.TrimSpace(*ip) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "IP obrigatório", nil)
		return
	}
	community := ""
	if comm != nil {
		community = strings.TrimSpace(*comm)
	}
	if community == "" {
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&comm)
		if comm != nil {
			community = strings.TrimSpace(*comm)
		}
	}
	if community == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community SNMP obrigatória", nil)
		return
	}
	if !isLikelyMikrotikDevice(devCat, devBrand, devModel, devDesc) {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "monitoramento em tempo real disponível apenas para MikroTik", nil)
		return
	}
	latestRaw, latestAt, _, _, err := loadLatestTwoInterfaceSnapshots(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(latestRaw) == 0 || latestAt == nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "execute refresh de interfaces antes de iniciar o monitoramento", nil)
		return
	}
	payload := buildInterfaceMonitorPayload(latestRaw, latestAt, nil, nil)
	tab, _ := payload["interface_table"].([]map[string]any)
	if len(tab) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id":       id,
			"collected_at":    time.Now().UTC().Format(time.RFC3339),
			"interface_table": []any{},
		})
		return
	}
	unlockSNMP := snmpdevicelock.Acquire(id)
	defer unlockSNMP()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	host := strings.TrimSpace(*ip)
	rates, dtSec := instantTrafficRatesByOID(ctx, host, community, 2*time.Second)
	walkMk, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: snmpmikrotik.DefaultOpticalWalkRoot,
		Version: "2c", Timeout: 14 * time.Second, Retries: 0, MaxRows: snmpMkOpticalMaxRows,
	})
	walkMkIf, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: snmpmikrotik.DefaultInterfaceStatsNameWalkRoot,
		Version: "2c", Timeout: 12 * time.Second, Retries: 0, MaxRows: snmpMkIfStatsMaxRows,
	})
	parsedVars := append([]probing.SNMPVar{}, walkJSONToSNMPVars(latestRaw)...)
	parsedVars = append(parsedVars, walkMk...)
	parsedVars = append(parsedVars, walkMkIf...)
	ifRows := snmpifparse.BuildIfTable(parsedVars)
	optByIf := snmpmikrotik.OpticalPowerByIfIndex(ifRows, parsedVars)
	updates := make([]map[string]any, 0, len(tab))
	for _, row := range tab {
		ifIdx, ok := row["if_index"].(int)
		if !ok || ifIdx <= 0 {
			continue
		}
		upd := map[string]any{
			"if_index": ifIdx,
		}
		if rr, ok := rates[ifIdx]; ok {
			upd["in_bps"] = rr.InBps
			upd["out_bps"] = rr.OutBps
		}
		if op, ok := optByIf[ifIdx]; ok {
			if op.TxDBm != nil {
				upd["tx_dbm"] = *op.TxDBm
			}
			if op.RxDBm != nil {
				upd["rx_dbm"] = *op.RxDBm
			}
		}
		updates = append(updates, upd)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":                id,
		"collected_at":             time.Now().UTC().Format(time.RFC3339),
		"traffic_interval_seconds": dtSec,
		"updates":                  updates,
	})
}

func (s *Server) interfacesHistory(w http.ResponseWriter, r *http.Request) {
	did := r.URL.Query().Get("device_id")
	if did == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "device_id obrigatório", nil)
		return
	}
	id, err := uuid.Parse(did)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	q := `SELECT id, collected_at, interfaces::text FROM interface_snapshots WHERE device_id=$1`
	args := []any{id}
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
		var pk int64
		var ts time.Time
		var j []byte
		if err := rows.Scan(&pk, &ts, &j); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{"id": pk, "collected_at": ts, "interfaces": json.RawMessage(j)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}
