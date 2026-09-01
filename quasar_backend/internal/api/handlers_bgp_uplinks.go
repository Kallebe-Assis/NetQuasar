package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netquasar/netquasar/quasar_backend/internal/bgpcollect"
)

// handlers_bgp_uplinks.go — rotula interfaces de um equipamento BGP como pertencentes a uma
// operadora (bgp_uplink_interfaces.carrier_id → bgp_carriers, ver handlers_bgp_carriers.go),
// mirror directo de handlers_bng_uplinks.go (CRUD only — sem endpoint de histórico próprio:
// /api/v1/bgp/devices/{id}/history já devolve interfaces[].in_bit_rate/out_bit_rate por amostra,
// calculado pelo próprio Huawei — hwIFExtInputBitRate/hwIFExtOutputBitRate).

type bgpUplinkInterface struct {
	ID               string `json:"id"`
	DeviceID         string `json:"device_id"`
	CarrierID        string `json:"carrier_id"`
	CarrierLabel     string `json:"carrier_label"`
	InterfaceLabel   string `json:"interface_label"`
	IfDescr          string `json:"if_descr"`
	IfName           string `json:"if_name"`
	IfIndexHint      *int   `json:"if_index_hint,omitempty"`
	IsPrimaryTraffic bool   `json:"is_primary_traffic"`
	SortOrder        int    `json:"sort_order"`
}

func loadBgpUplinks(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) ([]bgpUplinkInterface, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.id, u.device_id, u.carrier_id, c.name, u.interface_label, u.if_descr, u.if_name,
			u.if_index_hint, u.is_primary_traffic, u.sort_order
		FROM bgp_uplink_interfaces u
		JOIN bgp_carriers c ON c.id = u.carrier_id
		WHERE u.device_id = $1
		ORDER BY u.sort_order, c.name, u.interface_label
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]bgpUplinkInterface, 0, 4)
	for rows.Next() {
		var u bgpUplinkInterface
		var id, devID, carrierID uuid.UUID
		if err := rows.Scan(&id, &devID, &carrierID, &u.CarrierLabel, &u.InterfaceLabel, &u.IfDescr, &u.IfName,
			&u.IfIndexHint, &u.IsPrimaryTraffic, &u.SortOrder); err != nil {
			return nil, err
		}
		u.ID = id.String()
		u.DeviceID = devID.String()
		u.CarrierID = carrierID.String()
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Server) listBgpUplinks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	uplinks, err := loadBgpUplinks(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "uplinks": uplinks})
}

type bgpUplinkUpsertBody struct {
	CarrierID        string `json:"carrier_id"`
	InterfaceLabel   string `json:"interface_label"`
	IfDescr          string `json:"if_descr"`
	IfName           string `json:"if_name"`
	IfIndexHint      *int   `json:"if_index_hint"`
	IsPrimaryTraffic *bool  `json:"is_primary_traffic"`
	SortOrder        int    `json:"sort_order"`
}

func (s *Server) createBgpUplink(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var body bgpUplinkUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	carrierID, cerr := uuid.Parse(strings.TrimSpace(body.CarrierID))
	body.InterfaceLabel = strings.TrimSpace(body.InterfaceLabel)
	body.IfDescr = strings.TrimSpace(body.IfDescr)
	body.IfName = strings.TrimSpace(body.IfName)
	if cerr != nil || body.InterfaceLabel == "" || (body.IfDescr == "" && body.IfName == "") {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "carrier_id, interface_label e if_descr/if_name são obrigatórios", nil)
		return
	}
	isPrimary := true
	if body.IsPrimaryTraffic != nil {
		isPrimary = *body.IsPrimaryTraffic
	}
	var newID uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO bgp_uplink_interfaces
			(device_id, carrier_id, interface_label, if_descr, if_name, if_index_hint, is_primary_traffic, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`, id, carrierID, body.InterfaceLabel, body.IfDescr, body.IfName, body.IfIndexHint, isPrimary, body.SortOrder).Scan(&newID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID})
}

func (s *Server) updateBgpUplink(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	uplinkID, err := uuid.Parse(chi.URLParam(r, "uplinkId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "uplink id inválido", nil)
		return
	}
	var body bgpUplinkUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	carrierID, cerr := uuid.Parse(strings.TrimSpace(body.CarrierID))
	if cerr != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "carrier_id inválido", nil)
		return
	}
	isPrimary := true
	if body.IsPrimaryTraffic != nil {
		isPrimary = *body.IsPrimaryTraffic
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE bgp_uplink_interfaces SET
			carrier_id=$1, interface_label=$2, if_descr=$3, if_name=$4, if_index_hint=$5,
			is_primary_traffic=$6, sort_order=$7, updated_at=now()
		WHERE id=$8 AND device_id=$9
	`, carrierID, strings.TrimSpace(body.InterfaceLabel), strings.TrimSpace(body.IfDescr),
		strings.TrimSpace(body.IfName), body.IfIndexHint, isPrimary, body.SortOrder, uplinkID, deviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "uplink não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- tráfego por operadora (histórico) --------------------------------------------------------
// bgpCarrierTrafficHistory — mirror directo de bngUplinksHistory (mesmo ficheiro, acima), mas a
// partir de telemetry_samples/bgpcollect.BuildReportFromStoredMetrics (já pivota
// hw_if_in_bit_rate/hw_if_out_bit_rate por interface, taxa instantânea já calculada pelo próprio
// Huawei) em vez de interface_snapshots/snmpifparse. Devolve: (1) "total" — soma de TODAS as
// interfaces is_primary_traffic=true de TODAS as operadoras, ponto a ponto (gráfico principal);
// (2) "carriers[]" — 1 entrada por operadora, com os pontos já somados das suas interfaces
// (gráfico "Somado" dela) E a repartição por interface individual em "interfaces[]" (gráfico
// "Separado" dela, só relevante quando a operadora tem 2+ interfaces). Estatísticas (min/max/
// média) calculadas sobre esses mesmos pontos "bucketed" — não uma segunda varredura das
// amostras brutas (troca de performance já feita por getOLTReportsHistory/bngUplinksHistory).

type bgpTrafficPoint struct {
	T      string  `json:"t"`
	InBps  float64 `json:"in_bps"`
	OutBps float64 `json:"out_bps"`
}

type bgpCarrierStats struct {
	InMin  float64 `json:"in_min"`
	InMax  float64 `json:"in_max"`
	InAvg  float64 `json:"in_avg"`
	OutMin float64 `json:"out_min"`
	OutMax float64 `json:"out_max"`
	OutAvg float64 `json:"out_avg"`
}

type bgpInterfaceTraffic struct {
	InterfaceLabel string            `json:"interface_label"`
	Points         []bgpTrafficPoint `json:"points"`
	Stats          bgpCarrierStats   `json:"stats"`
}

type bgpCarrierTraffic struct {
	CarrierID          string                `json:"carrier_id"`
	CarrierLabel       string                `json:"carrier_label"`
	BandwidthLimitMbps *float64              `json:"bandwidth_limit_mbps,omitempty"`
	Points             []bgpTrafficPoint     `json:"points"`
	Stats              bgpCarrierStats       `json:"stats"`
	Interfaces         []bgpInterfaceTraffic `json:"interfaces"`
}

// findBgpIfRowForUplink localiza a interface de um uplink dentro de um Report já pivotado —
// mesma lógica de findIfRowForUplink (bngUplinksHistory), adaptada de snmpifparse.IfRow para
// bgpcollect.InterfaceReport (campos Descr/Alias/IfIndex).
func findBgpIfRowForUplink(ifaces []bgpcollect.InterfaceReport, u bgpUplinkInterface) (bgpcollect.InterfaceReport, bool) {
	if u.IfName != "" {
		for _, ifc := range ifaces {
			if strings.EqualFold(strings.TrimSpace(ifc.Alias), u.IfName) {
				return ifc, true
			}
		}
	}
	if u.IfDescr != "" {
		for _, ifc := range ifaces {
			if strings.EqualFold(strings.TrimSpace(ifc.Descr), u.IfDescr) {
				return ifc, true
			}
		}
	}
	if u.IfIndexHint != nil {
		want := strconv.Itoa(*u.IfIndexHint)
		for _, ifc := range ifaces {
			if ifc.IfIndex == want {
				return ifc, true
			}
		}
	}
	return bgpcollect.InterfaceReport{}, false
}

func computeBgpCarrierStats(points []bgpTrafficPoint) bgpCarrierStats {
	if len(points) == 0 {
		return bgpCarrierStats{}
	}
	inMin, outMin := math.Inf(1), math.Inf(1)
	inMax, outMax := math.Inf(-1), math.Inf(-1)
	var inSum, outSum float64
	for _, p := range points {
		if p.InBps < inMin {
			inMin = p.InBps
		}
		if p.InBps > inMax {
			inMax = p.InBps
		}
		if p.OutBps < outMin {
			outMin = p.OutBps
		}
		if p.OutBps > outMax {
			outMax = p.OutBps
		}
		inSum += p.InBps
		outSum += p.OutBps
	}
	n := float64(len(points))
	return bgpCarrierStats{
		InMin: round2(inMin), InMax: round2(inMax), InAvg: round2(inSum / n),
		OutMin: round2(outMin), OutMax: round2(outMax), OutAvg: round2(outSum / n),
	}
}

func (s *Server) bgpCarrierTrafficHistory(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	uplinks, err := loadBgpUplinks(r.Context(), s.DB(), deviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(uplinks) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": deviceID, "total": nil, "carriers": []any{}})
		return
	}

	q := r.URL.Query()
	days := 7
	if d := strings.TrimSpace(q.Get("days")); d != "" {
		if n, perr := strconv.Atoi(d); perr == nil {
			days = n
		}
	}
	switch days {
	case 1, 3, 7, 15, 30, 60, 90, 150, 300:
	default:
		days = 7
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	until := now
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		t, terr := time.Parse(time.RFC3339, fromStr)
		if terr != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Parâmetro \"from\" inválido — use formato ISO 8601 (RFC3339).", nil)
			return
		}
		since = t.UTC()
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		t, terr := time.Parse(time.RFC3339, toStr)
		if terr != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Parâmetro \"to\" inválido — use formato ISO 8601 (RFC3339).", nil)
			return
		}
		until = t.UTC()
	}
	if !until.After(since) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "Período inválido: \"to\" deve ser depois de \"from\".", nil)
		return
	}
	if since.Before(now.Add(-366 * 24 * time.Hour)) {
		since = now.Add(-366 * 24 * time.Hour) // teto de sanidade
	}

	bucket, _ := pickOLTHistoryBucket(until.Sub(since))

	rows, err := s.DB().Query(r.Context(), `
		SELECT DISTINCT ON (date_trunc($1, collected_at AT TIME ZONE 'UTC'))
			collected_at, metrics::text
		FROM telemetry_samples
		WHERE device_id = $2 AND metrics ? 'bgp_collection' AND collected_at >= $3 AND collected_at < $4
		ORDER BY date_trunc($1, collected_at AT TIME ZONE 'UTC'), collected_at DESC
	`, bucket, deviceID, since, until)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	type snap struct {
		at  time.Time
		rep bgpcollect.Report
	}
	var snaps []snap
	for rows.Next() {
		var at time.Time
		var raw []byte
		if err := rows.Scan(&at, &raw); err != nil {
			rows.Close()
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		snaps = append(snaps, snap{at: at, rep: bgpcollect.BuildReportFromStoredMetrics(raw)})
	}
	rows.Close()
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].at.Before(snaps[j].at) })

	limits := map[string]*float64{}
	limRows, err := s.DB().Query(r.Context(), `SELECT id, bandwidth_limit_mbps FROM bgp_carriers`)
	if err == nil {
		for limRows.Next() {
			var id uuid.UUID
			var v *float64
			if scanErr := limRows.Scan(&id, &v); scanErr == nil {
				limits[id.String()] = v
			}
		}
		limRows.Close()
	}

	// Lista estável de operadoras (só as que têm ao menos uma interface is_primary_traffic=true)
	// e, dentro de cada uma, a lista estável das suas interfaces primárias — cada série ganha
	// exactamente len(snaps) pontos, alinhados por índice, para o frontend poder somar sem
	// re-alinhar por timestamp.
	type carrierMeta struct {
		label      string
		interfaces []bgpUplinkInterface
	}
	carriersByID := map[string]*carrierMeta{}
	var carrierIDs []string
	for _, u := range uplinks {
		if !u.IsPrimaryTraffic {
			continue
		}
		cm, ok := carriersByID[u.CarrierID]
		if !ok {
			cm = &carrierMeta{label: u.CarrierLabel}
			carriersByID[u.CarrierID] = cm
			carrierIDs = append(carrierIDs, u.CarrierID)
		}
		cm.interfaces = append(cm.interfaces, u)
	}
	sort.Slice(carrierIDs, func(i, j int) bool { return carriersByID[carrierIDs[i]].label < carriersByID[carrierIDs[j]].label })

	totalPoints := make([]bgpTrafficPoint, 0, len(snaps))
	carrierPoints := map[string][]bgpTrafficPoint{}
	ifacePoints := map[string]map[string][]bgpTrafficPoint{} // carrierID -> uplinkID -> points
	for _, id := range carrierIDs {
		carrierPoints[id] = make([]bgpTrafficPoint, 0, len(snaps))
		ifacePoints[id] = map[string][]bgpTrafficPoint{}
		for _, u := range carriersByID[id].interfaces {
			ifacePoints[id][u.ID] = make([]bgpTrafficPoint, 0, len(snaps))
		}
	}

	for _, sn := range snaps {
		ts := sn.at.UTC().Format(time.RFC3339)
		var totalIn, totalOut float64
		for _, id := range carrierIDs {
			cm := carriersByID[id]
			var cIn, cOut float64
			for _, u := range cm.interfaces {
				var inBps, outBps float64
				if ifc, ok := findBgpIfRowForUplink(sn.rep.Interfaces, u); ok {
					inBps, _ = strconv.ParseFloat(strings.TrimSpace(ifc.InBitRate), 64)
					outBps, _ = strconv.ParseFloat(strings.TrimSpace(ifc.OutBitRate), 64)
				}
				ifacePoints[id][u.ID] = append(ifacePoints[id][u.ID], bgpTrafficPoint{T: ts, InBps: round2(inBps), OutBps: round2(outBps)})
				cIn += inBps
				cOut += outBps
			}
			carrierPoints[id] = append(carrierPoints[id], bgpTrafficPoint{T: ts, InBps: round2(cIn), OutBps: round2(cOut)})
			totalIn += cIn
			totalOut += cOut
		}
		totalPoints = append(totalPoints, bgpTrafficPoint{T: ts, InBps: round2(totalIn), OutBps: round2(totalOut)})
	}

	carriers := make([]bgpCarrierTraffic, 0, len(carrierIDs))
	for _, id := range carrierIDs {
		cm := carriersByID[id]
		ifs := make([]bgpInterfaceTraffic, 0, len(cm.interfaces))
		for _, u := range cm.interfaces {
			pts := ifacePoints[id][u.ID]
			ifs = append(ifs, bgpInterfaceTraffic{InterfaceLabel: u.InterfaceLabel, Points: pts, Stats: computeBgpCarrierStats(pts)})
		}
		pts := carrierPoints[id]
		carriers = append(carriers, bgpCarrierTraffic{
			CarrierID:          id,
			CarrierLabel:       cm.label,
			BandwidthLimitMbps: limits[id],
			Points:             pts,
			Stats:              computeBgpCarrierStats(pts),
			Interfaces:         ifs,
		})
	}

	// Teto de referência do "total geral" = soma dos limites de todas as operadoras que têm
	// limite configurado (as sem limite não entram na soma, mas continuam a desenhar).
	var totalLimit *float64
	var totalLimitSum float64
	hasTotalLimit := false
	for _, c := range carriers {
		if c.BandwidthLimitMbps != nil && *c.BandwidthLimitMbps > 0 {
			totalLimitSum += *c.BandwidthLimitMbps
			hasTotalLimit = true
		}
	}
	if hasTotalLimit {
		totalLimit = &totalLimitSum
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID,
		"since":     since.Format(time.RFC3339),
		"until":     until.Format(time.RFC3339),
		"bucket":    bucket,
		"total": map[string]any{
			"points":               totalPoints,
			"stats":                computeBgpCarrierStats(totalPoints),
			"bandwidth_limit_mbps": totalLimit,
		},
		"carriers": carriers,
	})
}

func (s *Server) deleteBgpUplink(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	uplinkID, err := uuid.Parse(chi.URLParam(r, "uplinkId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "uplink id inválido", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM bgp_uplink_interfaces WHERE id=$1 AND device_id=$2`, uplinkID, deviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "uplink não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
