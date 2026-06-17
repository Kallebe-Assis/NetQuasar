package api

import "time"

// mapDeviceReachabilityStatus alinha o mapa com a tela de monitoramento (ping_reachable / reach_ok).
func mapDeviceReachabilityStatus(pingEnabled bool, checkedAt *time.Time, probeOK bool, reachOK *bool) string {
	if checkedAt == nil {
		return "unknown"
	}
	if pingEnabled {
		if reachOK == nil {
			return "unknown"
		}
		if *reachOK {
			return "online"
		}
		return "offline"
	}
	if probeOK {
		return "online"
	}
	return "offline"
}
