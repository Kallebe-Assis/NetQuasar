package monitorworker

import (
	"strings"
	"testing"
)

func TestSQLDeviceEligibleForPingAlertsCriteria(t *testing.T) {
	for _, frag := range []string{
		SQLDeviceEligibleForPingAlerts,
		SQLDeviceEligibleForPingAlertsByID("a.device_id"),
	} {
		if !strings.Contains(frag, "ping_enabled") {
			t.Fatalf("missing ping_enabled in %q", frag)
		}
		if !strings.Contains(frag, "operational_mode") || !strings.Contains(frag, "'Ativo'") {
			t.Fatalf("missing Ativo in %q", frag)
		}
		if !strings.Contains(frag, "network_status") || !strings.Contains(frag, "'Normal'") {
			t.Fatalf("missing Normal in %q", frag)
		}
	}
}
