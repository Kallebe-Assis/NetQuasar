package monitorworker

// ShouldOpenPingUnreachableAlert abre ping_unreachable quando a falha está confirmada
// (mesmo critério de reach_ok no cache e estado offline na UI de monitoramento).
func ShouldOpenPingUnreachableAlert(probeReachOK bool, streakAfter, threshold int) bool {
	return pingOfflineConfirmed(probeReachOK, streakAfter, threshold)
}

func shouldOpenPingUnreachableAlert(probeReachOK bool, streakAfter, threshold int) bool {
	return ShouldOpenPingUnreachableAlert(probeReachOK, streakAfter, threshold)
}
