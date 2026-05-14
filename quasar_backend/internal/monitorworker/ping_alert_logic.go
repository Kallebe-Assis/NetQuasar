package monitorworker

// shouldOpenPingUnreachableAlert indica se a sequência de falhas ICMP/TCP atingiu o limiar
// configurado (offline_ping_fail_threshold). Não exige transição «acabou de ficar offline» na
// leitura anterior — só o streak no cache, para não perder alertas quando threshold > 1.
func shouldOpenPingUnreachableAlert(reachOK bool, streakAfter, threshold int) bool {
	if reachOK || threshold < 1 {
		return false
	}
	return streakAfter >= threshold
}
