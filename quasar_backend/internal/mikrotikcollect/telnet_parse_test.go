package mikrotikcollect

import "testing"

func TestParseSystemResourceColonFormat(t *testing.T) {
	out := `/system resource print
                   uptime: 5w2d3h15m20s
                  version: 7.15.3 (stable)
                 cpu-load: 12
            free-memory: 945123456
           total-memory: 1073741824
          free-hdd-space: 12345678
         total-hdd-space: 16777216
               board-name: CCR1036-8G-2S+
                 platform: MikroTik`
	res := parseSystemResource(out)
	if got := res["uptime"]; got != "5w2d3h15m20s" {
		t.Fatalf("uptime: got %v", got)
	}
	if got := res["version"]; got != "7.15.3 (stable)" {
		t.Fatalf("version: got %v", got)
	}
	if got := res["cpu-load"]; got != "12" {
		t.Fatalf("cpu-load: got %v", got)
	}
	if got := res["free-hdd-space"]; got != "12345678" {
		t.Fatalf("free-hdd-space: got %v", got)
	}
	if got := res["board-name"]; got != "CCR1036-8G-2S+" {
		t.Fatalf("board-name: got %v", got)
	}
}

func TestParseEthernetMonitorSFP(t *testing.T) {
	out := `           status: link-ok
              rate: 10Gbps
       sfp-rx-power: -8.2dBm
       sfp-tx-power: -2.1dBm
    sfp-temperature: 42C
  sfp-supply-voltage: 3.28V
 sfp-tx-bias-current: 35mA
   sfp-vendor-name: Mikrotik
sfp-vendor-part-number: S+RJ10
          sfp-serial: ABC123`
	row := parseEthernetMonitor(out, "sfpplus1")
	if got := row["sfp-rx-power"]; got != "-8.2dBm" {
		t.Fatalf("sfp-rx-power: got %v", got)
	}
	if got := row["sfp-tx-power"]; got != "-2.1dBm" {
		t.Fatalf("sfp-tx-power: got %v", got)
	}
	if got := row["sfp-supply-voltage"]; got != "3.28V" {
		t.Fatalf("sfp-supply-voltage: got %v", got)
	}
	field := parseEthernetMonitorField(out, "sfpplus1", "sfp-rx-power").(map[string]any)
	if field["sfp-rx-power"] != "-8.2dBm" {
		t.Fatalf("field rx: got %v", field["sfp-rx-power"])
	}
}

func TestParseSystemResourceField(t *testing.T) {
	out := `uptime: 1d2h3m4s`
	v := parseSystemResourceField(out, "uptime")
	m, ok := v.(map[string]any)
	if !ok || m["uptime"] != "1d2h3m4s" {
		t.Fatalf("unexpected: %#v", v)
	}
}
