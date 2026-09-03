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
