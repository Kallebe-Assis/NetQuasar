package vsolparse

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestOnuOnlineFromSta(t *testing.T) {
	if !OnuOnlineFromSta(1) {
		t.Fatal("1 must be online")
	}
	for _, st := range []int{0, 2, 3, fieldUnset} {
		if OnuOnlineFromSta(st) {
			t.Fatalf("%d must not be online", st)
		}
	}
}

func TestOnlineOfflineByPon_sta2IsOffline(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: oidBase + ".4.1.8.1.1", Value: "1"},
		{OID: oidBase + ".4.1.8.1.2", Value: "2"},
	}
	on, off := OnlineOfflineByPon(vars)
	if on[1] != 1 || off[1] != 1 {
		t.Fatalf("pon1 on=%d off=%d want 1/1", on[1], off[1])
	}
}

func TestFormatUptimeDisplay_vsolString(t *testing.T) {
	raw := `"92 Days 20 Hours 28 Minutes 53 Seconds"`
	got := FormatUptimeDisplay(raw)
	if got != "92 Days 20 Hours 28 Minutes 53 Seconds" {
		t.Fatalf("got %q", got)
	}
}

func TestUptimeMinutesFromValue_vsolString(t *testing.T) {
	m, ok := UptimeMinutesFromValue("92 Days 20 Hours 28 Minutes 53 Seconds")
	if !ok || m < 133000 {
		t.Fatalf("minutes=%v ok=%v", m, ok)
	}
}
