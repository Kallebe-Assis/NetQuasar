package monitorworker

import "testing"

func TestPingOfflineConfirmedAndCacheReachOK(t *testing.T) {
	th := 3
	cases := []struct {
		name        string
		probeOK     bool
		streak      int
		wantOffline bool
		wantCacheOK bool
	}{
		{"probe ok", true, 0, false, true},
		{"first fail not confirmed", false, 1, false, true},
		{"second fail not confirmed", false, 2, false, true},
		{"third fail confirmed", false, 3, true, false},
		{"continued fail", false, 5, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotOff := pingOfflineConfirmed(tc.probeOK, tc.streak, th)
			if gotOff != tc.wantOffline {
				t.Fatalf("offline: got %v want %v", gotOff, tc.wantOffline)
			}
			gotCache := cacheReachOK(tc.probeOK, tc.streak, th)
			if gotCache != tc.wantCacheOK {
				t.Fatalf("cacheReachOK: got %v want %v", gotCache, tc.wantCacheOK)
			}
		})
	}
}
