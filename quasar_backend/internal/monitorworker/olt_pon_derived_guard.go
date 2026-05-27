package monitorworker

import "strings"

// OltUsesIfDerivedPonSnapshots indica que contagem ONUs/PON deve vir de derive IF-MIB + worker walk.
// ZTE/DATACOM usam SNMP de fabricante (e em geral Telnet/ZTE CLI no refresh da API); gravar apenas IF-MIB aqui sobrescreve snapshots correctos com dados errados ou vazios.
func OltUsesIfDerivedPonSnapshots(category, brand, model string) bool {
	cat := strings.EqualFold(strings.TrimSpace(category), "olt")
	if !cat {
		return false
	}
	bl := strings.ToLower(strings.TrimSpace(brand) + " " + strings.TrimSpace(model))
	if strings.Contains(bl, "zte") || strings.Contains(bl, "zxa10") {
		return false
	}
	if strings.Contains(bl, "datacom") {
		return false
	}
	// VSOL: contagens e estado vêm do MIB enterprise (gOnuAuthList); IF-MIB incompleto sobrescreve totais.
	if strings.Contains(bl, "vsol") || strings.Contains(bl, "v1600") || strings.Contains(bl, "1600g") {
		return false
	}
	return true
}
