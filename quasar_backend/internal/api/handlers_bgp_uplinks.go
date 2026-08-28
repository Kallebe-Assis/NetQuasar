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
// operadora (bgp_uplink_interfaces), mirror directo de handlers_bng_uplinks.go (CRUD only —
// sem endpoint de histórico próprio: /api/v1/bgp/devices/{id}/history já devolve
// interfaces[].in_bit_rate/out_bit_rate por amostra, calculado pelo próprio Huawei
// (hwIFExtInputBitRate/hwIFExtOutputBitRate), então o frontend filtra por essas etiquetas
// directamente em cima desse endpoint já existente).

type bgpUplinkInterface struct {
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

func loadBgpUplinks(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) ([]bgpUplinkInterface, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, device_id, carrier_label, interface_label, if_descr, if_name, if_index_hint,
			is_primary_traffic, sort_order
		FROM bgp_uplink_interfaces
		WHERE device_id = $1
		ORDER BY sort_order, carrier_label, interface_label
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]bgpUplinkInterface, 0, 4)
	for rows.Next() {
		var u bgpUplinkInterface
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
	CarrierLabel     string `json:"carrier_label"`
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
		INSERT INTO bgp_uplink_interfaces
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
	isPrimary := true
	if body.IsPrimaryTraffic != nil {
		isPrimary = *body.IsPrimaryTraffic
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE bgp_uplink_interfaces SET
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

// --- limite de banda por operadora -----------------------------------------------------------
// bgp_uplink_carrier_limits (124_bgp_uplink_carrier_limits.sql) — o limite pertence à operadora
// como um todo (carrier_label), não a uma interface específica de bgp_uplink_interfaces, já que
// uma operadora pode ter várias interfaces somadas. Usado pelo frontend para definir o teto do
// eixo Y do gráfico de tráfego por operadora.

type bgpCarrierLimit struct {
	CarrierLabel       string   `json:"carrier_label"`
	BandwidthLimitMbps *float64 `json:"bandwidth_limit_mbps,omitempty"`
}

func (s *Server) listBgpCarrierLimits(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT carrier_label, bandwidth_limit_mbps FROM bgp_uplink_carrier_limits WHERE device_id=$1
	`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := make([]bgpCarrierLimit, 0, 4)
	for rows.Next() {
		var l bgpCarrierLimit
		if err := rows.Scan(&l.CarrierLabel, &l.BandwidthLimitMbps); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		out = append(out, l)
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "limits": out})
}

type bgpCarrierLimitBody struct {
	BandwidthLimitMbps *float64 `json:"bandwidth_limit_mbps"`
}

func (s *Server) upsertBgpCarrierLimit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	label := strings.TrimSpace(chi.URLParam(r, "label"))
	if label == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "operadora inválida", nil)
		return
	}
	var body bgpCarrierLimitBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `
		INSERT INTO bgp_uplink_carrier_limits (device_id, carrier_label, bandwidth_limit_mbps, updated_at)
		VALUES ($1,$2,$3, now())
		ON CONFLICT (device_id, carrier_label) DO UPDATE SET bandwidth_limit_mbps=$3, updated_at=now()
	`, id, label, body.BandwidthLimitMbps)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- tráfego por operadora (histórico) --------------------------------------------------------
// bgpCarrierTrafficHistory — mirror directo de bngUplinksHistory (mesmo ficheiro, acima), mas a
// partir de telemetry_samples/bgpcollect.BuildReportFromStoredMetrics (já pivota
// hw_if_in_bit_rate/hw_if_out_bit_rate por interface, taxa instantânea já calculada pelo próprio
// Huawei) em vez de interface_snapshots/snmpifparse. Soma por operadora (carrier_label) as
// interfaces is_primary_traffic=true de cada bucket, e devolve estatísticas (min/max/média)
// calculadas sobre esses mesmos pontos "bucketed" — não uma segunda varredura das amostras
// brutas (troca de performance confirmada com o utilizador, mesma já feita por
// getOLTReportsHistory/bngUplinksHistory para períodos longos).

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

type bgpCarrierTraffic struct {
	CarrierLabel       string            `json:"carrier_label"`
	BandwidthLimitMbps *float64          `json:"bandwidth_limit_mbps,omitempty"`
	Points             []bgpTrafficPoint `json:"points"`
	Stats              bgpCarrierStats   `json:"stats"`
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
		writeJSON(w, http.StatusOK, map[string]any{"device_id": deviceID, "carriers": []any{}})
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
	limRows, err := s.DB().Query(r.Context(), `
		SELECT carrier_label, bandwidth_limit_mbps FROM bgp_uplink_carrier_limits WHERE device_id=$1
	`, deviceID)
	if err == nil {
		for limRows.Next() {
			var label string
			var v *float64
			if scanErr := limRows.Scan(&label, &v); scanErr == nil {
				limits[label] = v
			}
		}
		limRows.Close()
	}

	// Lista estável de operadoras (só as que têm ao menos uma interface is_primary_traffic=true)
	// — cada uma ganha exactamente len(snaps) pontos, alinhados por timestamp/índice, para o
	// frontend poder somar várias operadoras ponto-a-ponto no modo "Somado" sem re-alinhar séries.
	seen := map[string]bool{}
	var carrierLabels []string
	for _, u := range uplinks {
		if !u.IsPrimaryTraffic || seen[u.CarrierLabel] {
			continue
		}
		seen[u.CarrierLabel] = true
		carrierLabels = append(carrierLabels, u.CarrierLabel)
	}
	sort.Strings(carrierLabels)

	pointsByCarrier := make(map[string][]bgpTrafficPoint, len(carrierLabels))
	for _, label := range carrierLabels {
		pointsByCarrier[label] = make([]bgpTrafficPoint, 0, len(snaps))
	}
	for _, sn := range snaps {
		sums := map[string][2]float64{}
		for _, u := range uplinks {
			if !u.IsPrimaryTraffic {
				continue
			}
			ifc, ok := findBgpIfRowForUplink(sn.rep.Interfaces, u)
			if !ok {
				continue
			}
			inBps, _ := strconv.ParseFloat(strings.TrimSpace(ifc.InBitRate), 64)
			outBps, _ := strconv.ParseFloat(strings.TrimSpace(ifc.OutBitRate), 64)
			cur := sums[u.CarrierLabel]
			cur[0] += inBps
			cur[1] += outBps
			sums[u.CarrierLabel] = cur
		}
		ts := sn.at.UTC().Format(time.RFC3339)
		for _, label := range carrierLabels {
			v := sums[label]
			pointsByCarrier[label] = append(pointsByCarrier[label], bgpTrafficPoint{T: ts, InBps: round2(v[0]), OutBps: round2(v[1])})
		}
	}

	carriers := make([]bgpCarrierTraffic, 0, len(carrierLabels))
	for _, label := range carrierLabels {
		pts := pointsByCarrier[label]
		carriers = append(carriers, bgpCarrierTraffic{
			CarrierLabel:       label,
			BandwidthLimitMbps: limits[label],
			Points:             pts,
			Stats:              computeBgpCarrierStats(pts),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID,
		"since":     since.Format(time.RFC3339),
		"until":     until.Format(time.RFC3339),
		"bucket":    bucket,
		"carriers":  carriers,
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
