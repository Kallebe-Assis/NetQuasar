package ifaceoptical

import (
	"encoding/json"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

func TestApplyPortsToOpticalMap_EthAliasAndPortKey(t *testing.T) {
	tx, rx := -0.12, 0.48
	ports := []Port{{
		Name:  "Ethernet1/12",
		TxDBm: &tx,
		RxDBm: &rx,
	}}
	rows := []snmpifparse.IfRow{
		{IfIndex: 436207616, IfName: "Eth1/12", DisplayName: "Eth1/12", Descr: "Ethernet1/12"},
		{IfIndex: 436207744, IfName: "Eth1/13", DisplayName: "Eth1/13"},
	}
	opt := ApplyPortsToOpticalMap(nil, ports, rows)
	op, ok := opt[436207616]
	if !ok {
		t.Fatal("expected optical on Eth1/12 ifIndex")
	}
	if op.TxDBm == nil || *op.TxDBm != tx {
		t.Fatalf("tx=%v want %v", op.TxDBm, tx)
	}
	if op.RxDBm == nil || *op.RxDBm != rx {
		t.Fatalf("rx=%v want %v", op.RxDBm, rx)
	}
}

func TestPortsFromTelnetCollectionJSON_SwitchWrap(t *testing.T) {
	doc := map[string]any{
		"switch_telnet_collection": map[string]any{
			"fields": map[string]any{
				"telnet_sfp_tx_power": map[string]any{
					"ok": true,
					"value": []map[string]any{
						{"interface": "Ethernet1/12", "sfp-tx-power": "-0.12"},
					},
				},
				"telnet_sfp_rx_power": map[string]any{
					"ok": true,
					"value": []map[string]any{
						{"interface": "Ethernet1/12", "sfp-rx-power": "0.48"},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(doc)
	ports := PortsFromTelnetCollectionJSON(b)
	if len(ports) != 1 {
		t.Fatalf("ports=%d want 1: %+v", len(ports), ports)
	}
	if ports[0].TxDBm == nil || *ports[0].TxDBm != -0.12 {
		t.Fatalf("tx=%v", ports[0].TxDBm)
	}
	if ports[0].RxDBm == nil || *ports[0].RxDBm != 0.48 {
		t.Fatalf("rx=%v", ports[0].RxDBm)
	}
}
