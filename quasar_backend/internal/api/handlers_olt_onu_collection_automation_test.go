package api

import (
	"testing"
	"time"
)

func TestOltOnuCollectionDue(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	ago := func(m int) *time.Time { v := now.Add(-time.Duration(m) * time.Minute); return &v }

	cases := []struct {
		name                string
		lightEn             bool
		lightMin            int
		lastLight           *time.Time
		runL                bool
		fullEn              bool
		fullMin             int
		lastFull            *time.Time
		runF                bool
		wantFull, wantLight bool
	}{
		{"nunca rodou: completa primeiro", true, 5, nil, false, true, 360, nil, false, true, false},
		{"leve vencida, completa em dia", true, 5, ago(6), false, true, 360, ago(10), false, false, true},
		{"nenhuma vencida", true, 5, ago(2), false, true, 360, ago(10), false, false, false},
		{"completa vencida tem prioridade", true, 5, ago(30), false, true, 360, ago(361), false, true, false},
		{"leve não roda durante completa", true, 5, ago(30), false, true, 360, ago(10), true, false, false},
		{"leve já em execução", true, 5, ago(30), true, true, 360, ago(10), false, false, false},
		{"só leve ligada", true, 5, ago(6), false, false, 360, nil, false, false, true},
		{"só completa ligada", false, 5, nil, false, true, 360, ago(400), false, true, false},
		{"tudo desligado", false, 5, nil, false, false, 360, nil, false, false, false},
	}
	for _, c := range cases {
		gotF, gotL := oltOnuCollectionDue(now, c.lightEn, c.lightMin, c.lastLight, c.runL, c.fullEn, c.fullMin, c.lastFull, c.runF)
		if gotF != c.wantFull || gotL != c.wantLight {
			t.Errorf("%s: full=%v light=%v, want full=%v light=%v", c.name, gotF, gotL, c.wantFull, c.wantLight)
		}
	}
}
