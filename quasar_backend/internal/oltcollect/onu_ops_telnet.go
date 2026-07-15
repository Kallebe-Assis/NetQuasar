package oltcollect

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// Padrões (âncora no início) para desdobrar scripts ZTE colados numa linha só.
// Ordem: mais específicos primeiro (ex.: gemport … name … antes de name …).
var mashedTelnetCmdRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^configure\s+terminal\b`),
	regexp.MustCompile(`(?i)^exit\b`),
	regexp.MustCompile(`(?i)^interface\s+\S+`),
	regexp.MustCompile(`(?i)^pon-onu-mng\s+\S+`),
	regexp.MustCompile(`(?i)^onu\s+(?:\d+|\{onu\})\s+type\s+\S+\s+sn\s+\S+`),
	regexp.MustCompile(`(?i)^no\s+onu\s+(?:\d+|\{onu\})\b`),
	regexp.MustCompile(`(?i)^sn-bind\s+enable\s+sn\b`),
	regexp.MustCompile(`(?i)^tcont\s+\d+\s+profile\s+\S+`),
	regexp.MustCompile(`(?i)^gemport\s+\d+(?:\s+name\s+\S+)?(?:\s+tcont\s+\d+)?`),
	regexp.MustCompile(`(?i)^service-port\s+\d+\s+user-vlan\s+\S+\s+vlan\s+\S+`),
	regexp.MustCompile(`(?i)^vlan\s+port\s+\S+\s+mode\s+tag\s+vlan\s+\S+`),
	regexp.MustCompile(`(?i)^service\s+\d+\s+gemport\s+\d+\s+vlan\s+\S+`),
	regexp.MustCompile(`(?i)^name\s+\S+`),
}

// OnuTelnetActionResult resultado de acção telnet pontual (autorizar, listar, etc.).
type OnuTelnetActionResult struct {
	OK          bool
	Command     string
	Commands    []string
	PonsQueried []int
	Output      string
	Error       string
	Steps       []probing.TelnetScriptStepResult
}

// RunOnuTelnetAction executa pré-comandos do perfil ONU (métricas) + um comando telnet.
func RunOnuTelnetAction(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	target OnuReportTarget,
	commandTpl string,
	timeout time.Duration,
) OnuTelnetActionResult {
	return RunOnuTelnetActionWithPre(ctx, host, user, password, enable, cfg.PreCommands, secrets, target, commandTpl, timeout)
}

// RunUnauthorizedOnuQuery executa pré-comandos e comando exclusivos de ONUs não autorizadas.
func RunUnauthorizedOnuQuery(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	timeout time.Duration,
) OnuTelnetActionResult {
	return RunOnuTelnetActionWithPre(
		ctx, host, user, password, enable,
		cfg.UnauthorizedOnuPreCommands, secrets, OnuReportTarget{},
		cfg.UnauthorizedOnuQueryCommand, timeout,
	)
}

// RunOnuTelnetActionWithPre executa os pré-comandos indicados + um comando telnet.
func RunOnuTelnetActionWithPre(
	ctx context.Context,
	host, user, password, enable string,
	preTemplates []string,
	secrets TelnetSecrets,
	target OnuReportTarget,
	commandTpl string,
	timeout time.Duration,
) OnuTelnetActionResult {
	res := OnuTelnetActionResult{}
	commandTpl = strings.TrimSpace(commandTpl)
	if commandTpl == "" {
		res.Error = "comando telnet não configurado"
		return res
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	cmd := SubstituteTelnetTemplate(commandTpl, target, secrets)
	commands := splitTelnetCommands(cmd)
	commands = ensureConfigTerminalPrefix(preTemplates, commands)
	if len(commands) == 0 {
		res.Error = "comando vazio após substituição de placeholders"
		return res
	}
	// TEMP provisório: dump do script telnet no terminal (remover quando validado).
	if looksLikeAuthorizeTelnetScript(commands) {
		logAuthorizeTelnetScript(host, target, preTemplates, commands)
	}
	preRendered := renderTelnetPreTemplates(preTemplates, target, secrets)
	script := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		PreCommands: preRendered, RawPreCommands: preTemplates,
		Commands: commands, MaxReadBytes: 320000,
	})
	res.OK = script.OK
	res.Command = commands[0]
	res.Commands = append([]string(nil), commands...)
	res.Output = script.Output
	res.Error = script.Error
	res.Steps = script.Steps
	return res
}

func splitTelnetCommands(rendered string) []string {
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return nil
	}
	parts := strings.FieldsFunc(rendered, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ';'
	})
	out := make([]string, 0, len(parts)+8)
	for _, p := range parts {
		cmd := strings.TrimSpace(p)
		if cmd == "" {
			continue
		}
		if looksLikeMashedTelnetChain(cmd) {
			out = append(out, expandMashedTelnetCommands(cmd)...)
			continue
		}
		out = append(out, cmd)
	}
	return out
}

func looksLikeMashedTelnetChain(s string) bool {
	lower := strings.ToLower(s)
	hasIface := strings.Contains(lower, "interface ")
	hasExit := strings.Contains(lower, " exit") || strings.HasSuffix(lower, " exit") || strings.Contains(lower, "exit ")
	hasOnu := regexp.MustCompile(`(?i)\sonu\s+(\d+|\{onu\})\s+type\b`).MatchString(s)
	hasMng := strings.Contains(lower, "pon-onu-mng ")
	hits := 0
	if hasIface {
		hits++
	}
	if hasExit {
		hits++
	}
	if hasOnu {
		hits++
	}
	if hasMng {
		hits++
	}
	// Ex.: "interface gpon_olt-1/1/9 onu 80 type … sn …" sem ";"
	return hits >= 2 || (hasIface && hasOnu)
}

func expandMashedTelnetCommands(blob string) []string {
	s := strings.TrimSpace(blob)
	if s == "" {
		return nil
	}
	out := make([]string, 0, 16)
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		matched := false
		for _, re := range mashedTelnetCmdRes {
			loc := re.FindStringIndex(s)
			if loc == nil || loc[0] != 0 {
				continue
			}
			out = append(out, strings.TrimSpace(s[loc[0]:loc[1]]))
			s = strings.TrimSpace(s[loc[1]:])
			matched = true
			break
		}
		if matched {
			continue
		}
		// Texto residual: corta no próximo comando reconhecível.
		next := -1
		for i := 1; i < len(s); i++ {
			if s[i] != ' ' && s[i] != '\t' {
				continue
			}
			rest := strings.TrimSpace(s[i:])
			for _, re := range mashedTelnetCmdRes {
				if loc := re.FindStringIndex(rest); loc != nil && loc[0] == 0 {
					next = i
					break
				}
			}
			if next >= 0 {
				break
			}
		}
		if next > 0 {
			frag := strings.TrimSpace(s[:next])
			if frag != "" {
				out = append(out, frag)
			}
			s = strings.TrimSpace(s[next:])
			continue
		}
		out = append(out, s)
		break
	}
	return out
}

// ensureConfigTerminalPrefix garante entrada em config se o script começa por interface/onu ZTE
// e nem os pré-comandos nem o script já fazem "configure terminal".
func ensureConfigTerminalPrefix(preTemplates, commands []string) []string {
	if len(commands) == 0 {
		return commands
	}
	joined := strings.ToLower(strings.Join(preTemplates, "\n") + "\n" + strings.Join(commands, "\n"))
	if strings.Contains(joined, "configure terminal") || strings.Contains(joined, "conf t") {
		return commands
	}
	first := strings.ToLower(strings.TrimSpace(commands[0]))
	needs := strings.HasPrefix(first, "interface gpon_olt") ||
		strings.HasPrefix(first, "interface gpon_onu") ||
		strings.HasPrefix(first, "pon-onu-mng") ||
		strings.HasPrefix(first, "onu ")
	if !needs {
		return commands
	}
	out := make([]string, 0, len(commands)+1)
	out = append(out, "configure terminal")
	out = append(out, commands...)
	return out
}

func looksLikeAuthorizeTelnetScript(commands []string) bool {
	joined := strings.ToLower(strings.Join(commands, "\n"))
	hasOnuType := regexp.MustCompile(`(?i)\bonu\s+\d+\s+type\b`).MatchString(joined)
	hasVport := strings.Contains(joined, "interface vport-") || strings.Contains(joined, "service-port ")
	hasMng := strings.Contains(joined, "pon-onu-mng ")
	return hasOnuType && (hasVport || hasMng)
}

func logAuthorizeTelnetScript(host string, target OnuReportTarget, pre, commands []string) {
	fmt.Fprintf(os.Stderr, "\n======== [TEMP] provisionamento ONU telnet → %s ========\n", host)
	fmt.Fprintf(os.Stderr, "  pon=%d onu=%d serial=%s vlan=%s type=%s name=%s gpon_onu=%s\n",
		target.Pon, target.Onu, strings.TrimSpace(target.Serial), strings.TrimSpace(target.Vlan),
		strings.TrimSpace(target.OnuType), strings.TrimSpace(target.Name), ResolveGponOnu(target))
	if len(pre) > 0 {
		fmt.Fprintf(os.Stderr, "  pré-comandos (%d):\n", len(pre))
		for i, c := range pre {
			fmt.Fprintf(os.Stderr, "    P%02d: %s\n", i+1, c)
		}
	}
	fmt.Fprintf(os.Stderr, "  comandos (%d):\n", len(commands))
	for i, c := range commands {
		fmt.Fprintf(os.Stderr, "    %02d: %s\n", i+1, c)
	}
	fmt.Fprintf(os.Stderr, "======== [TEMP] fim script provisionamento ========\n\n")
}
