package alertthresholds

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
)

func TestPonOperIsUp_LinkPreferred(t *testing.T) {
	t.Parallel()
	// Link SNMP down deve prevalecer mesmo com status UI "up" residual.
	row := map[string]any{
		"id":               "01",
		"name":             "GPON0/1",
		"status":           "up",
		"link_oper_status": "down",
		"onu_online":       5,
	}
	up, ok := oltifderive.PonOperIsUp(row)
	if !ok || up {
		t.Fatalf("expected link down => not up, got up=%v ok=%v", up, ok)
	}
}

func TestPonOperIsUp_PonUpDown(t *testing.T) {
	t.Parallel()
	up, ok := oltifderive.PonOperIsUp(map[string]any{"status": "pon_up"})
	if !ok || !up {
		t.Fatalf("pon_up: up=%v ok=%v", up, ok)
	}
	up, ok = oltifderive.PonOperIsUp(map[string]any{"status": "pon_down"})
	if !ok || up {
		t.Fatalf("pon_down: up=%v ok=%v", up, ok)
	}
}
