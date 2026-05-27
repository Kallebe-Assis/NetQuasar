package vsolparse

import (
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// OnlineOfflineByPon conta só OIDs 4.1.8 (1=online; 0, 2 ou outro=offline).
func OnlineOfflineByPon(vars []probing.SNMPVar) (online, offline map[int]int) {
	online = map[int]int{}
	offline = map[int]int{}
	for _, v := range vars {
		t, col, pon, onu, ok := parseSuffix(v.OID)
		if !ok || t != 4 || col != 8 || pon < 1 || onu < 1 {
			continue
		}
		if !probing.SNMPValueUsable(v.Value) {
			continue
		}
		st := intFromVal(strings.TrimSpace(v.Value))
		if OnuOnlineFromSta(st) {
			online[pon]++
		} else if st != fieldUnset {
			offline[pon]++
		}
		_ = onu
	}
	return online, offline
}

// AttachOnlineOfflineToIfPons mantém onu_total do IF-MIB; online/offline só do 4.1.8.
func AttachOnlineOfflineToIfPons(ifPons []map[string]any, onlineByPon, offlineByPon map[int]int) []map[string]any {
	if len(ifPons) == 0 {
		return ifPons
	}
	out := make([]map[string]any, 0, len(ifPons))
	for _, row := range ifPons {
		r := cloneRow(row)
		pid := ponIndexFromRow(r)
		on := onlineByPon[pid]
		off := offlineByPon[pid]
		tot := pickInt(r, "onu_total")
		r["onu_online"] = on
		r["onu_offline"] = off
		if tot > 0 && on+off < tot {
			r["onu_no_status"] = tot - on - off
		}
		r["online_source"] = "vsol_4.1.8"
		r["status"] = "if_mib_onu"
		oltifderive.NormalizePonONUCounts(r)
		out = append(out, r)
	}
	return out
}

func ponIndexFromRow(row map[string]any) int {
	if row == nil {
		return 0
	}
	k := oltifderive.StablePonRowKey(row)
	if k == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimLeft(k, "0"))
	if err != nil || n < 1 {
		n, _ = strconv.Atoi(k)
	}
	return n
}

func pickInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch x := m[key].(type) {
	case int:
		return x
	case float64:
		return int(x)
	case int64:
		return int(x)
	default:
		return 0
	}
}

func cloneRow(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+4)
	for k, v := range m {
		out[k] = v
	}
	return out
}
