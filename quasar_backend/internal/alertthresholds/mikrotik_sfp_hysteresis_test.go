package alertthresholds

import (
	"testing"

	"github.com/google/uuid"
)

func TestWithinClearBandPotenciaERemperatura(t *testing.T) {
	rx := thresholdMetric{Operator: "lte", WarningMin: -19, HasWarning: true}
	if evalOne(-19.0, rx) != "warning" || evalOne(-18.98, rx) != "ok" {
		t.Fatal("pré-condição: -19,0 alarma e -18,98 não")
	}
	if !withinClearBand(-18.98, rx, sfpClearMarginDBm) {
		t.Error("-18,98 está dentro da folga: não pode resolver")
	}
	if withinClearBand(-18.2, rx, sfpClearMarginDBm) {
		t.Error("-18,2 está bem acima do limiar: pode resolver")
	}
	temp := thresholdMetric{Operator: "gte", WarningMin: 70, HasWarning: true}
	if !withinClearBand(69, temp, sfpClearMarginTemp) || withinClearBand(60, temp, sfpClearMarginTemp) {
		t.Error("folga da temperatura")
	}
}

func TestLeituraAusenteSoFechaDepoisDeVariasColetas(t *testing.T) {
	id := uuid.New()
	for i := 1; i < sfpMissesToClose; i++ {
		if sfpMissedTooLong(id, alertTypeSfpRx, 10) {
			t.Fatalf("fechou cedo demais na coleta %d", i)
		}
	}
	if !sfpMissedTooLong(id, alertTypeSfpRx, 10) {
		t.Fatal("deveria fechar após várias coletas sem leitura")
	}
	sfpMissedTooLong(id, alertTypeSfpRx, 11)
	sfpResetMiss(id, alertTypeSfpRx, 11)
	for i := 1; i < sfpMissesToClose; i++ {
		if sfpMissedTooLong(id, alertTypeSfpRx, 11) {
			t.Fatal("uma leitura boa zera a contagem")
		}
	}
}
