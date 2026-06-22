package oltifderive

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PonsJSONToMaps faz parse de snapshot `pons::text`.
func PonsJSONToMaps(raw []byte) []map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var arr []any
	if json.Unmarshal(raw, &arr) != nil || len(arr) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// PonsAnySliceToMaps converte o slice da API/handlers antes da estabilização.
func PonsAnySliceToMaps(pons []any) []map[string]any {
	out := make([]map[string]any, 0, len(pons))
	for _, x := range pons {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// PonsMapsToAny prepara valores para gravar em JSONB.
func PonsMapsToAny(rows []map[string]any) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

// SummaryJSONBytesToMap faz parse de `summary`.
func SummaryJSONBytesToMap(raw []byte) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// MinPrevOnlineToDoubtZeroRead — se o snapshot anterior tinha pelo menos este número de ONUs online
// numa PON e a nova leitura veio ≤0.5, a contagem duvidosa só é aceite após repetir N vezes (streak).
const MinPrevOnlineToDoubtZeroRead = 4

// SuspiciousZeroReadsBeforeAccept — confirmações consecutivas de leitura “zero suspeito” até aceitar
// o valor 0 nos alertas/gravação (evita dois ciclos com walk SNMP falho).
const SuspiciousZeroReadsBeforeAccept = 3

const summaryKeyOnuZeroConfirm = "onu_zero_confirm_streak"

// OnuOnlineFromRow lê contagens vindas de JSON Go, VSOL ou mapas SNMP.
func OnuOnlineFromRow(row map[string]any) (float64, bool) {
	if row == nil {
		return 0, false
	}
	switch v := row["onu_online"].(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil && f >= 0
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(v, ",", "."))
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil && f >= 0
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return 0, false
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		return f, err == nil && f >= 0
	}
}

func loadOnuZeroStreaks(prevSummary map[string]any) map[string]int {
	out := make(map[string]int)
	if prevSummary == nil {
		return out
	}
	raw, ok := prevSummary[summaryKeyOnuZeroConfirm]
	if !ok || raw == nil {
		return out
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for k, v := range m {
		switch n := v.(type) {
		case float64:
			out[k] = int(n)
		case int:
			out[k] = n
		case int64:
			out[k] = int(n)
		default:
			i, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
			out[k] = i
		}
	}
	return out
}

// PreserveMissingPonRows repõe PONs do snapshot anterior ausentes na leitura nova (walk truncado/incompleto).
func PreserveMissingPonRows(prevPons, newPons []map[string]any, collectionIncomplete bool) []map[string]any {
	dedup := DedupePonMaps(newPons)
	if !collectionIncomplete {
		return dedup
	}
	idx := map[string]map[string]any{}
	var order []string
	for _, row := range dedup {
		k := StablePonRowKey(row)
		if k == "" {
			continue
		}
		idx[k] = row
		order = append(order, k)
	}
	for _, prow := range prevPons {
		k := StablePonRowKey(prow)
		if k == "" {
			continue
		}
		if _, ok := idx[k]; ok {
			continue
		}
		carry := cloneMap(prow)
		carry["pon_row_carried_forward"] = true
		idx[k] = carry
		order = append(order, k)
	}
	out := make([]map[string]any, 0, len(order))
	for _, k := range order {
		row := idx[k]
		if k != "" {
			row["id"] = k
		}
		NormalizePonONUCounts(row)
		out = append(out, row)
	}
	return out
}

func ponRowIsVsolSNMP(row map[string]any) bool {
	return strings.TrimSpace(fmt.Sprint(row["status"])) == "vsol_snmp"
}

// StabilizePonSnapshotRows devolve cópias das linhas de `newPons` onde uma leitura 0 duvidosa
// (snapshot anterior alto) mantém temporariamente a contagem anterior até segunda coleta igual.
func StabilizePonSnapshotRows(prevPons []map[string]any, newPons []map[string]any, prevSummary map[string]any, collectionIncomplete bool) ([]map[string]any, map[string]any) {
	newPons = PreserveMissingPonRows(prevPons, newPons, collectionIncomplete)
	streak := loadOnuZeroStreaks(prevSummary)

	prevOnlineByKey := map[string]float64{}
	for _, row := range prevPons {
		k := StablePonRowKey(row)
		if k == "" {
			continue
		}
		if v, ok := OnuOnlineFromRow(row); ok {
			prevOnlineByKey[k] = v
		}
	}

	dedup := DedupePonMaps(newPons)
	keySeen := map[string]struct{}{}
	out := make([]map[string]any, 0, len(dedup))
	for _, row := range dedup {
		k := StablePonRowKey(row)
		if k == "" {
			continue
		}
		keySeen[k] = struct{}{}

		prevOn := prevOnlineByKey[k]

		rowCopy := cloneMap(row)
		cur, curOK := OnuOnlineFromRow(row)
		if !curOK {
			cur = 0
		}

		suspiciousZero := prevOn >= float64(MinPrevOnlineToDoubtZeroRead) && cur <= 0.5 && prevOn > 0.5
		delete(rowCopy, "onu_online_snap_held")

		if suspiciousZero && ponRowIsVsolSNMP(row) && collectionIncomplete {
			// Leitura VSOL incompleta: não forçar 0 offline nem aceitar queda por truncamento.
			streak[k] = 0
		} else if suspiciousZero {
			streak[k]++
			if streak[k] < SuspiciousZeroReadsBeforeAccept {
				on := int(prevOn + 0.5)
				rowCopy["onu_online"] = on
				rowCopy["onu_online_snap_held"] = true
				if tot := rowPickInt(rowCopy, "onu_total", "total_onu", "onus", "onus_total", "onu_count"); tot > on {
					rowCopy["onu_offline"] = tot - on
				}
			} else {
				// Segunda vez consecutiva com 0: aceitar (queda ou leituras consistentemente vazias).
				streak[k] = 0
				rowCopy["onu_online"] = int(cur + 0.5)
				if tot := rowPickInt(rowCopy, "onu_total", "total_onu", "onus", "onus_total", "onu_count"); tot > 0 && cur <= 0.5 {
					rowCopy["onu_offline"] = tot
				}
			}
		} else {
			streak[k] = 0
		}

		out = append(out, rowCopy)
	}
	for kk := range streak {
		if _, ok := keySeen[kk]; !ok {
			delete(streak, kk)
		}
	}

	// Remove chaves sem sequência pendente para o JSON ficar compacto e substituível no summary.
	activeStreak := map[string]any{}
	for k, v := range streak {
		if v > 0 {
			activeStreak[k] = v
		}
	}
	patchSumm := map[string]any{}
	if len(activeStreak) > 0 {
		patchSumm[summaryKeyOnuZeroConfirm] = activeStreak
	} else {
		// Sobrescreve entradas antigas no merge com estado vazio.
		patchSumm[summaryKeyOnuZeroConfirm] = map[string]any{}
	}

	return DedupePonMaps(out), patchSumm
}

// PreservePonCountsOnIncomplete mantém contagens online/offline do snapshot anterior quando a coleta actual é incompleta.
func PreservePonCountsOnIncomplete(prev, cur []map[string]any) ([]map[string]any, map[string]any) {
	patch := map[string]any{}
	if len(prev) == 0 || len(cur) == 0 {
		return cur, patch
	}
	prevByKey := map[string]map[string]any{}
	for _, p := range prev {
		k := StablePonRowKey(p)
		if k != "" {
			prevByKey[k] = p
		}
	}
	out := make([]map[string]any, 0, len(cur))
	carried := 0
	for _, p := range cur {
		cp := map[string]any{}
		for k, v := range p {
			cp[k] = v
		}
		key := StablePonRowKey(p)
		if prevP, ok := prevByKey[key]; ok {
			prevOn, prevOK := OnuOnlineFromRow(prevP)
			curOn, curOK := OnuOnlineFromRow(p)
			if prevOK && curOK && curOn < prevOn {
				cp["onu_online"] = prevOn
				if v, ok := prevP["onu_offline"]; ok {
					cp["onu_offline"] = v
				}
				cp["online_source"] = "carried_incomplete_snmp"
				carried++
			}
		}
		out = append(out, cp)
	}
	if carried > 0 {
		patch["pon_counts_carried_incomplete"] = carried
	}
	return out, patch
}
