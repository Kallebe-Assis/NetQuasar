package mikrotikcollect

import "testing"

func TestParseNxosSystemUptime(t *testing.T) {
	raw := `System start time:          Sat Feb 21 20:03:43 2026
System uptime:              145 days, 0 hours, 17 minutes, 25 seconds
Kernel uptime:              145 days, 0 hours, 32 minutes, 15 seconds
Active supervisor uptime:   145 days, 0 hours, 17 minutes, 25 seconds`
	got := ParseNxosSystemUptime(raw)
	if got["uptime"] != "145d 00h 17m" {
		t.Fatalf("got %#v", got)
	}
}

func TestParseNxosInterfaceStatus(t *testing.T) {
	raw := `--------------------------------------------------------------------------------
Port          Name               Status    Vlan      Duplex  Speed   Type
--------------------------------------------------------------------------------
Eth1/1        HUAWEI-BORDA       connected trunk     full    10G     SFP-H10GB-C
Eth1/5        --                 sfpAbsent 1         full    1000    --
Eth1/29       OLT-VSOL-02        notconnec trunk     full    10G     SFP-H10GB-C
Po1           LAG-HILLSTONE      connected trunk     full    10G     --
mgmt0         --                 connected routed    full    a-1000  --
`
	rows := ParseNxosInterfaceStatus(raw)
	if len(rows) < 5 {
		t.Fatalf("want >=5 rows, got %d: %#v", len(rows), rows)
	}
	found := map[string]map[string]any{}
	for _, r := range rows {
		n, _ := r["name"].(string)
		found[n] = r
	}
	eth1 := found["Ethernet1/1"]
	if eth1 == nil {
		t.Fatalf("missing Ethernet1/1: %#v", found)
	}
	if eth1["descr"] != "HUAWEI-BORDA" || eth1["oper_status"] != "up" {
		t.Fatalf("eth1=%#v", eth1)
	}
	eth29 := found["Ethernet1/29"]
	if eth29 == nil || eth29["oper_status"] != "down" {
		t.Fatalf("eth29=%#v", eth29)
	}
	po := found["port-channel1"]
	if po == nil || po["descr"] != "LAG-HILLSTONE" {
		t.Fatalf("po=%#v", po)
	}
}

func TestParseNxosTransceiverDetails(t *testing.T) {
	raw := `Ethernet1/20
    transceiver is not present

Ethernet1/23
    transceiver is present
    type is 10Gbase-LR
    name is OEM
    part number is SFP+ 10G LR
    revision is A
    serial number is AST1807011493
    nominal bitrate is 10300 MBit/sec
    Link length supported for 9/125um fiber is 10 km
    cisco id is --
    cisco extended id number is 4

           SFP Detail Diagnostics Information (internal calibration)
  ----------------------------------------------------------------------------
                Current              Alarms                  Warnings
                Measurement     High        Low         High          Low
  ----------------------------------------------------------------------------
  Temperature   41.70 C        80.00 C    -10.00 C     75.00 C       -5.00 C
  Voltage        3.23 V         3.79 V      2.79 V      3.70 V        2.90 V
  Current       35.09 mA       80.00 mA     5.00 mA    75.00 mA       6.00 mA
  Tx Power       -2.17 dBm       0.99 dBm   -7.01 dBm    0.49 dBm     -6.00 dBm
  Rx Power       -2.44 dBm       0.99 dBm  -17.21 dBm    0.00 dBm    -16.02 dBm
  ----------------------------------------------------------------------------
  Note: ++  high-alarm; +  high-warning; --  low-alarm; -  low-warning

Ethernet1/24
    transceiver is not present
`
	rows := ParseNxosTransceiverDetails(raw)
	if len(rows) != 1 {
		t.Fatalf("want 1 present module, got %d %#v", len(rows), rows)
	}
	r := rows[0]
	if r["interface"] != "Ethernet1/23" {
		t.Fatalf("iface=%v", r["interface"])
	}
	if r["sfp-tx-power"] != "-2.17" || r["sfp-rx-power"] != "-2.44" {
		t.Fatalf("powers %#v", r)
	}
	if r["sfp-temperature"] != "41.70" || r["sfp-supply-voltage"] != "3.23" {
		t.Fatalf("temp/volt %#v", r)
	}
	if r["sfp-vendor-name"] != "OEM" || r["sfp-serial"] != "AST1807011493" {
		t.Fatalf("vendor %#v", r)
	}
	rx := ExtractNxosTransceiverField(raw, "sfp-rx-power").([]map[string]any)
	if len(rx) != 1 || rx[0]["sfp-rx-power"] != "-2.44" {
		t.Fatalf("rx extract %#v", rx)
	}
}

func TestNormalizeNxosIfName(t *testing.T) {
	if got := NormalizeNxosIfName("Eth1/23"); got != "Ethernet1/23" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeNxosIfName("Po2"); got != "port-channel2" {
		t.Fatalf("got %q", got)
	}
}
