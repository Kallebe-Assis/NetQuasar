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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
)

func recordOLTOnuSample(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, summaryJSON, ponsJSON []byte) {
	if pool == nil {
		return
	}
	c := oltparse.SnapshotComputed(summaryJSON, ponsJSON)
	total := intValMap(c, "onu_total_sum")
	online := intValMap(c, "onu_online_sum")
	offline := intValMap(c, "onu_offline_sum")
	if total == 0 && online == 0 && offline == 0 {
		return
	}
	_, _ = pool.Exec(ctx, `
		INSERT INTO olt_onu_samples (device_id, onu_total, onu_online, onu_offline)
		VALUES ($1, $2, $3, $4)
	`, deviceID, total, online, offline)
}

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

func (s *Server) getOLTReportsHistory(w http.ResponseWriter, r *http.Request) {
	days := 7
	if d := strings.TrimSpace(r.URL.Query().Get("days")); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			days = n
		}
	}
	switch days {
	case 1, 3, 7, 30:
	default:
		days = 7
	}
	bucket := "day"
	if days == 1 {
		bucket = "hour"
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	rows, err := s.DB().Query(r.Context(), `
		WITH per_device AS (
			SELECT s.device_id,
				date_trunc($1, s.recorded_at AT TIME ZONE 'UTC') AS bucket,
				max(s.onu_total) AS onu_total,
				max(s.onu_online) AS onu_online,
				max(s.onu_offline) AS onu_offline
			FROM olt_onu_samples s
			WHERE s.recorded_at >= $2
			GROUP BY s.device_id, bucket
		)
		SELECT d.id, d.description, pd.bucket, pd.onu_total, pd.onu_online, pd.onu_offline
		FROM per_device pd
		JOIN devices d ON d.id = pd.device_id
		ORDER BY d.description, pd.bucket
	`, bucket, since)
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
	aggBuckets := map[string]*point{}

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

		if aggBuckets[ts] == nil {
			aggBuckets[ts] = &point{T: ts}
		}
		aggBuckets[ts].Total += total
		aggBuckets[ts].Online += online
		aggBuckets[ts].Offline += offline
	}

	series := make([]map[string]any, 0, len(byDevice))
	for _, v := range byDevice {
		series = append(series, v)
	}
	sort.Slice(series, func(i, j int) bool {
		return fmt.Sprint(series[i]["description"]) < fmt.Sprint(series[j]["description"])
	})
	aggPts := make([]point, 0, len(aggBuckets))
	for _, p := range aggBuckets {
		aggPts = append(aggPts, *p)
	}
	sort.Slice(aggPts, func(i, j int) bool { return aggPts[i].T < aggPts[j].T })

	writeJSON(w, http.StatusOK, map[string]any{
		"days":      days,
		"bucket":    bucket,
		"since":     since.Format(time.RFC3339),
		"series":    series,
		"aggregate": map[string]any{"points": aggPts},
	})
}
