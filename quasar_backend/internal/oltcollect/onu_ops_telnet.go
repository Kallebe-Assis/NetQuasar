package oltcollect

import (
	"context"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// OnuTelnetActionResult resultado de acção telnet pontual (autorizar, listar, etc.).
type OnuTelnetActionResult struct {
	OK       bool
	Command  string
	Commands []string
	Output   string
	Error    string
	Steps    []probing.TelnetScriptStepResult
}

// RunOnuTelnetAction executa pré-comandos + um comando telnet com substituição de placeholders.
func RunOnuTelnetAction(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
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
	if cmd == "" {
		res.Error = "comando vazio após substituição de placeholders"
		return res
	}
	preRendered := cfg.RenderPreCommands(target, secrets)
	script := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		PreCommands: preRendered, RawPreCommands: cfg.PreCommands,
		Commands: []string{cmd}, MaxReadBytes: 320000,
	})
	res.OK = script.OK
	res.Command = cmd
	res.Commands = []string{cmd}
	res.Output = script.Output
	res.Error = script.Error
	res.Steps = script.Steps
	return res
}
