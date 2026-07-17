package ifaceoptical

import (
	"encoding/json"
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

func TestPortsFromTelnetNxos(t *testing.T) {
	out := mikrotikcollect.TelnetCollectOutput{
		Fields: map[string]mikrotikcollect.TelnetFieldResult{
			"telnet_sfp_rx_power": {
				OK: true,
				Value: []map[string]any{
					{"interface": "Ethernet1/23", "sfp-rx-power": "-2.44"},
				},
			},
			"telnet_sfp_tx_power": {
				OK: true,
				Value: []map[string]any{
					{"interface": "Ethernet1/23", "sfp-tx-power": "-2.17"},
				},
			},
		},
	}
	ports := PortsFromTelnet(out)
	if len(ports) != 1 {
		t.Fatalf("ports=%d %#v", len(ports), ports)
	}
	if ports[0].RxDBm == nil || *ports[0].RxDBm != -2.44 {
		t.Fatalf("rx %#v", ports[0].RxDBm)
	}
	if ports[0].TxDBm == nil || *ports[0].TxDBm != -2.17 {
		t.Fatalf("tx %#v", ports[0].TxDBm)
	}
	rows := []snmpifparse.IfRow{
		{IfIndex: 50, IfName: "Eth1/23", DisplayName: "Eth1/23", Descr: "Eth1/23"},
	}
	ports = ResolveIfIndexes(ports, rows)
	if ports[0].IfIndex != 50 {
		t.Fatalf("ifIndex=%d", ports[0].IfIndex)
	}
	opt := MergeIntoOpticalMap(nil, ports)
	if opt[50].TxDBm == nil || *opt[50].TxDBm != -2.17 {
		t.Fatalf("merged %#v", opt[50])
	}
	_ = snmpmikrotik.OpticalPower{}
}

func TestAppendMetaRoundTrip(t *testing.T) {
	tx, rx := -2.17, -2.44
	arr := []map[string]any{
		{"oid": "1.3.6.1.2.1.2.2.1.2.1", "value": "Eth1/1", "type": "OCTET STRING"},
	}
	arr = AppendMeta(arr, []Port{{IfIndex: 50, Name: "Ethernet1/23", TxDBm: &tx, RxDBm: &rx}})
	b, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	got := ParseMetaFromWalkJSON(b)
	if len(got) != 1 || got[0].IfIndex != 50 || got[0].TxDBm == nil || *got[0].TxDBm != -2.17 {
		t.Fatalf("got %#v", got)
	}
}
