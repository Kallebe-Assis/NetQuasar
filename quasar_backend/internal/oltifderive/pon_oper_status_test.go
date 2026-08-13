package oltifderive

import "testing"

func TestApplyPonOperStatusFromOnuCounts(t *testing.T) {
	on := map[string]any{"onu_online": 3, "status": "pon_up"}
	if !ApplyPonOperStatusFromOnuCounts(on) {
		t.Fatal("expected apply ok")
	}
	if on["status"] != "up" || on["pon_oper_status"] != "up" {
		t.Fatalf("want up, got status=%v pon=%v", on["status"], on["pon_oper_status"])
	}
	if on["link_oper_status"] != "up" {
		t.Fatalf("want link_oper_status=up, got %v", on["link_oper_status"])
	}
	off := map[string]any{"onu_online": 0, "status": "pon_down"}
	ApplyPonOperStatusFromOnuCounts(off)
	if off["status"] != "down" {
		t.Fatalf("want down, got %v", off["status"])
	}
	if off["link_oper_status"] != "down" {
		t.Fatalf("want link_oper_status=down, got %v", off["link_oper_status"])
	}
	// Link SNMP down prevalece sobre ONUs online no PonOperIsUp.
	ghost := map[string]any{"onu_online": 5, "status": "pon_down"}
	ApplyPonOperStatusFromOnuCounts(ghost)
	if ghost["status"] != "up" {
		t.Fatalf("UI status from ONU counts should be up, got %v", ghost["status"])
	}
	up, ok := PonOperIsUp(ghost)
	if !ok || up {
		t.Fatalf("link down should force PonOperIsUp=false, got up=%v ok=%v", up, ok)
	}
	if ApplyPonOperStatusFromOnuCounts(map[string]any{"name": "PON1"}) {
		t.Fatal("expected false without onu_online")
	}
}

func TestPonLooksUpOnOltPageIgnoresLinkDownWhenOnusOnline(t *testing.T) {
	ghost := map[string]any{"onu_online": 5, "status": "pon_down"}
	ApplyPonOperStatusFromOnuCounts(ghost)
	up, ok := PonLooksUpOnOltPage(ghost)
	if !ok || !up {
		t.Fatalf("OLT page should treat ONU>=1 as UP, got up=%v ok=%v", up, ok)
	}
	off := map[string]any{"onu_online": 0, "status": "up"}
	ApplyPonOperStatusFromOnuCounts(off)
	up, ok = PonLooksUpOnOltPage(off)
	if !ok || up {
		t.Fatalf("OLT page should treat 0 ONUs as DOWN, got up=%v ok=%v", up, ok)
	}
}
