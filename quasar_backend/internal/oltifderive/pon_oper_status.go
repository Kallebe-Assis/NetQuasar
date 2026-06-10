package oltifderive

// ApplyPonOperStatusFromOnuCounts define o status operacional da PON a partir de onu_online:
// ON (up) se ≥1 ONU online; OFF (down) se nenhuma ONU online.
// Regra universal aplicada após cada coleta quando há contagem de ONUs.
func ApplyPonOperStatusFromOnuCounts(row map[string]any) bool {
	on, ok := OnuOnlineFromRow(row)
	if !ok {
		return false
	}
	st := "down"
	if on >= 1 {
		st = "up"
	}
	row["pon_oper_status"] = st
	row["if_oper_status"] = st
	row["status"] = st
	row["pon_status_from_onu_counts"] = true
	return true
}

// ApplyPonOperStatusAll aplica ApplyPonOperStatusFromOnuCounts em cada linha PON.
func ApplyPonOperStatusAll(pons []map[string]any) {
	for _, p := range pons {
		ApplyPonOperStatusFromOnuCounts(p)
	}
}
