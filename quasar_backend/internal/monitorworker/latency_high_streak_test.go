package monitorworker

import "testing"

func TestLatencyHighStreakAfter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		prev, want int
		isHigh     bool
	}{
		{0, 1, true},
		{1, 2, true},
		{2, 3, true},
		{3, 4, true},
		{2, 0, false},
		{0, 0, false},
	}
	for _, c := range cases {
		got := latencyHighStreakAfter(c.prev, c.isHigh)
		if got != c.want {
			t.Fatalf("prev=%d isHigh=%v: got %d want %d", c.prev, c.isHigh, got, c.want)
		}
	}
}

func TestLatencyHighConsecutiveRequired(t *testing.T) {
	if latencyHighConsecutiveRequired != 3 {
		t.Fatalf("expected 3 consecutive readings, got %d", latencyHighConsecutiveRequired)
	}
}
