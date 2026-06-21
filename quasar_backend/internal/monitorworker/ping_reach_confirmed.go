package monitorworker

// pingOfflineConfirmed indica falha ICMP/TCP confirmada (streak >= limiar configurado).
// O cache reach_ok e o alerta ping_unreachable só devem reflectir este estado — não a falha
// isolada de um único ciclo, para alinhar monitoramento e alertas.
func pingOfflineConfirmed(probeReachOK bool, streakAfter, threshold int) bool {
	if probeReachOK {
		return false
	}
	return streakAfter >= consecutivePingsRequired(threshold)
}

// cacheReachOK é o valor persistido em device_probe_cache.reach_ok.
func cacheReachOK(probeReachOK bool, streakAfter, threshold int) bool {
	return !pingOfflineConfirmed(probeReachOK, streakAfter, threshold)
}
