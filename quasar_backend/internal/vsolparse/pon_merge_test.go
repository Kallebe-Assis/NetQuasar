package vsolparse

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestOnlineOfflineByPon_onlyExplicit48(t *testing.T) {
	vars := []probing.SNMPVar{
		{OID: oidBase + ".4.1.8.1.1", Value: "1"},
		{OID: oidBase + ".4.1.8.1.9", Value: "0"},
		{OID: oidBase + ".4.1.8.2.12", Value: "0"},
		{OID: oidBase + ".4.1.8.2.13", Value: "1"},
	}
	on, off := OnlineOfflineByPon(vars)
	if on[1] != 1 || off[1] != 1 {
		t.Fatalf("pon1 on=%d off=%d want 1/1", on[1], off[1])
	}
	if on[2] != 1 || off[2] != 1 {
		t.Fatalf("pon2 on=%d off=%d want 1/1", on[2], off[2])
	}
}

func TestAttachOnlineOfflineToIfPons(t *testing.T) {
	ifPons := []map[string]any{
		{"id": "01", "name": "GPON0/1", "onu_total": 99, "onu_online": 50, "onu_offline": 49},
	}
	on := map[int]int{1: 91}
	off := map[int]int{1: 2}
	out := AttachOnlineOfflineToIfPons(ifPons, on, off)
	if pickInt(out[0], "onu_online") != 91 {
		t.Fatalf("online %v", out[0]["onu_online"])
	}
	if pickInt(out[0], "onu_offline") != 2 {
		t.Fatalf("offline %v", out[0]["onu_offline"])
	}
	if pickInt(out[0], "onu_no_status") != 6 {
		t.Fatalf("no_status %v", out[0]["onu_no_status"])
	}
}
