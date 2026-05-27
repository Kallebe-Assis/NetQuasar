package oltifderive

import "fmt"

// NormalizePonONUCounts ajusta totais por PON. Com online_source=vsol_4.1.8 não inventa offline (= total − online).
func NormalizePonONUCounts(m map[string]any) {
	if m == nil {
		return
	}
	tot := rowPickInt(m, "onu_total", "total_onu", "onus", "onus_total", "onu_count")
	on := rowPickInt(m, "onu_online", "online", "onu_ok")
	off := rowPickInt(m, "onu_offline", "offline", "onu_down")
	if fmt.Sprint(m["online_source"]) == "vsol_4.1.8" {
		if on > tot && tot > 0 {
			on = tot
		}
		if off > tot && tot > 0 {
			off = tot
		}
		if tot > 0 && on+off > tot {
			off = tot - on
			if off < 0 {
				off = 0
			}
		}
		m["onu_total"] = tot
		m["onu_online"] = on
		m["onu_offline"] = off
		if tot > 0 && on+off < tot {
			m["onu_no_status"] = tot - on - off
		}
		return
	}
	if tot <= 0 {
		if on > 0 || off > 0 {
			tot = on + off
		}
	} else {
		if on > tot {
			on = tot
		}
		if on+off > tot {
			off = tot - on
			if off < 0 {
				off = 0
			}
		} else if on+off < tot {
			off = tot - on
		}
	}
	m["onu_total"] = tot
	m["onu_online"] = on
	m["onu_offline"] = off
}
