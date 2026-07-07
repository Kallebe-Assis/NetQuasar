package oltcollect

import (
	"context"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

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
	if cmd == "" {
		res.Error = "comando vazio após substituição de placeholders"
		return res
	}
	preRendered := renderTelnetPreTemplates(preTemplates, target, secrets)
	script := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		PreCommands: preRendered, RawPreCommands: preTemplates,
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
