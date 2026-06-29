package bngcollect

import (
	"fmt"
	"sort"
	"strings"
)

type CountBucket struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type LimitBucket struct {
	Kbps    int    `json:"kbps"`
	Label   string `json:"label"`
	Display string `json:"display"`
	Count   int    `json:"count"`
}

type OnlineTimeReport struct {
	AvgSeconds  int           `json:"avg_seconds"`
	AvgDisplay  string        `json:"avg_display"`
	MaxSeconds  int           `json:"max_seconds"`
	MaxDisplay  string        `json:"max_display"`
	WithData    int           `json:"with_data"`
	Buckets     []CountBucket `json:"buckets"`
}

type SessionReport struct {
	SessionCount int              `json:"session_count"`
	ByVLAN       []CountBucket    `json:"by_vlan"`
	ByUpLimit    []LimitBucket    `json:"by_up_limit"`
	ByDnLimit    []LimitBucket    `json:"by_dn_limit"`
	OnlineTime   OnlineTimeReport `json:"online_time"`
	Traffic      TrafficSummary   `json:"traffic"`
}

type TrafficSummary struct {
	UpFlow64Total   int64  `json:"up_flow64_total"`
	DnFlow64Total   int64  `json:"dn_flow64_total"`
	UpFlowDisplay   string `json:"up_flow_display"`
	DnFlowDisplay   string `json:"dn_flow_display"`
	SessionsWithUp  int    `json:"sessions_with_up"`
	SessionsWithDn  int    `json:"sessions_with_dn"`
}

func vlanLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "0" {
		return "Sem VLAN"
	}
	return v
}

func limitKey(kbps int) string {
	if kbps <= 0 {
		return "none"
	}
	return fmt.Sprintf("%d", kbps)
}

func onlineTimeBucket(sec int) string {
	switch {
	case sec < 3600:
		return "lt_1h"
	case sec < 86400:
		return "1h_24h"
	case sec < 7*86400:
		return "1d_7d"
	case sec < 30*86400:
		return "7d_30d"
	default:
		return "gt_30d"
	}
}

var onlineTimeBucketLabels = map[string]string{
	"lt_1h":   "Menos de 1 hora",
	"1h_24h":  "1 hora a 24 horas",
	"1d_7d":   "1 a 7 dias",
	"7d_30d":  "7 a 30 dias",
	"gt_30d":  "Mais de 30 dias",
}

// BuildSessionReport agrega sessões PPPoE para a aba Relatório.
func BuildSessionReport(sessions []SessionRow) SessionReport {
	rep := SessionReport{SessionCount: len(sessions)}
	if len(sessions) == 0 {
		return rep
	}

	vlanCounts := map[string]int{}
	upCounts := map[int]int{}
	dnCounts := map[int]int{}
	onlineBuckets := map[string]int{}
	var onlineSum, onlineMax, onlineN int
	var upFlowSum, dnFlowSum int64
	var upN, dnN int

	for _, row := range sessions {
		vlan := vlanLabel(row.VLAN)
		vlanCounts[vlan]++

		up, _ := parseIntMetric(row.CarUpCIRKbps)
		dn, _ := parseIntMetric(row.CarDnCIRKbps)
		upCounts[up]++
		dnCounts[dn]++

		if n, ok := parseInt64Metric(row.UpFlowBytes); ok && n > 0 {
			upFlowSum += n
			upN++
		}
		if n, ok := parseInt64Metric(row.DnFlowBytes); ok && n > 0 {
			dnFlowSum += n
			dnN++
		}

		sec, _ := parseIntMetric(row.OnlineTimeSec)
		if sec > 0 {
			onlineN++
			onlineSum += sec
			if sec > onlineMax {
				onlineMax = sec
			}
			onlineBuckets[onlineTimeBucket(sec)]++
		}
	}

	rep.ByVLAN = sortCountBuckets(vlanCounts)
	rep.ByUpLimit = sortLimitBuckets(upCounts)
	rep.ByDnLimit = sortLimitBuckets(dnCounts)
	rep.Traffic = TrafficSummary{
		UpFlow64Total:  upFlowSum,
		DnFlow64Total:  dnFlowSum,
		UpFlowDisplay:  FormatFlow64Volume(fmt.Sprintf("%d", upFlowSum)),
		DnFlowDisplay:  FormatFlow64Volume(fmt.Sprintf("%d", dnFlowSum)),
		SessionsWithUp: upN,
		SessionsWithDn: dnN,
	}

	if onlineN > 0 {
		avg := int(mathRound(float64(onlineSum) / float64(onlineN)))
		rep.OnlineTime = OnlineTimeReport{
			AvgSeconds: avg,
			AvgDisplay: FormatDurationSeconds(avg),
			MaxSeconds: onlineMax,
			MaxDisplay: FormatDurationSeconds(onlineMax),
			WithData:   onlineN,
			Buckets:    sortOnlineBuckets(onlineBuckets),
		}
	} else {
		rep.OnlineTime.Buckets = []CountBucket{}
	}
	return rep
}

func mathRound(v float64) int {
	if v <= 0 {
		return 0
	}
	return int(v + 0.5)
}

func SessionRowFromMap(m map[string]any) SessionRow {
	str := func(k string) string {
		if m == nil {
			return ""
		}
		v, ok := m[k]
		if !ok || v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return SessionRow{
		Index:          str("index"),
		Login:          str("login"),
		VLAN:           str("vlan"),
		OnlineTime:     str("online_time"),
		OnlineTimeSec:  str("online_time_sec"),
		AuthState:      str("auth_state"),
		AuthStateRaw:   str("auth_state_raw"),
		AuthorState:    str("author_state"),
		AuthorStateRaw: str("author_state_raw"),
		CarUpCIRKbps:   str("car_up_cir_kbps"),
		CarDnCIRKbps:   str("car_dn_cir_kbps"),
		UpFlowBytes:    str("up_flow_bytes"),
		DnFlowBytes:    str("dn_flow_bytes"),
	}
}

// BuildSessionReportFromMaps agrega a partir de snapshots JSON.
func BuildSessionReportFromMaps(maps []map[string]any) SessionReport {
	rows := make([]SessionRow, 0, len(maps))
	for _, m := range maps {
		rows = append(rows, SessionRowFromMap(m))
	}
	return BuildSessionReport(rows)
}

func sortCountBuckets(m map[string]int) []CountBucket {
	out := make([]CountBucket, 0, len(m))
	for k, n := range m {
		out = append(out, CountBucket{Key: k, Label: k, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func sortLimitBuckets(m map[int]int) []LimitBucket {
	out := make([]LimitBucket, 0, len(m))
	for kbps, n := range m {
		label := "Sem limite"
		display := FormatKbitRate(0)
		if kbps > 0 {
			label = FormatKbitRate(kbps)
			display = label
		}
		out = append(out, LimitBucket{
			Kbps: kbps, Label: label, Display: display, Count: n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Kbps > out[j].Kbps
	})
	return out
}

func sortOnlineBuckets(m map[string]int) []CountBucket {
	order := []string{"lt_1h", "1h_24h", "1d_7d", "7d_30d", "gt_30d"}
	out := make([]CountBucket, 0, len(order))
	for _, key := range order {
		if n, ok := m[key]; ok && n > 0 {
			out = append(out, CountBucket{
				Key: key, Label: onlineTimeBucketLabels[key], Count: n,
			})
		}
	}
	return out
}
