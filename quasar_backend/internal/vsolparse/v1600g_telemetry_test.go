package vsolparse

import (
	"fmt"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestFromSNMPWalk_telemetryWithoutOnlineFirst(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.2.1", Value: "-23.46"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6.4.10", Value: "125GV1.0"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.4.2.1", Value: "3.3"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5.2.1", Value: "SN0000123456"},
	}
	_, _, rows := FromSNMPWalk(vars, false)
	byKey := map[string]map[string]any{}
	for _, r := range rows {
		byKey[fmt.Sprintf("%v.%v", r["pon"], r["onu"])] = r
	}
	if len(byKey) != 2 {
		t.Fatalf("rows %d", len(byKey))
	}
	r := byKey["2.1"]
	if r["rx_pwr"] != "-23.46" || r["voltage"] != "3.3" || r["serial"] != "SN0000123456" {
		t.Fatalf("pon2 %+v", r)
	}
	r = byKey["4.10"]
	if r["model"] != "125GV1.0" {
		t.Fatalf("pon4 model %v", r["model"])
	}
}

// TestFromSNMPWalk_rejectsImplausibleSerial cobre o bug visto em produção na firmware VSOL
// V1600G0B: a coluna que deveria trazer o serial da ONU devolvia um código de estado numérico
// curto ("3", "4", "6"...), que era gravado como se fosse o serial real — bloqueando depois o
// serial correto vindo do telnet (mergeTelnetFieldsIntoOnuRow/setIfEmpty só preenche campo
// vazio). isPlausibleOnuSerial deve rejeitar esses valores curtos em ambas as tabelas (branch
// 2 e branch 4, coluna 5) e deixar "serial" vazio em vez de gravar o valor errado.
func TestFromSNMPWalk_rejectsImplausibleSerial(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5.1.1", Value: "3"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6.1.1", Value: "D401"}, // model, para a linha não ser descartada por "vazia"
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.5.2.1", Value: "6"},
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.4.1.17.2.1", Value: "D401"}, // idem
		{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5.3.1", Value: "SN0000123456"},
	}
	_, _, rows := FromSNMPWalk(vars, false)
	byKey := map[string]map[string]any{}
	for _, r := range rows {
		byKey[fmt.Sprintf("%v.%v", r["pon"], r["onu"])] = r
	}
	if r := byKey["1.1"]; r["serial"] != "" {
		t.Fatalf("pon1 devia rejeitar serial curto, veio %+v", r)
	}
	if r := byKey["2.1"]; r["serial"] != "" {
		t.Fatalf("pon2 devia rejeitar serial curto, veio %+v", r)
	}
	if r := byKey["3.1"]; r["serial"] != "SN0000123456" {
		t.Fatalf("pon3 devia manter serial plausível, veio %+v", r)
	}
}
