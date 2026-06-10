package oltifderive

import "testing"

func TestApplyPonOperStatusFromOnuCounts(t *testing.T) {
	on := map[string]any{"onu_online": 3, "status": "snmp_metrics"}
	if !ApplyPonOperStatusFromOnuCounts(on) {
		t.Fatal("expected apply ok")
	}
	if on["status"] != "up" || on["pon_oper_status"] != "up" {
		t.Fatalf("want up, got status=%v pon=%v", on["status"], on["pon_oper_status"])
	}
	off := map[string]any{"onu_online": 0}
	ApplyPonOperStatusFromOnuCounts(off)
	if off["status"] != "down" {
		t.Fatalf("want down, got %v", off["status"])
	}
	if ApplyPonOperStatusFromOnuCounts(map[string]any{"name": "PON1"}) {
		t.Fatal("expected false without onu_online")
	}
}
