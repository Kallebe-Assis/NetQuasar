package oltcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const defaultVsolUnauthorizedIfaceTpl = "interface gpon 0/{pon}"

// UnauthorizedUsesPonPlaceholder indica iteração por porta PON (pré-comandos ou comando com {pon}).
func (c OnuReportConfig) UnauthorizedUsesPonPlaceholder() bool {
	for _, tpl := range c.UnauthorizedOnuPreCommands {
		if strings.Contains(tpl, "{pon}") || strings.Contains(tpl, "{pon_port}") {
			return true
		}
	}
	cmd := strings.TrimSpace(c.UnauthorizedOnuQueryCommand)
	if strings.Contains(cmd, "{pon}") || strings.Contains(cmd, "{pon_port}") {
		return true
	}
	return unauthorizedNeedsPonIteration(c)
}

func unauthorizedNeedsPonIteration(c OnuReportConfig) bool {
	cmd := strings.ToLower(strings.TrimSpace(c.UnauthorizedOnuQueryCommand))
	if cmd == "show onu auto-find" {
		return true
	}
	for _, tpl := range c.UnauthorizedOnuPreCommands {
		if vsolGponInterfaceRE.MatchString(strings.TrimSpace(tpl)) {
			return true
		}
	}
	return false
}

// RunUnauthorizedOnuQueryMulti executa a consulta de ONUs não autorizadas.
// Percorre as portas PON do snapshot (ou 1–16) numa única sessão telnet.
func RunUnauthorizedOnuQueryMulti(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	ponIndexes []int,
	timeout time.Duration,
) OnuTelnetActionResult {
	cmdTpl := strings.TrimSpace(cfg.UnauthorizedOnuQueryCommand)
	if cmdTpl == "" {
		return OnuTelnetActionResult{Error: "comando telnet não configurado"}
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if !unauthorizedNeedsPonIteration(cfg) && !cfg.UnauthorizedUsesPonPlaceholder() {
		return RunUnauthorizedOnuQuery(ctx, host, user, password, enable, cfg, secrets, timeout)
	}

	ponsToQuery := resolvePonsForSerialSearch(0, ponIndexes)
	if len(ponsToQuery) == 0 {
		return OnuTelnetActionResult{Error: "informe portas PON ou actualize o snapshot da OLT para listar portas"}
	}

	globalPre, perPonPre := splitUnauthorizedPreCommands(cfg.UnauthorizedOnuPreCommands)
	if len(perPonPre) == 0 && unauthorizedNeedsPonIteration(cfg) {
		perPonPre = []string{defaultVsolUnauthorizedIfaceTpl}
	}
	globalRendered := renderTelnetPreTemplates(globalPre, OnuReportTarget{}, secrets)

	session, err := probing.OpenTelnetSession(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		PreCommands: globalRendered, RawPreCommands: globalPre,
		MaxReadBytes: 320000,
	})
	if err != nil {
		return OnuTelnetActionResult{Error: err.Error()}
	}
	defer session.Close()

	res := OnuTelnetActionResult{PonsQueried: append([]int(nil), ponsToQuery...)}
	cmdRead := 20 * time.Second
	if n := len(ponsToQuery); n > 0 {
		if avg := timeout / time.Duration(n); avg > 8*time.Second && avg < cmdRead {
			cmdRead = avg
		}
	}

	var allEntries []SerialSearchOnuEntry
	var outputs []string
	seen := map[string]bool{}

	for _, pon := range ponsToQuery {
		if ctx.Err() != nil {
			res.Error = ctx.Err().Error()
			break
		}
		target := OnuReportTarget{Pon: pon}
		var scriptCmds []string
		if len(perPonPre) > 0 {
			perRendered := renderTelnetPreTemplates(perPonPre, target, secrets)
			scriptCmds = append(scriptCmds, perRendered...)
		}
		cmd := SubstituteTelnetTemplate(cmdTpl, target, secrets)
		if cmd == "" {
			continue
		}
		scriptCmds = append(scriptCmds, cmd)
		script := session.ExecCommands(scriptCmds, cmdRead)
		res.Steps = append(res.Steps, script.Steps...)
		stepOut := script.Output
		if stepOut == "" && len(script.Steps) > 0 {
			stepOut = script.Steps[len(script.Steps)-1].Output
		}
		outputs = append(outputs, fmt.Sprintf("=== PON %d ===\n%s", pon, stepOut))
		res.Commands = append(res.Commands, scriptCmds...)
		if script.OK {
			res.OK = true
			for _, e := range ParseOnuListFromTelnetOutput(stepOut) {
				key := onuListEntryKey(e)
				if seen[key] {
					continue
				}
				seen[key] = true
				allEntries = append(allEntries, e)
			}
		} else if res.Error == "" && script.Error != "" {
			res.Error = script.Error
		}
	}

	res.Output = strings.TrimSpace(strings.Join(outputs, "\n\n"))
	if len(ponsToQuery) > 0 {
		res.Command = fmt.Sprintf("%s em %d porta(s) PON", cmdTpl, len(ponsToQuery))
	}
	if len(allEntries) > 0 {
		res.OK = true
	}
	if !res.OK && res.Error == "" {
		res.Error = "nenhuma ONU não autorizada encontrada na consulta telnet"
	}
	return res
}

func splitUnauthorizedPreCommands(pre []string) (global, perPon []string) {
	for _, tpl := range pre {
		trimmed := strings.TrimSpace(tpl)
		if strings.Contains(tpl, "{pon}") || strings.Contains(tpl, "{pon_port}") || vsolGponInterfaceRE.MatchString(trimmed) {
			perPon = append(perPon, tpl)
		} else {
			global = append(global, tpl)
		}
	}
	return global, perPon
}
