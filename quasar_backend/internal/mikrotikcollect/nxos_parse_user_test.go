package mikrotikcollect

import "testing"

func TestParseNxosTransceiver_UserSampleWithAlarm(t *testing.T) {
	raw := `
Ethernet1/7
    transceiver is not present

Ethernet1/12
    transceiver is present
    type is 10Gbase-LR
    name is OEM
    part number is SFP+ WDM13-R20
    revision is A
    serial number is ASTF2304182775
    nominal bitrate is 10300 MBit/sec
    Link length supported for 9/125um fiber is 20 km
    cisco id is --
    cisco extended id number is 4

           SFP Detail Diagnostics Information (internal calibration)
  ----------------------------------------------------------------------------
                Current              Alarms                  Warnings
                Measurement     High        Low         High          Low
  ----------------------------------------------------------------------------
  Temperature   43.23 C        80.00 C    -10.00 C     75.00 C       -5.00 C
  Voltage        3.23 V         3.79 V      2.79 V      3.70 V        2.90 V
  Current       38.33 mA       80.00 mA     5.00 mA    75.00 mA       6.00 mA
  Tx Power       -0.12 dBm       4.99 dBm   -5.00 dBm    3.99 dBm     -4.00 dBm
  Rx Power        0.48 dBm  +    0.99 dBm  -18.23 dBm    0.00 dBm    -17.21 dBm
  ----------------------------------------------------------------------------
  Note: ++  high-alarm; +  high-warning; --  low-alarm; -  low-warning

Ethernet1/13
    transceiver is not present
`
	rows := ParseNxosTransceiverDetails(raw)
	if len(rows) != 1 {
		t.Fatalf("want 1 present module, got %d %#v", len(rows), rows)
	}
	r := rows[0]
	if r["interface"] != "Ethernet1/12" {
		t.Fatalf("iface=%v", r["interface"])
	}
	if r["sfp-tx-power"] != "-0.12" {
		t.Fatalf("tx=%v", r["sfp-tx-power"])
	}
	if r["sfp-rx-power"] != "0.48" {
		t.Fatalf("rx=%v want 0.48", r["sfp-rx-power"])
	}
	rx := ExtractNxosTransceiverField(raw, "sfp-rx-power").([]map[string]any)
	if len(rx) != 1 || rx[0]["sfp-rx-power"] != "0.48" {
		t.Fatalf("extract %#v", rx)
	}
}
