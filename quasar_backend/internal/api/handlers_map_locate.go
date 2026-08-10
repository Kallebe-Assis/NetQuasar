package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reLatLngPair = regexp.MustCompile(`(?i)^\s*(-?\d{1,3}(?:[.,]\d+)?)\s*[,;\s]\s*(-?\d{1,3}(?:[.,]\d+)?)\s*$`)
	reMapsAt     = regexp.MustCompile(`(?i)@(-?\d{1,3}(?:\.\d+)?),\s*(-?\d{1,3}(?:\.\d+)?)`)
	reMapsQ      = regexp.MustCompile(`(?i)[?&](?:q|query|ll)=(-?\d{1,3}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)`)
	reMaps3d4d   = regexp.MustCompile(`(?i)!3d(-?\d{1,3}(?:\.\d+)?)!4d(-?\d{1,3}(?:\.\d+)?)`)
	reOSMHash    = regexp.MustCompile(`(?i)#map=\d+/(-?\d{1,3}(?:\.\d+)?)/(-?\d{1,3}(?:\.\d+)?)`)
)

type mapLocateHit struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Label   string  `json:"label"`
	Source  string  `json:"source"` // coords | maps_url | geocode
	Display string  `json:"display,omitempty"`
}

func parseFloatLoose(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func validLatLng(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func parseLatLngPair(raw string) (lat, lng float64, ok bool) {
	m := reLatLngPair.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0, 0, false
	}
	lat, ok1 := parseFloatLoose(m[1])
	lng, ok2 := parseFloatLoose(m[2])
	if !ok1 || !ok2 || !validLatLng(lat, lng) {
		return 0, 0, false
	}
	return lat, lng, true
}

func parseCoordsFromMapsURL(raw string) (lat, lng float64, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, 0, false
	}
	for _, re := range []*regexp.Regexp{reMapsAt, reMapsQ, reMaps3d4d, reOSMHash} {
		if m := re.FindStringSubmatch(s); m != nil {
			lat, ok1 := parseFloatLoose(m[1])
			lng, ok2 := parseFloatLoose(m[2])
			if ok1 && ok2 && validLatLng(lat, lng) {
				return lat, lng, true
			}
		}
	}
	u, err := url.Parse(s)
	if err == nil {
		q := u.Query()
		for _, key := range []string{"q", "query", "ll"} {
			if v := strings.TrimSpace(q.Get(key)); v != "" {
				if lat, lng, ok = parseLatLngPair(v); ok {
					return lat, lng, true
				}
			}
		}
	}
	return 0, 0, false
}

func looksLikeHTTPURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func resolveHTTPFinalURL(ctx context.Context, raw string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "NetQuasar/1.0 (map-locate)")
	client := &http.Client{
		Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 12 {
				return fmt.Errorf("demasiados redireccionamentos")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String(), nil
	}
	return raw, nil
}

func nominatimSearch(ctx context.Context, q string, limit int) ([]mapLocateHit, error) {
	if limit <= 0 || limit > 8 {
		limit = 5
	}
	u := url.URL{
		Scheme: "https",
		Host:   "nominatim.openstreetmap.org",
		Path:   "/search",
	}
	qs := u.Query()
	qs.Set("format", "jsonv2")
	qs.Set("limit", strconv.Itoa(limit))
	qs.Set("q", q)
	u.RawQuery = qs.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NetQuasar/1.0 (ISP NOC map locate; contact local admin)")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("geocode HTTP %d", resp.StatusCode)
	}
	var rows []struct {
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rows); err != nil {
		return nil, err
	}
	out := make([]mapLocateHit, 0, len(rows))
	for _, row := range rows {
		lat, ok1 := parseFloatLoose(row.Lat)
		lng, ok2 := parseFloatLoose(row.Lon)
		if !ok1 || !ok2 || !validLatLng(lat, lng) {
			continue
		}
		label := strings.TrimSpace(row.DisplayName)
		if label == "" {
			label = fmt.Sprintf("%.5f, %.5f", lat, lng)
		}
		out = append(out, mapLocateHit{
			Lat: lat, Lng: lng, Label: label, Source: "geocode", Display: label,
		})
	}
	return out, nil
}

// mapLocate resolve coordenadas, URLs do Google Maps (incl. goo.gl) ou endereço (Nominatim).
func (s *Server) mapLocate(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "informe q", nil)
		return
	}
	ctx := r.Context()
	hits := make([]mapLocateHit, 0, 6)

	if lat, lng, ok := parseLatLngPair(q); ok {
		hits = append(hits, mapLocateHit{
			Lat: lat, Lng: lng,
			Label:   fmt.Sprintf("%.6f, %.6f", lat, lng),
			Source:  "coords",
			Display: "Coordenadas",
		})
		writeJSON(w, http.StatusOK, map[string]any{"results": hits})
		return
	}

	if looksLikeHTTPURL(q) {
		finalURL := q
		if lat, lng, ok := parseCoordsFromMapsURL(q); ok {
			hits = append(hits, mapLocateHit{
				Lat: lat, Lng: lng,
				Label:   fmt.Sprintf("%.6f, %.6f", lat, lng),
				Source:  "maps_url",
				Display: "Link do mapa",
			})
			writeJSON(w, http.StatusOK, map[string]any{"results": hits, "resolved_url": finalURL})
			return
		}
		resolved, err := resolveHTTPFinalURL(ctx, q)
		if err == nil && resolved != "" {
			finalURL = resolved
		}
		if lat, lng, ok := parseCoordsFromMapsURL(finalURL); ok {
			hits = append(hits, mapLocateHit{
				Lat: lat, Lng: lng,
				Label:   fmt.Sprintf("%.6f, %.6f", lat, lng),
				Source:  "maps_url",
				Display: "Link do mapa",
			})
			writeJSON(w, http.StatusOK, map[string]any{"results": hits, "resolved_url": finalURL})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"results":      hits,
			"resolved_url": finalURL,
			"note":         "Não foi possível extrair coordenadas deste link.",
		})
		return
	}

	geo, err := nominatimSearch(ctx, q, 5)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "GEOCODE", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": geo})
}
