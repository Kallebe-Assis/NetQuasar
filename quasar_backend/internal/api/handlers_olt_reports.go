package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
)

func intValMap(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

// syncCommercialMonthlyFromOLTSnapshots grava totais ONU por localidade na base comercial (mês AAAA-MM).
func (s *Server) syncCommercialMonthlyFromOLTSnapshots(ctx context.Context, yearMonth string) (int, error) {
	yearMonth = strings.TrimSpace(yearMonth)
	if !yearMonthCommercialRe.MatchString(yearMonth) {
		return 0, fmt.Errorf("year_month inválido")
	}
	rows, err := s.DB().Query(ctx, `
		SELECT d.locality_id, os.summary::text, os.pons::text
		FROM devices d
		INNER JOIN olt_snapshots os ON os.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt' AND d.locality_id IS NOT NULL
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	byLoc := map[uuid.UUID]int{}
	for rows.Next() {
		var locID uuid.UUID
		var sumRaw, ponsRaw string
		if err := rows.Scan(&locID, &sumRaw, &ponsRaw); err != nil {
			continue
		}
		c := oltparse.SnapshotComputed([]byte(sumRaw), []byte(ponsRaw))
		byLoc[locID] += intValMap(c, "onu_total_sum")
	}
	if len(byLoc) == 0 {
		return 0, nil
	}
	tx, err := s.DB().Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n := 0
	for locID, count := range byLoc {
		if count <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO commercial_monthly_records (locality_id, year_month, client_count)
			VALUES ($1, $2, $3)
			ON CONFLICT (locality_id, year_month) DO UPDATE SET client_count = EXCLUDED.client_count, updated_at = now()
		`, locID, yearMonth, count); err != nil {
			return 0, err
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

// pickOLTHistoryBucket escolhe a granularidade do bucket a partir do período pedido: janelas
// de até ~1 dia usam minuto (mostra cada coleta real em vez de esconder variação dentro da
// hora), até ~4 dias usam hora, o resto usa dia — evita gerar milhares de pontos em períodos
// longos mantendo boa resolução em períodos curtos.
func pickOLTHistoryBucket(span time.Duration) (bucket string, interval string) {
	switch {
	case span <= 26*time.Hour:
		return "minute", "1 minute"
	case span <= 4*24*time.Hour:
		return "hour", "1 hour"
	default:
		return "day", "1 day"
	}
}

func (s *Server) getOLTReportsHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days := 7
	if d := strings.TrimSpace(q.Get("days")); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			days = n
		}
	}
	switch days {
	case 1, 3, 7, 30:
	default:
		days = 7
	}

	// Período customizado (from/to) tem prioridade sobre o preset `days` — usado pela consulta
	// livre de período específico na aba Relatórios. Sem from/to, mantém o comportamento antigo
	// (preset relativo a "agora").
	now := time.Now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	until := now
	customRange := false
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Parâmetro \"from\" inválido — use formato ISO 8601 (RFC3339).", nil)
			return
		}
		since = t.UTC()
		customRange = true
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Parâmetro \"to\" inválido — use formato ISO 8601 (RFC3339).", nil)
			return
		}
		until = t.UTC()
		customRange = true
	}
	if !until.After(since) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "Período inválido: \"to\" deve ser depois de \"from\".", nil)
		return
	}
	if since.Before(now.Add(-366 * 24 * time.Hour)) {
		since = now.Add(-366 * 24 * time.Hour) // teto de sanidade — evita varrer o histórico inteiro por engano
	}

	var deviceFilter *uuid.UUID
	if idStr := strings.TrimSpace(q.Get("device_id")); idStr != "" && idStr != "all" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "device_id inválido.", nil)
			return
		}
		deviceFilter = &id
	}

	bucket, interval := pickOLTHistoryBucket(until.Sub(since))
	if !customRange && days != 1 {
		// Mantém o comportamento antigo dos presets >1 dia (bucket diário) mesmo que o cálculo
		// de span desse "hour" para 3 dias — só o preset 24h ganha a granularidade fina nova.
		bucket, interval = "day", "1 day"
	}

	rows, err := s.DB().Query(r.Context(), `
		WITH per_device AS (
			SELECT s.device_id,
				date_trunc($1, s.recorded_at AT TIME ZONE 'UTC') AS bucket,
				max(s.onu_total) AS onu_total,
				max(s.onu_online) AS onu_online,
				max(s.onu_offline) AS onu_offline
			FROM olt_onu_samples s
			WHERE s.recorded_at >= $2 AND s.recorded_at < $3
				AND ($4::uuid IS NULL OR s.device_id = $4::uuid)
			GROUP BY s.device_id, bucket
		)
		SELECT d.id, d.description, pd.bucket, pd.onu_total, pd.onu_online, pd.onu_offline
		FROM per_device pd
		JOIN devices d ON d.id = pd.device_id
		ORDER BY d.description, pd.bucket
	`, bucket, since, until, deviceFilter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	type point struct {
		T       string `json:"t"`
		Total   int    `json:"total"`
		Online  int    `json:"online"`
		Offline int    `json:"offline"`
	}
	byDevice := map[string]map[string]any{}

	for rows.Next() {
		var id uuid.UUID
		var desc string
		var bucketTime time.Time
		var total, online, offline int
		if err := rows.Scan(&id, &desc, &bucketTime, &total, &online, &offline); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		ts := bucketTime.UTC().Format(time.RFC3339)
		key := id.String()
		if _, ok := byDevice[key]; !ok {
			byDevice[key] = map[string]any{
				"device_id":   id.String(),
				"description": desc,
				"points":      []point{},
			}
		}
		pts := byDevice[key]["points"].([]point)
		pts = append(pts, point{T: ts, Total: total, Online: online, Offline: offline})
		byDevice[key]["points"] = pts

	}

	series := make([]map[string]any, 0, len(byDevice))
	for _, v := range byDevice {
		series = append(series, v)
	}
	sort.Slice(series, func(i, j int) bool {
		return fmt.Sprint(series[i]["description"]) < fmt.Sprint(series[j]["description"])
	})

	aggRows, err := s.DB().Query(r.Context(), `
		WITH bucket_series AS (
			SELECT generate_series(
				date_trunc($1, $2::timestamptz),
				date_trunc($1, $3::timestamptz),
				$4::interval
			) AS bucket_start
		),
		device_ids AS (
			SELECT DISTINCT device_id FROM olt_onu_samples
			WHERE recorded_at >= $2 AND recorded_at < $3
				AND ($5::uuid IS NULL OR device_id = $5::uuid)
		),
		per_device_bucket AS (
			SELECT DISTINCT ON (bs.bucket_start, di.device_id)
				bs.bucket_start AS bucket,
				di.device_id,
				s.onu_total,
				s.onu_online,
				s.onu_offline
			FROM bucket_series bs
			CROSS JOIN device_ids di
			INNER JOIN olt_onu_samples s ON s.device_id = di.device_id
				AND s.recorded_at >= $2
				AND s.recorded_at < bs.bucket_start + $4::interval
			ORDER BY bs.bucket_start, di.device_id, s.recorded_at DESC
		)
		SELECT bucket, COALESCE(SUM(onu_total), 0), COALESCE(SUM(onu_online), 0), COALESCE(SUM(onu_offline), 0)
		FROM per_device_bucket
		GROUP BY bucket
		ORDER BY bucket
	`, bucket, since, until, interval, deviceFilter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer aggRows.Close()
	aggPts := make([]point, 0)
	for aggRows.Next() {
		var bucketTime time.Time
		var total, online, offline int
		if err := aggRows.Scan(&bucketTime, &total, &online, &offline); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		aggPts = append(aggPts, point{
			T:       bucketTime.UTC().Format(time.RFC3339),
			Total:   total,
			Online:  online,
			Offline: offline,
		})
	}

	var fleetTotal, fleetOn, fleetOff int64
	_ = s.DB().QueryRow(r.Context(), `
		SELECT
			COALESCE(SUM(sub.onu_total), 0),
			COALESCE(SUM(sub.onu_online), 0),
			COALESCE(SUM(sub.onu_offline), 0)
		FROM (
			SELECT DISTINCT ON (s.device_id)
				s.onu_total, s.onu_online, s.onu_offline
			FROM olt_onu_samples s
			WHERE $1::uuid IS NULL OR s.device_id = $1::uuid
			ORDER BY s.device_id, s.recorded_at DESC
		) sub
	`, deviceFilter).Scan(&fleetTotal, &fleetOn, &fleetOff)

	deviceIDResp := "all"
	if deviceFilter != nil {
		deviceIDResp = deviceFilter.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":      days,
		"bucket":    bucket,
		"since":     since.Format(time.RFC3339),
		"until":     until.Format(time.RFC3339),
		"device_id": deviceIDResp,
		"series":    series,
		"aggregate": map[string]any{"points": aggPts},
		"current_fleet": map[string]any{
			"onu_total":   fleetTotal,
			"onu_online":  fleetOn,
			"onu_offline": fleetOff,
		},
	})
}

// getOLTPonHistory — histórico por PORTA PON de UMA OLT específica (olt_pon_samples). Usado
// pela aba Relatório da OLT só quando uma OLT está seleccionada: um gráfico grande com o
// agregado (soma de todas as PONs por bucket) e um por PON para o grid. Mesmo bucketing
// dinâmico de getOLTReportsHistory (pickOLTHistoryBucket).
func (s *Server) getOLTPonHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	idStr := strings.TrimSpace(q.Get("device_id"))
	if idStr == "" || idStr == "all" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "device_id obrigatório — o histórico por PON é sempre de uma OLT específica.", nil)
		return
	}
	deviceID, err := uuid.Parse(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "device_id inválido.", nil)
		return
	}

	days := 7
	if d := strings.TrimSpace(q.Get("days")); d != "" {
		if n, e := strconv.Atoi(d); e == nil {
			days = n
		}
	}
	switch days {
	case 1, 3, 7, 30:
	default:
		days = 7
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	until := now
	customRange := false
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		t, e := time.Parse(time.RFC3339, fromStr)
		if e != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Parâmetro \"from\" inválido — use ISO 8601 (RFC3339).", nil)
			return
		}
		since, customRange = t.UTC(), true
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		t, e := time.Parse(time.RFC3339, toStr)
		if e != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Parâmetro \"to\" inválido — use ISO 8601 (RFC3339).", nil)
			return
		}
		until, customRange = t.UTC(), true
	}
	if !until.After(since) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "Período inválido: \"to\" deve ser depois de \"from\".", nil)
		return
	}
	if since.Before(now.Add(-366 * 24 * time.Hour)) {
		since = now.Add(-366 * 24 * time.Hour)
	}

	bucket, interval := pickOLTHistoryBucket(until.Sub(since))
	if !customRange && days != 1 {
		bucket, interval = "day", "1 day"
	}

	// Uma linha por (pon, bucket) — o valor de cada bucket é a última amostra dentro dele.
	rows, err := s.DB().Query(r.Context(), `
		SELECT DISTINCT ON (s.pon, date_trunc($1, s.recorded_at AT TIME ZONE 'UTC'))
			s.pon,
			COALESCE(NULLIF(s.pon_name, ''), 'PON ' || s.pon::text) AS pon_name,
			date_trunc($1, s.recorded_at AT TIME ZONE 'UTC') AS bucket,
			s.onu_total, s.onu_online, s.onu_offline
		FROM olt_pon_samples s
		WHERE s.device_id = $2 AND s.recorded_at >= $3 AND s.recorded_at < $4
		ORDER BY s.pon, date_trunc($1, s.recorded_at AT TIME ZONE 'UTC'), s.recorded_at DESC
	`, bucket, deviceID, since, until)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	type ponPoint struct {
		T       string `json:"t"`
		Total   int    `json:"total"`
		Online  int    `json:"online"`
		Offline int    `json:"offline"`
	}
	type ponSeries struct {
		Pon     int        `json:"pon"`
		PonName string     `json:"pon_name"`
		Points  []ponPoint `json:"points"`
	}
	byPon := map[int]*ponSeries{}
	// agregado: soma de todas as PONs por bucket
	agg := map[string]*ponPoint{}
	aggOrder := []string{}

	for rows.Next() {
		var pon, total, online, offline int
		var ponName string
		var bt time.Time
		if err := rows.Scan(&pon, &ponName, &bt, &total, &online, &offline); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		ts := bt.UTC().Format(time.RFC3339)
		ps := byPon[pon]
		if ps == nil {
			ps = &ponSeries{Pon: pon, PonName: ponName, Points: []ponPoint{}}
			byPon[pon] = ps
		}
		ps.Points = append(ps.Points, ponPoint{T: ts, Total: total, Online: online, Offline: offline})

		a := agg[ts]
		if a == nil {
			a = &ponPoint{T: ts}
			agg[ts] = a
			aggOrder = append(aggOrder, ts)
		}
		a.Total += total
		a.Online += online
		a.Offline += offline
	}

	pons := make([]ponSeries, 0, len(byPon))
	for _, ps := range byPon {
		pons = append(pons, *ps)
	}
	sort.Slice(pons, func(i, j int) bool { return pons[i].Pon < pons[j].Pon })

	sort.Strings(aggOrder)
	aggPts := make([]ponPoint, 0, len(aggOrder))
	for _, ts := range aggOrder {
		aggPts = append(aggPts, *agg[ts])
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID.String(),
		"days":      days,
		"bucket":    bucket,
		"since":     since.Format(time.RFC3339),
		"until":     until.Format(time.RFC3339),
		"interval":  interval,
		"pons":      pons,
		"aggregate": aggPts,
	})
}
