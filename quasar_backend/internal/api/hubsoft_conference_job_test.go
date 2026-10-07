package api

import "testing"

// O progresso da conferência anda de 5 em 5, nunca regride e só chega a 100 quando termina.
func TestHubsoftConferenceJobProgressSteps(t *testing.T) {
	j := &hubsoftConferenceJob{Status: "running"}
	for _, c := range []struct{ in, want int }{{0, 0}, {3, 0}, {4, 0}, {5, 5}, {12, 10}, {9, 10}, {47, 45}, {46, 45}, {99, 95}, {100, 95}} {
		j.setProgress(c.in, "")
		if j.Percent != c.want {
			t.Fatalf("setProgress(%d) → %d, esperado %d", c.in, j.Percent, c.want)
		}
	}
}
