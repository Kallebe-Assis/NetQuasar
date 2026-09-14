package oltcollect

import (
	"encoding/json"
	"strings"
)

// IsPlausibleOnuSerial evita gravar como serial um valor vindo de uma coluna/OID errado do
// perfil da OLT (Definições → Perfis OLT) — visto em produção: um perfil VSOL com o campo
// "serial" configurado por engano para a mesma tabela SNMP do "status", devolvendo o código de
// fase (3=working, 4, 6...) em vez do serial real. Todo serial de ONU real (VSOL/ZTE/Huawei/
// outros) tem pelo menos 8 caracteres (prefixo de fabricante + sufixo hex); um valor mais curto
// quase certamente não é um serial.
func IsPlausibleOnuSerial(s string) bool {
	return len(strings.TrimSpace(s)) >= 8
}

// SanitizeOnuSerialsMap é a última linha de defesa antes de qualquer gravação em
// olt_snapshots.summary: percorre summary["vsol_onu_rows"] e remove o campo "serial" de toda
// linha cujo valor não pareça um serial de verdade (ver IsPlausibleOnuSerial) — em vez de
// confiar só na validação em cada coletor individual (SNMP walk, "onu_metrics_collect"
// genérico, telnet...), que já falhou uma vez por um perfil de OLT mal configurado e pode
// falhar de novo por engano de configuração futuro, sem precisar de mudança de código.
//
// Muta summary in-place (os mapas dentro de vsol_onu_rows são partilhados pela referência do
// Go, não há necessidade de regravar a chave) e devolve quantas linhas foram limpas — 0 =
// nada a fazer, o chamador pode ignorar.
func SanitizeOnuSerialsMap(summary map[string]any) int {
	if summary == nil {
		return 0
	}
	rows := OnuRowsFromSummary(summary)
	cleaned := 0
	for _, row := range rows {
		if row == nil {
			continue
		}
		s, ok := row["serial"].(string)
		if !ok {
			continue
		}
		if !IsPlausibleOnuSerial(s) {
			delete(row, "serial")
			cleaned++
		}
	}
	return cleaned
}

// MergeSerialSearchMatch funde na lista de ONUs do summary uma correspondência confirmada por
// pesquisa telnet ao vivo (searchOLTOnuBySerial) — sem isto, uma ONU que a própria pesquisa
// acabou de provar que existe na OLT (PON/ONU/serial devolvidos pelo comando telnet) podia ficar
// de fora do snapshot SNMP (walk incompleto, ONU ainda não vista por SNMP, etc.), e por
// consequência de fora do conjunto de "seriais conhecidos" que valida o vínculo de cliente
// (allOltOnuSerials, handlers_olt_onu_client_links.go) — "encontrei pela pesquisa, mas não
// consigo vincular cliente porque não está na lista" era exactamente essa inconsistência.
//
// Actualiza a linha existente (por pon+onu) se já houver uma, ou acrescenta uma linha nova
// mínima caso contrário — o próximo ciclo de coleta completa o resto (potência, temperatura...).
// Devolve true se o summary foi alterado (o chamador decide se vale gravar).
func MergeSerialSearchMatch(summary map[string]any, pon, onu int, serial, model string) bool {
	serial = strings.ToUpper(strings.TrimSpace(serial))
	if summary == nil || pon <= 0 || onu <= 0 || !IsPlausibleOnuSerial(serial) {
		return false
	}
	rows := OnuRowsFromSummary(summary)
	changed := false
	found := false
	for _, row := range rows {
		if intFromRow(row, "pon") != pon || intFromRow(row, "onu") != onu {
			continue
		}
		found = true
		if cur, ok := row["serial"].(string); !ok || !strings.EqualFold(strings.TrimSpace(cur), serial) {
			row["serial"] = serial
			changed = true
		}
		if strings.TrimSpace(model) != "" {
			if cur, ok := row["model"].(string); !ok || strings.TrimSpace(cur) == "" {
				row["model"] = model
			}
		}
		break
	}
	if !found {
		newRow := map[string]any{"pon": pon, "onu": onu, "serial": serial}
		if strings.TrimSpace(model) != "" {
			newRow["model"] = model
		}
		rows = append(rows, newRow)
		changed = true
	}
	if changed {
		arr := make([]any, len(rows))
		for i, r := range rows {
			arr[i] = r
		}
		summary["vsol_onu_rows"] = arr
	}
	return changed
}

// CarryForwardOnuIdentity preenche serial/model das linhas ONU do snapshot NOVO a partir das
// linhas do snapshot ANTERIOR (casadas por pon+onu), quando a coleta atual não trouxe esses
// campos. Isto porque a maioria das OLTs deixa de devolver o serial por SNMP assim que a ONU
// cai — mas o serial é identidade estável, continua válido offline. Uma coleta que TRAZ um
// serial plausível sempre vence (confirma/atualiza); só se preenche o que veio em falta.
//
// Muta as linhas de summary["vsol_onu_rows"] in-place. Devolve quantas linhas ganharam serial.
func CarryForwardOnuIdentity(summary map[string]any, prevRows []map[string]any) int {
	if summary == nil || len(prevRows) == 0 {
		return 0
	}
	prevByKey := make(map[string]map[string]any, len(prevRows))
	for _, r := range prevRows {
		if r == nil {
			continue
		}
		if k := onuRowKey(r); k != "0.0" {
			prevByKey[k] = r
		}
	}
	filled := 0
	for _, row := range OnuRowsFromSummary(summary) {
		if row == nil {
			continue
		}
		prev, ok := prevByKey[onuRowKey(row)]
		if !ok {
			continue
		}
		if cur, _ := row["serial"].(string); !IsPlausibleOnuSerial(cur) {
			if ps, _ := prev["serial"].(string); IsPlausibleOnuSerial(ps) {
				row["serial"] = strings.ToUpper(strings.TrimSpace(ps))
				row["serial_source"] = "carried_prev"
				filled++
			}
		}
		if cur, _ := row["model"].(string); strings.TrimSpace(cur) == "" {
			if pm, _ := prev["model"].(string); strings.TrimSpace(pm) != "" {
				row["model"] = strings.TrimSpace(pm)
			}
		}
	}
	return filled
}

// CarryForwardOnuIdentityJSON é o equivalente de CarryForwardOnuIdentity para pontos de gravação
// que lidam com o summary já serializado. `newRaw` é o summary novo; `prevRaw` o anterior.
func CarryForwardOnuIdentityJSON(newRaw, prevRaw []byte) []byte {
	if len(newRaw) == 0 || len(prevRaw) == 0 {
		return newRaw
	}
	var newSum, prevSum map[string]any
	if json.Unmarshal(newRaw, &newSum) != nil || json.Unmarshal(prevRaw, &prevSum) != nil {
		return newRaw
	}
	if CarryForwardOnuIdentity(newSum, OnuRowsFromSummary(prevSum)) == 0 {
		return newRaw
	}
	out, err := json.Marshal(newSum)
	if err != nil {
		return newRaw
	}
	return out
}

// SanitizeOnuSerialsJSON é a mesma limpeza que SanitizeOnuSerialsMap, mas para quando só se tem
// o JSON já serializado (alguns pontos de gravação em olt_snapshots recebem/produzem []byte, não
// o map[string]any vivo). Devolve o `raw` original sem alterações se não houver nada para limpar
// ou se o JSON não puder ser interpretado (nunca falha silenciosamente perdendo dados).
func SanitizeOnuSerialsJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	if SanitizeOnuSerialsMap(m) == 0 {
		return raw
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}
