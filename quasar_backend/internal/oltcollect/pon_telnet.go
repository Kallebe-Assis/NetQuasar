package oltcollect

import (
	"encoding/json"
	"strings"
)

// PonTelnetConfig comandos telnet para métricas ópticas da PON/SFP (voltagem, TX, temperatura, bias).
// Placeholders: {pon}, {enable}, {enable_password}, {password}, {telnet_password}
type PonTelnetConfig struct {
	Enabled         bool     `json:"enabled"`
	MaxPonsPerCycle int      `json:"max_pons_per_cycle"`
	PreCommands     []string `json:"pre_commands"`
	Commands        []string `json:"commands"`
}

func ParsePonTelnetConfig(raw []byte) PonTelnetConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return PonTelnetConfig{}
	}
	var cfg PonTelnetConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return PonTelnetConfig{}
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
	return cfg
}

func (c PonTelnetConfig) MonitorEnabled() bool {
	return c.Enabled && c.HasCommands()
}

func (c PonTelnetConfig) HasCommands() bool {
	return len(c.Commands) > 0
}

func (c PonTelnetConfig) EffectiveMaxPons() int {
	if c.MaxPonsPerCycle <= 0 {
		return 16
	}
	if c.MaxPonsPerCycle > 64 {
		return 64
	}
	return c.MaxPonsPerCycle
}

func (c PonTelnetConfig) RenderPreCommands(t OnuReportTarget, sec TelnetSecrets) []string {
	var out []string
	for _, tpl := range c.PreCommands {
		if cmd := SubstituteTelnetTemplate(tpl, t, sec); cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func (c PonTelnetConfig) RenderCommands(t OnuReportTarget, sec TelnetSecrets) []string {
	var cmds []string
	for _, tpl := range c.Commands {
		if cmd := SubstituteTelnetTemplate(tpl, t, sec); cmd != "" {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func (c PonTelnetConfig) NeedsEnablePassword() bool {
	for _, tpl := range c.PreCommands {
		t := strings.TrimSpace(tpl)
		if strings.EqualFold(t, "enable") || t == "{enable}" || t == "{enable_password}" || t == "{telnet_enable}" {
			return true
		}
	}
	return false
}
