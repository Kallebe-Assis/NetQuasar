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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

// huaweiIfExt{In,Out}BitRateOID — HUAWEI-IF-EXT-MIB::hwIFExtInputBitRate/hwIFExtOutputBitRate
// (colunas 39/40 de hwIFExtTable), coletadas por internal/monitorworker quando o equipamento
// é Huawei (ver collectHuaweiIfExtRates). Taxa instantânea em bps, já calculada no
// equipamento — preferida sobre o delta de ifHCInOctets/ifHCOutOctets quando disponível.
const (
	huaweiIfExtInBitRateOID  = "1.3.6.1.4.1.2011.5.25.41.1.1.1.1.39"
	huaweiIfExtOutBitRateOID = "1.3.6.1.4.1.2011.5.25.41.1.1.1.1.40"
)

func findHuaweiIfExtRate(vars []probing.SNMPVar, ifIndex int) (inBps, outBps float64, ok bool) {
	inSuffix := "." + strconv.Itoa(ifIndex)
	var foundIn, foundOut bool
	for _, v := range vars {
		oid := probing.NormalizeSNMPOID(v.OID)
		if !foundIn && oid == huaweiIfExtInBitRateOID+inSuffix {
			if n, err := strconv.ParseFloat(strings.TrimSpace(v.Value), 64); err == nil {
				inBps = n
				foundIn = true
			}
		} else if !foundOut && oid == huaweiIfExtOutBitRateOID+inSuffix {
			if n, err := strconv.ParseFloat(strings.TrimSpace(v.Value), 64); err == nil {
				outBps = n
				foundOut = true
			}
		}
		if foundIn && foundOut {
			break
		}
	}
	return inBps, outBps, foundIn && foundOut
}

// handlers_bng_uplinks.go — monitoramento de tráfego dos uplinks de operadora (ex.: K2, FORTE)
// num equipamento BNG. Rotula interfaces específicas (bng_uplink_interfaces) e calcula
// tráfego (bps) a partir do histórico já coletado em interface_snapshots (IF-MIB padrão —
// ifHCInOctets/ifHCOutOctets, confirmado no MIB reference do Huawei NE8000 M8) — sem
// introduzir nenhuma coleta SNMP nova: reaproveita snmpifparse.BuildIfTable, o mesmo parser
// já usado pela aba "Interfaces" da tela de equipamento. A lista de interfaces candidatas
// (para o utilizador escolher qual é K2/FORTE) já existe em GET /api/v1/interfaces/devices/{id}.

type bngUplinkInterface struct {
	ID               string `json:"id"`
	DeviceID         string `json:"device_id"`
	CarrierLabel     string `json:"carrier_label"`
	InterfaceLabel   string `json:"interface_label"`
	IfDescr          string `json:"if_descr"`
	IfName           string `json:"if_name"`
	IfIndexHint      *int   `json:"if_index_hint,omitempty"`
	IsPrimaryTraffic bool   `json:"is_primary_traffic"`
	SortOrder        int    `json:"sort_order"`
}

func loadBngUplinks(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) ([]bngUplinkInterface, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, device_id, carrier_label, interface_label, if_descr, if_name, if_index_hint,
			is_primary_traffic, sort_order
		FROM bng_uplink_interfaces
		WHERE device_id = $1
		ORDER BY sort_order, carrier_label, interface_label
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]bngUplinkInterface, 0, 4)
	for rows.Next() {
		var u bngUplinkInterface
		var id, devID uuid.UUID
		if err := rows.Scan(&id, &devID, &u.CarrierLabel, &u.InterfaceLabel, &u.IfDescr, &u.IfName,
			&u.IfIndexHint, &u.IsPrimaryTraffic, &u.SortOrder); err != nil {
			return nil, err
		}
		u.ID = id.String()
		u.DeviceID = devID.String()
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Server) listBngUplinks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	if _, _, err := s.resolveBngDevice(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	uplinks, err := loadBngUplinks(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "uplinks": uplinks})
}

type bngUplinkUpsertBody struct {
	CarrierLabel     string `json:"carrier_label"`
	InterfaceLabel   string `json:"interface_label"`
	IfDescr          string `json:"if_descr"`
	IfName           string `json:"if_name"`
	IfIndexHint      *int   `json:"if_index_hint"`
	IsPrimaryTraffic *bool  `json:"is_primary_traffic"`
	SortOrder        int    `json:"sort_order"`
}

func (s *Server) createBngUplink(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	if _, _, err := s.resolveBngDevice(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	var body bngUplinkUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.CarrierLabel = strings.TrimSpace(body.CarrierLabel)
	body.InterfaceLabel = strings.TrimSpace(body.InterfaceLabel)
	body.IfDescr = strings.TrimSpace(body.IfDescr)
	body.IfName = strings.TrimSpace(body.IfName)
	if body.CarrierLabel == "" || body.InterfaceLabel == "" || (body.IfDescr == "" && body.IfName == "") {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "carrier_label, interface_label e if_descr/if_name são obrigatórios", nil)
		return
	}
	isPrimary := true
	if body.IsPrimaryTraffic != nil {
		isPrimary = *body.IsPrimaryTraffic
	}
	var newID uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO bng_uplink_interfaces
			(device_id, carrier_label, interface_label, if_descr, if_name, if_index_hint, is_primary_traffic, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`, id, body.CarrierLabel, body.InterfaceLabel, body.IfDescr, body.IfName, body.IfIndexHint, isPrimary, body.SortOrder).Scan(&newID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID})
}

func (s *Server) updateBngUplink(w http.ResponseWriter, r *http.Request) {
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
	var body bngUplinkUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	isPrimary := true
	if body.IsPrimaryTraffic != nil {
		isPrimary = *body.IsPrimaryTraffic
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE bng_uplink_interfaces SET
			carrier_label=$1, interface_label=$2, if_descr=$3, if_name=$4, if_index_hint=$5,
			is_primary_traffic=$6, sort_order=$7, updated_at=now()
		WHERE id=$8 AND device_id=$9
	`, strings.TrimSpace(body.CarrierLabel), strings.TrimSpace(body.InterfaceLabel), strings.TrimSpace(body.IfDescr),
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

func (s *Server) deleteBngUplink(w http.ResponseWriter, r *http.Request) {
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
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM bng_uplink_interfaces WHERE id=$1 AND device_id=$2`, uplinkID, deviceID)
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

// --- histórico de tráfego -------------------------------------------------------------------

func findIfRowForUplink(rows []snmpifparse.IfRow, u bngUplinkInterface) (snmpifparse.IfRow, bool) {
	if u.IfName != "" {
		for _, r := range rows {
			if strings.EqualFold(strings.TrimSpace(r.IfName), u.IfName) {
				return r, true
			}
		}
	}
	if u.IfDescr != "" {
		for _, r := range rows {
			if strings.EqualFold(strings.TrimSpace(r.Descr), u.IfDescr) ||
				strings.EqualFold(strings.TrimSpace(r.DisplayName), u.IfDescr) {
				return r, true
			}
		}
	}
	if u.IfIndexHint != nil {
		for _, r := range rows {
			if r.IfIndex == *u.IfIndexHint {
				return r, true
			}
		}
	}
	return snmpifparse.IfRow{}, false
}

type uplinkPoint struct {
	T      string  `json:"t"`
	InBps  float64 `json:"in_bps"`
	OutBps float64 `json:"out_bps"`
}

func (s *Server) bngUplinksHistory(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	if _, _, err := s.resolveBngDevice(r.Context(), deviceID); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	uplinks, err := loadBngUplinks(r.Context(), s.DB(), deviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(uplinks) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": deviceID, "uplinks": []any{}, "carrier_totals": map[string]any{}})
		return
	}

	q := r.URL.Query()
	days := 1
	if d := strings.TrimSpace(q.Get("days")); d != "" {
		if n, perr := strconv.Atoi(d); perr == nil {
			days = n
		}
	}
	switch days {
	case 1, 3, 7, 30:
	default:
		days = 1
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	until := now
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		if t, terr := time.Parse(time.RFC3339, fromStr); terr == nil {
			since = t.UTC()
		}
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		if t, terr := time.Parse(time.RFC3339, toStr); terr == nil {
			until = t.UTC()
		}
	}
	if !until.After(since) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "período inválido", nil)
		return
	}
	if since.Before(now.Add(-31 * 24 * time.Hour)) {
		since = now.Add(-31 * 24 * time.Hour) // teto de sanidade: interface_snapshots é pesado
	}

	// Um snapshot representativo por bucket (não todo snapshot bruto — cada um pode ter
	// milhares de OIDs) — mesma escolha de granularidade dinâmica do histórico de ONUs.
	bucket, _ := pickOLTHistoryBucket(until.Sub(since))

	rows, err := s.DB().Query(r.Context(), `
		SELECT DISTINCT ON (date_trunc($1, collected_at AT TIME ZONE 'UTC'))
			collected_at, interfaces::text
		FROM interface_snapshots
		WHERE device_id = $2 AND collected_at >= $3 AND collected_at < $4
		ORDER BY date_trunc($1, collected_at AT TIME ZONE 'UTC'), collected_at DESC
	`, bucket, deviceID, since, until)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	type snap struct {
		at  time.Time
		raw []byte
	}
	var snaps []snap
	for rows.Next() {
		var sRow snap
		if err := rows.Scan(&sRow.at, &sRow.raw); err != nil {
			rows.Close()
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		snaps = append(snaps, sRow)
	}
	rows.Close()

	type uplinkResult struct {
		bngUplinkInterface
		Points  []uplinkPoint `json:"points"`
		Current *uplinkPoint  `json:"current,omitempty"`
	}
	results := make([]uplinkResult, len(uplinks))
	for i, u := range uplinks {
		results[i] = uplinkResult{bngUplinkInterface: u, Points: []uplinkPoint{}}
	}

	// vars/rows por snapshot, calculados uma vez e reaproveitados: leitura directa (Huawei
	// hwIFExtInputBitRate/hwIFExtOutputBitRate, ver interface_snapshot_worker.go) quando
	// disponível para o ifIndex — não depende de par anterior, é a taxa já suavizada pelo
	// próprio equipamento. Sem isso (outro vendor, ou snapshot anterior à extensão Huawei),
	// cai para o delta clássico entre ifHCInOctets/ifHCOutOctets consecutivos.
	type parsedSnap struct {
		vars []probing.SNMPVar
		rows []snmpifparse.IfRow
	}
	parsed := make([]parsedSnap, len(snaps))
	for i, sn := range snaps {
		v := walkJSONToSNMPVars(sn.raw)
		parsed[i] = parsedSnap{vars: v, rows: snmpifparse.BuildIfTable(v)}
	}

	for i, cur := range snaps {
		if len(parsed[i].rows) == 0 {
			continue
		}
		for idx, u := range uplinks {
			cr, cOK := findIfRowForUplink(parsed[i].rows, u)
			if !cOK {
				continue
			}
			if inRate, outRate, ok := findHuaweiIfExtRate(parsed[i].vars, cr.IfIndex); ok {
				pt := uplinkPoint{T: cur.at.UTC().Format(time.RFC3339), InBps: round2(inRate), OutBps: round2(outRate)}
				results[idx].Points = append(results[idx].Points, pt)
				last := pt
				results[idx].Current = &last
				continue
			}
			if i == 0 {
				continue
			}
			prev := snaps[i-1]
			dt := cur.at.Sub(prev.at).Seconds()
			if dt <= 0 || len(parsed[i-1].rows) == 0 {
				continue
			}
			pr, pOK := findIfRowForUplink(parsed[i-1].rows, u)
			if !pOK || cr.InOctets < pr.InOctets || cr.OutOctets < pr.OutOctets {
				continue
			}
			pt := uplinkPoint{
				T:      cur.at.UTC().Format(time.RFC3339),
				InBps:  round2(float64(cr.InOctets-pr.InOctets) * 8 / dt),
				OutBps: round2(float64(cr.OutOctets-pr.OutOctets) * 8 / dt),
			}
			results[idx].Points = append(results[idx].Points, pt)
			last := pt
			results[idx].Current = &last
		}
	}

	carrierTotals := map[string]map[string]float64{}
	for _, res := range results {
		if !res.IsPrimaryTraffic || res.Current == nil {
			continue
		}
		t := carrierTotals[res.CarrierLabel]
		if t == nil {
			t = map[string]float64{"in_bps": 0, "out_bps": 0}
			carrierTotals[res.CarrierLabel] = t
		}
		t["in_bps"] += res.Current.InBps
		t["out_bps"] += res.Current.OutBps
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":      deviceID,
		"since":          since.Format(time.RFC3339),
		"until":          until.Format(time.RFC3339),
		"bucket":         bucket,
		"uplinks":        results,
		"carrier_totals": carrierTotals,
	})
}
