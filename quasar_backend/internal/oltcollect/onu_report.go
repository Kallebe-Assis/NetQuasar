package oltcollect

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	gponOnuInterfaceRE = regexp.MustCompile(`gpon_onu-\d+/\d+/\d+:\d+`)
	gponOnuPonOnuRE    = regexp.MustCompile(`(\d+):(\d+)\s*$`)
)

// OnuReportConfig comandos telnet para relatório individual de uma ONU.
// Placeholders ONU: {pon}, {onu}, {serial}, {if_index}, {gpon_onu}, {onu_if}
// Placeholders credenciais (pré-comandos): {enable}, {enable_password}, {password}, {telnet_password}
type OnuReportConfig struct {
	Enabled           bool     `json:"enabled"`
	MonitorOnlineOnly bool     `json:"monitor_online_only"`
	MaxOnusPerCycle   int      `json:"max_onus_per_cycle"`
	PreCommands          []string `json:"pre_commands"`
	Command              string   `json:"command"`
	Commands             []string `json:"commands"`
	SerialSearchCommand  string   `json:"serial_search_command"`
}

// MonitorEnabled indica se o monitoramento deve enriquecer ONUs via telnet.
func (c OnuReportConfig) MonitorEnabled() bool {
	return c.Enabled && c.HasCommands()
}

// EffectiveMaxOnus limite por ciclo (0 = 25).
func (c OnuReportConfig) EffectiveMaxOnus() int {
	if c.MaxOnusPerCycle <= 0 {
		return 25
	}
	if c.MaxOnusPerCycle > 200 {
		return 200
	}
	return c.MaxOnusPerCycle
}

func ParseOnuReportConfig(raw []byte) OnuReportConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return OnuReportConfig{}
	}
	var cfg OnuReportConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return OnuReportConfig{}
	}
	cfg.PreCommands = trimNonEmpty(cfg.PreCommands)
	if len(cfg.Commands) > 0 {
		out := make([]string, 0, len(cfg.Commands))
		for _, c := range cfg.Commands {
			if t := strings.TrimSpace(c); t != "" {
				out = append(out, t)
			}
		}
		cfg.Commands = out
	}
	cfg.Command = strings.TrimSpace(cfg.Command)
	cfg.SerialSearchCommand = strings.TrimSpace(cfg.SerialSearchCommand)
	return cfg
}

func (c OnuReportConfig) HasCommands() bool {
	return c.Command != "" || len(c.Commands) > 0
}

func (c OnuReportConfig) NeedsGponOnu() bool {
	for _, tpl := range c.allTemplates() {
		if strings.Contains(tpl, "{gpon_onu}") || strings.Contains(tpl, "{onu_if}") {
			return true
		}
	}
	return false
}

func (c OnuReportConfig) allTemplates() []string {
	if len(c.Commands) > 0 {
		return c.Commands
	}
	if c.Command != "" {
		return []string{c.Command}
	}
	return nil
}

type OnuReportTarget struct {
	Pon     int
	Onu     int
	Serial  string
	IfIndex int
	IfName  string
	GponOnu string
}

type TelnetSecrets struct {
	Password string
	Enable   string
}

func SubstituteTelnetTemplate(tpl string, t OnuReportTarget, sec TelnetSecrets) string {
	s := SubstituteOnuReportTemplate(tpl, t)
	s = strings.ReplaceAll(s, "{enable}", strings.TrimSpace(sec.Enable))
	s = strings.ReplaceAll(s, "{enable_password}", strings.TrimSpace(sec.Enable))
	s = strings.ReplaceAll(s, "{telnet_enable}", strings.TrimSpace(sec.Enable))
	s = strings.ReplaceAll(s, "{password}", strings.TrimSpace(sec.Password))
	s = strings.ReplaceAll(s, "{telnet_password}", strings.TrimSpace(sec.Password))
	return strings.TrimSpace(s)
}

func (c OnuReportConfig) RenderPreCommands(t OnuReportTarget, sec TelnetSecrets) []string {
	var out []string
	for _, tpl := range c.PreCommands {
		if cmd := SubstituteTelnetTemplate(tpl, t, sec); cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func (c OnuReportConfig) RenderCommands(t OnuReportTarget, sec TelnetSecrets) []string {
	var cmds []string
	if len(c.Commands) > 0 {
		for _, tpl := range c.Commands {
			if cmd := SubstituteTelnetTemplate(tpl, t, sec); cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
		return cmds
	}
	if c.Command != "" {
		if cmd := SubstituteTelnetTemplate(c.Command, t, sec); cmd != "" {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func ParseGponOnuFromOutput(output string) string {
	if m := gponOnuInterfaceRE.FindString(output); m != "" {
		return m
	}
	return ""
}

// ParsePonOnuFromGponOnu extrai PON e ONU de interfaces como gpon_onu-1/1/9:80.
func ParsePonOnuFromGponOnu(gpon string) (pon, onu int) {
	m := gponOnuPonOnuRE.FindStringSubmatch(strings.TrimSpace(gpon))
	if len(m) < 3 {
		return 0, 0
	}
	pon, _ = strconv.Atoi(m[1])
	onu, _ = strconv.Atoi(m[2])
	return pon, onu
}

func (c OnuReportConfig) DefaultSerialSearchCommand() string {
	if tpl := strings.TrimSpace(c.SerialSearchCommand); tpl != "" {
		return tpl
	}
	return "show gpon onu by sn {serial}"
}

func (c OnuReportConfig) RenderSerialSearchCommand(t OnuReportTarget, sec TelnetSecrets) string {
	return SubstituteTelnetTemplate(c.DefaultSerialSearchCommand(), t, sec)
}

func ResolveGponOnu(t OnuReportTarget) string {
	if g := strings.TrimSpace(t.GponOnu); g != "" {
		return g
	}
	ifName := strings.TrimSpace(t.IfName)
	if ifName != "" {
		if g := ParseGponOnuFromOutput(ifName); g != "" {
			return g
		}
		if strings.HasPrefix(strings.ToLower(ifName), "gpon") {
			return ifName
		}
	}
	if t.Pon > 0 && t.Onu > 0 {
		return fmt.Sprintf("gpon_onu-1/1/%d:%d", t.Pon, t.Onu)
	}
	return ""
}

func SubstituteOnuReportTemplate(tpl string, t OnuReportTarget) string {
	gponOnu := ResolveGponOnu(t)
	s := tpl
	s = strings.ReplaceAll(s, "{pon}", strconv.Itoa(t.Pon))
	s = strings.ReplaceAll(s, "{onu}", strconv.Itoa(t.Onu))
	s = strings.ReplaceAll(s, "{serial}", strings.TrimSpace(t.Serial))
	s = strings.ReplaceAll(s, "{gpon_onu}", gponOnu)
	s = strings.ReplaceAll(s, "{onu_if}", gponOnu)
	if t.IfIndex > 0 {
		s = strings.ReplaceAll(s, "{if_index}", strconv.Itoa(t.IfIndex))
	} else {
		s = strings.ReplaceAll(s, "{if_index}", "")
	}
	return strings.TrimSpace(s)
}

func (c OnuReportConfig) NeedsEnablePassword() bool {
	for _, tpl := range c.PreCommands {
		t := strings.TrimSpace(tpl)
		if strings.EqualFold(t, "enable") || t == "{enable}" || t == "{enable_password}" || t == "{telnet_enable}" {
			return true
		}
	}
	return false
}

func trimNonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
