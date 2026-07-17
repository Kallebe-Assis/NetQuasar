package bngcollect

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func TestMergeSessionColumnWalkIPv6(t *testing.T) {
	const base = "1.3.6.1.4.1.2011.5.2.1.15.1.59"
	columns := map[string]map[string]string{}
	known := map[string]string{"101.7": "cliente"}
	vars := []probing.SNMPVar{
		{OID: base + ".101.7", Value: "2001:db8::10"},
		{OID: base + ".999.1", Value: "2001:db8::99"},
	}

	got := mergeSessionColumnWalk(columns, "access_ipv6", base, vars, known)
	if got != 1 {
		t.Fatalf("esperava 1 IPv6 associado, obteve %d", got)
	}
	if columns["access_ipv6"]["101.7"] != "2001:db8::10" {
		t.Fatalf("IPv6 não associado ao índice composto: %+v", columns)
	}
	if _, ok := columns["access_ipv6"]["999.1"]; ok {
		t.Fatal("índice sem login não deveria ser incluído")
	}
}

func TestProfileWithCollectModeMonitoring(t *testing.T) {
	profile := Profile{Metrics: DefaultMetrics()}
	got := ProfileWithCollectMode(profile, "monitoring")
	for _, key := range []string{"sys_uptime", "cpu_usage", "memory_usage", "temperature", "pppoe_online", "ipv4_online", "ipv6_online", "dual_stack_online"} {
		if !got.Metrics[key].Enabled {
			t.Fatalf("métrica da linha-base BNG não activada: %s", key)
		}
	}
	for _, key := range []string{"access_login", "if_mib_table"} {
		if got.Metrics[key].Enabled {
			t.Fatalf("walk pesado não deveria estar na linha-base BNG: %s", key)
		}
	}
}
