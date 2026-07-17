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
	"github.com/netquasar/netquasar/quasar_backend/internal/ifaceoptical"
	"github.com/netquasar/netquasar/quasar_backend/internal/interfacealerts"
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
	// Escaneia snapshots antigos apenas quando solicitado (evita ler 20 JSONBs por pedido).
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("resolve_best")), "1") {
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
	enrichInterfaceTableOpticalFromTelemetry(r.Context(), s.DB(), id, payload)
	if err := enrichInterfaceTableMetadata(r.Context(), s.DB(), id, payload); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out := map[string]any{"device_id": id, "collected_at": useAt.UTC().Format(time.RFC3339), "interfaces": json.RawMessage(useRaw)}
	if useAt != collected {
		out["note"] = "último snapshot estava parcial; exibindo snapshot anterior mais completo"
	}
	// GET = só cache (último interface_snapshots). Taxa instantânea SNMP fica em /refresh ou /realtime.
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
	isSwitch := strings.EqualFold(strings.TrimSpace(devCat), "switch")
	isMikrotik := !isSwitch && isLikelyMikrotikDevice(devCat, devBrand, devModel, devDesc)
	walkRes := collectInterfaceSNMPWalks(ctx, s.DB(), host, c, ifRefreshTO, isMikrotik || isSwitch, isSwitch)
	merged := walkRes.Merged
	note := walkRes.Note
	arr := make([]map[string]any, 0, len(merged)+1)
	for _, v := range merged {
		arr = append(arr, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
	}
	if walkRes.Truncated {
		arr = append(arr, map[string]any{"oid": "__netquasar.walk", "value": "truncated", "type": "meta"})
	}
	// Telnet interfaces/óptica (NX-OS transceiver / RouterOS SFP) — persistido no snapshot.
	if isMikrotik || isSwitch {
		telnetTO := 60 * time.Second
		if ifRefreshTO > 0 && ifRefreshTO < telnetTO {
			telnetTO = ifRefreshTO
		}
		arr = ifaceoptical.EnrichSnapshotArray(ctx, s.DB(), id, host, isSwitch, arr, telnetTO)
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
	if isSwitch {
		enrichInterfaceTableOpticalFromTelemetry(r.Context(), s.DB(), id, payload)
	}
	if err := enrichInterfaceTableMetadata(r.Context(), s.DB(), id, payload); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if isMikrotik || isSwitch {
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
		// Snapshot OLT/PON só via refresh manual (perfil em Definições).
	}
	if s.DB() != nil {
		interfacealerts.EvaluateAfterSnapshot(ctx, s.DB(), &s.Log, interfacealerts.Params{
			DeviceID:   id,
			Host:       host,
			DeviceDesc: devDesc,
			Category:   devCat,
			Brand:      devBrand,
			Model:      devModel,
			Source:     "interface_snmp_refresh",
			PrevJSON:   latestBeforeInsertRaw,
			CurrJSON:   b,
		})
	}
	ifaceCount := 0
	if tab, ok := payload["interface_table"].([]map[string]any); ok {
		ifaceCount = len(tab)
	}
	s.appendAuditLog(r.Context(), "device", id.String(), "refresh_interfaces", s.actorFromRequest(r), nil, map[string]any{
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
	isMikrotik := isLikelyMikrotikDevice(devCat, devBrand, devModel, devDesc)
	isSwitch := strings.EqualFold(strings.TrimSpace(devCat), "switch")
	if !isMikrotik && !isSwitch {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "monitoramento em tempo real disponível para MikroTik e Switch", nil)
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
	if isSwitch {
		enrichInterfaceTableOpticalFromTelemetry(r.Context(), s.DB(), id, payload)
	}
	if err := enrichInterfaceTableMetadata(r.Context(), s.DB(), id, payload); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
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
	var optByIf map[int]snmpmikrotik.OpticalPower
	if isMikrotik {
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
		optByIf = snmpmikrotik.OpticalPowerByIfIndex(ifRows, parsedVars)
	}
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
		// Switch: óptica vem do snapshot/telemetria Telnet — reenvia no realtime para a UI não perder.
		if upd["tx_dbm"] == nil && row["tx_dbm"] != nil {
			upd["tx_dbm"] = row["tx_dbm"]
		}
		if upd["rx_dbm"] == nil && row["rx_dbm"] != nil {
			upd["rx_dbm"] = row["rx_dbm"]
		}
		updates = append(updates, upd)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":                id,
		"collected_at":             time.Now().UTC().Format(time.RFC3339),
		"traffic_interval_seconds": dtSec,
		"updates":                  updates,
	})
	if s.rt != nil {
		s.rt.publish(r.Context(), "realtime.interfaces", map[string]any{
			"device_id":                id.String(),
			"collected_at":             time.Now().UTC().Format(time.RFC3339),
			"traffic_interval_seconds": dtSec,
			"updates":                  updates,
		})
	}
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
