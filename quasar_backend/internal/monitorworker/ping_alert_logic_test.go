package monitorworker

import "testing"

func TestShouldOpenPingUnreachableAlert(t *testing.T) {
	th := 3
	cases := []struct {
		name        string
		reachOK     bool
		streakAfter int
		want        bool
	}{
		{"ok reachable", true, 0, false},
		{"first fail below threshold", false, 1, false},
		{"second fail below threshold", false, 2, false},
		{"third fail opens alert", false, 3, true},
		{"continued fail keeps eligible", false, 5, true},
		{"threshold one still needs three fails", false, 1, false},
		{"threshold one opens on third fail", false, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thUse := th
			if tc.name == "threshold one first fail" {
				thUse = 1
			}
			got := shouldOpenPingUnreachableAlert(tc.reachOK, tc.streakAfter, thUse)
			if got != tc.want {
				t.Fatalf("reachOK=%v streak=%d th=%d => got %v want %v", tc.reachOK, tc.streakAfter, thUse, got, tc.want)
			}
		})
	}
}
