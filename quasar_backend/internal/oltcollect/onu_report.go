package oltcollect

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// vsolGponInterfaceRE detecta linhas "interface gpon …" nos pré-comandos.
var vsolGponInterfaceRE = regexp.MustCompile(`(?i)^interface\s+gpon\s+`)

var (
	gponOnuInterfaceRE = regexp.MustCompile(`gpon_onu-\d+/\d+/\d+:\d+`)
	gponOnuPonOnuRE    = regexp.MustCompile(`(\d+):(\d+)\s*$`)
)

// OnuReportConfig comandos telnet para relatório individual de uma ONU.
// Placeholders ONU: {pon}, {onu}, {serial}, {if_index}, {gpon_onu}, {onu_if}, {vlan}, {onu_type}, {name}
// Placeholders credenciais (pré-comandos): {enable}, {enable_password}, {password}, {telnet_password}
type OnuReportConfig struct {
	Enabled           bool     `json:"enabled"`
	MonitorOnlineOnly bool     `json:"monitor_online_only"`
	MaxOnusPerCycle   int      `json:"max_onus_per_cycle"`
	PreCommands          []string `json:"pre_commands"`
	Command              string   `json:"command"`
	Commands             []string `json:"commands"`
	SerialSearchCommand      string   `json:"serial_search_command"`
	SerialListSearchCommand  string   `json:"serial_list_search_command"`
	OnuAuthorizeCommand             string   `json:"onu_authorize_command"`
	OnuDeauthorizeCommand           string   `json:"onu_deauthorize_command"`
	UnauthorizedOnuQueryCommand     string   `json:"unauthorized_onu_query_command"`
	UnauthorizedOnuPreCommands      []string `json:"unauthorized_onu_pre_commands"`
	// Valores por omissão para placeholders de autorização (ex. ZTE).
	AuthorizeVlan    string `json:"authorize_vlan,omitempty"`
	AuthorizeOnuType string `json:"authorize_onu_type,omitempty"`
	AuthorizeName    string `json:"authorize_name,omitempty"`
	// Catálogo SNMP de VLANs por PON (configurável no perfil).
	AuthorizeVlanSnmpOID string                      `json:"authorize_vlan_snmp_oid,omitempty"`
	AuthorizeVlanCatalog []AuthorizeVlanCatalogEntry `json:"authorize_vlan_catalog,omitempty"`
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
	cfg.SerialListSearchCommand = strings.TrimSpace(cfg.SerialListSearchCommand)
	cfg.OnuAuthorizeCommand = strings.TrimSpace(cfg.OnuAuthorizeCommand)
	cfg.OnuDeauthorizeCommand = strings.TrimSpace(cfg.OnuDeauthorizeCommand)
	cfg.UnauthorizedOnuQueryCommand = strings.TrimSpace(cfg.UnauthorizedOnuQueryCommand)
	cfg.UnauthorizedOnuPreCommands = trimNonEmpty(cfg.UnauthorizedOnuPreCommands)
	cfg.AuthorizeVlan = strings.TrimSpace(cfg.AuthorizeVlan)
	cfg.AuthorizeOnuType = strings.TrimSpace(cfg.AuthorizeOnuType)
	cfg.AuthorizeName = strings.TrimSpace(cfg.AuthorizeName)
	cfg.AuthorizeVlanSnmpOID = strings.TrimSpace(cfg.AuthorizeVlanSnmpOID)
	if len(cfg.AuthorizeVlanCatalog) > 0 {
		out := make([]AuthorizeVlanCatalogEntry, 0, len(cfg.AuthorizeVlanCatalog))
		for _, e := range cfg.AuthorizeVlanCatalog {
			if e.VID <= 0 {
				continue
			}
			e.Name = strings.TrimSpace(e.Name)
			e.Description = strings.TrimSpace(e.Description)
			if e.Pon <= 0 {
				e.Pon = PonFromZteVlanDescription(e.Description)
			}
			out = append(out, e)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].VID < out[j].VID })
		cfg.AuthorizeVlanCatalog = out
	}
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
	Vlan    string // {vlan}
	OnuType string // {onu_type}
	Name    string // {name}
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
	return renderTelnetPreTemplates(c.PreCommands, t, sec)
}

// RenderUnauthorizedPreCommands pré-comandos exclusivos da consulta de ONUs não autorizadas.
func (c OnuReportConfig) RenderUnauthorizedPreCommands(t OnuReportTarget, sec TelnetSecrets) []string {
	return renderTelnetPreTemplates(c.UnauthorizedOnuPreCommands, t, sec)
}

func renderTelnetPreTemplates(tpls []string, t OnuReportTarget, sec TelnetSecrets) []string {
	var out []string
	for _, tpl := range tpls {
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

// ResolveGponOnuFromSerialSearchOutput tenta listagem ou lookup directo na saída telnet.
func ResolveGponOnuFromSerialSearchOutput(output, serial string, listMode bool) string {
	if listMode {
		matches := FilterSerialSearchEntries(ParseOnuListFromTelnetOutput(output), serial, 0)
		if len(matches) > 0 {
			if g := strings.TrimSpace(matches[0].GponOnu); g != "" {
				return g
			}
			if matches[0].Pon > 0 && matches[0].Onu > 0 {
				return fmt.Sprintf("gpon_onu-1/1/%d:%d", matches[0].Pon, matches[0].Onu)
			}
		}
		return ""
	}
	return ParseGponOnuFromOutput(output)
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

// ListSerialSearchCommand devolve o template de listagem por PON (quando existir).
func (c OnuReportConfig) ListSerialSearchCommand() string {
	if tpl := strings.TrimSpace(c.SerialListSearchCommand); tpl != "" {
		return tpl
	}
	if c.SerialSearchUsesPonPlaceholder() {
		return c.DefaultSerialSearchCommand()
	}
	return ""
}

func (c OnuReportConfig) RenderListSerialSearchCommand(t OnuReportTarget, sec TelnetSecrets) string {
	tpl := c.ListSerialSearchCommand()
	if tpl == "" {
		return ""
	}
	tmp := c
	tmp.SerialSearchCommand = tpl
	return tmp.RenderSerialSearchCommand(t, sec)
}

func (c OnuReportConfig) RenderSerialSearchCommand(t OnuReportTarget, sec TelnetSecrets) string {
	return SubstituteTelnetTemplate(c.DefaultSerialSearchCommand(), t, sec)
}

func ResolveGponOnu(t OnuReportTarget) string {
	if g := strings.TrimSpace(t.GponOnu); looksLikeGponOnuInterface(g) {
		return g
	}
	ifName := strings.TrimSpace(t.IfName)
	if looksLikeGponOnuInterface(ifName) {
		return ifName
	}
	if ifName != "" {
		if g := ParseGponOnuFromOutput(ifName); looksLikeGponOnuInterface(g) {
			return g
		}
	}
	if t.Pon > 0 && t.Onu > 0 {
		return fmt.Sprintf("gpon_onu-1/1/%d:%d", t.Pon, t.Onu)
	}
	return ""
}

// looksLikeGponOnuInterface distingue gpon_onu-…:N (ONU) de gpon_olt-… (porta OLT).
func looksLikeGponOnuInterface(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "gpon_olt") || strings.Contains(lower, "gpon-olt") {
		return false
	}
	return gponOnuInterfaceRE.MatchString(s) ||
		strings.HasPrefix(lower, "gpon_onu-") ||
		strings.HasPrefix(lower, "gpon-onu_")
}

// ApplyAuthorizeTemplateDefaults preenche {onu_type}/{name}.
// VLAN não recebe fallback aqui: deve vir do catálogo PON ou SNMP.
// Tipo ONU: sempre GU201-G no script operativo (independente do modelo reportado).
func ApplyAuthorizeTemplateDefaults(t OnuReportTarget, cfg OnuReportConfig) OnuReportTarget {
	t.OnuType = "GU201-G"
	if strings.TrimSpace(t.Name) == "" {
		if t.Pon > 0 && t.Onu > 0 {
			t.Name = fmt.Sprintf("%d-%d", t.Pon, t.Onu)
		}
	}
	return t
}

func SubstituteOnuReportTemplate(tpl string, t OnuReportTarget) string {
	gponOnu := ResolveGponOnu(t)
	s := tpl
	if t.Pon > 0 {
		s = strings.ReplaceAll(s, "{pon_port}", fmt.Sprintf("0/%d", t.Pon))
	}
	s = strings.ReplaceAll(s, "{pon}", strconv.Itoa(t.Pon))
	s = strings.ReplaceAll(s, "{onu}", strconv.Itoa(t.Onu))
	s = strings.ReplaceAll(s, "{serial}", strings.TrimSpace(t.Serial))
	s = strings.ReplaceAll(s, "{gpon_onu}", gponOnu)
	s = strings.ReplaceAll(s, "{onu_if}", gponOnu)
	s = strings.ReplaceAll(s, "{vlan}", strings.TrimSpace(t.Vlan))
	s = strings.ReplaceAll(s, "{onu_type}", strings.TrimSpace(t.OnuType))
	s = strings.ReplaceAll(s, "{name}", strings.TrimSpace(t.Name))
	if t.IfIndex > 0 {
		s = strings.ReplaceAll(s, "{if_index}", strconv.Itoa(t.IfIndex))
	} else {
		s = strings.ReplaceAll(s, "{if_index}", "")
	}
	s = normalizeVsolGponInterfaceCmd(strings.TrimSpace(s), t.Pon)
	return strings.TrimSpace(s)
}

// normalizeVsolGponInterfaceCmd corrige "interface gpon 4" → "interface gpon 0/4" (VSOL).
func normalizeVsolGponInterfaceCmd(cmd string, pon int) string {
	if pon <= 0 {
		return cmd
	}
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if !strings.HasPrefix(lower, "interface gpon ") {
		return cmd
	}
	suffix := strings.TrimSpace(cmd[len("interface gpon "):])
	want := fmt.Sprintf("0/%d", pon)
	if suffix == want {
		return cmd
	}
	if suffix == strconv.Itoa(pon) || suffix == "" {
		return "interface gpon " + want
	}
	return cmd
}

func (c OnuReportConfig) NeedsEnablePassword() bool {
	return preCommandsNeedEnable(c.PreCommands)
}

// NeedsEnablePasswordForUnauthorized indica se a consulta de não autorizadas exige senha enable.
func (c OnuReportConfig) NeedsEnablePasswordForUnauthorized() bool {
	return preCommandsNeedEnable(c.UnauthorizedOnuPreCommands)
}

func preCommandsNeedEnable(pre []string) bool {
	for _, tpl := range pre {
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
