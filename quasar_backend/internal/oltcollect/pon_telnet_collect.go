package oltcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// PonTelnetCollectResult resultado do enriquecimento telnet por PON.
type PonTelnetCollectResult struct {
	Rows    []map[string]any
	Summary map[string]any
}

// EnrichPonRowsViaTelnet executa comandos CLI por porta PON e funde métricas ópticas nas linhas.
func EnrichPonRowsViaTelnet(
	ctx context.Context,
	host string,
	creds TelnetCredentials,
	cfg PonTelnetConfig,
	pons []map[string]any,
	perPonBudget time.Duration,
) PonTelnetCollectResult {
	res := PonTelnetCollectResult{
		Rows:    pons,
		Summary: map[string]any{},
	}
	if !cfg.MonitorEnabled() || len(pons) == 0 {
		res.Summary["pon_telnet_skipped"] = "desactivado ou sem PONs"
		return res
	}
	if strings.TrimSpace(host) == "" {
		res.Summary["pon_telnet_error"] = "host em falta"
		return res
	}
	secrets := TelnetSecrets{Password: creds.Password, Enable: creds.Enable}
	if cfg.NeedsEnablePassword() && strings.TrimSpace(creds.Enable) == "" {
		res.Summary["pon_telnet_error"] = "palavra-passe enable em falta"
		return res
	}

	limit := cfg.EffectiveMaxPons()
	if limit > len(pons) {
		limit = len(pons)
	}
	reportedAt := time.Now().UTC().Format(time.RFC3339)
	okCount, errCount := 0, 0

	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			res.Summary["pon_telnet_cancelled"] = ctx.Err().Error()
			break
		}
		row := pons[i]
		if row == nil {
			continue
		}
		pon := ponIndexFromRowMap(row)
		if pon <= 0 {
			continue
		}
		target := OnuReportTarget{Pon: pon}
		preRendered := cfg.RenderPreCommands(target, secrets)
		cmds := cfg.RenderCommands(target, secrets)
		if len(cmds) == 0 {
			errCount++
			continue
		}
		budget := perPonBudget
		if budget <= 0 {
			budget = 35 * time.Second
		}
		script := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
			Host: host, Port: "23", Timeout: budget,
			User: creds.User, Password: creds.Password, Enable: creds.Enable,
			PreCommands: preRendered, RawPreCommands: cfg.PreCommands,
			Commands: cmds, MaxReadBytes: 120000,
		})
		if !script.OK {
			errCount++
			continue
		}
		var steps []struct {
			Command string
			Output  string
		}
		for _, st := range script.Steps {
			steps = append(steps, struct {
				Command string
				Output  string
			}{Command: st.Command, Output: st.Output})
		}
		fields := ParseTelnetReportSteps(steps)
		if len(fields) == 0 {
			errCount++
			continue
		}
		mergeTelnetFieldsIntoPonRow(row, fields, reportedAt)
		okCount++
	}

	res.Summary["pon_telnet_collected"] = okCount
	res.Summary["pon_telnet_errors"] = errCount
	res.Summary["pon_telnet_at"] = reportedAt
	if okCount == 0 && errCount > 0 {
		res.Summary["pon_telnet_error"] = fmt.Sprintf("%d PON(s) sem resposta telnet", errCount)
	}
	return res
}

// ApplyPonTelnetResultToSummary actualiza summary e devolve linhas PON enriquecidas.
func ApplyPonTelnetResultToSummary(summary map[string]any, result PonTelnetCollectResult) {
	if summary == nil {
		return
	}
	for k, v := range result.Summary {
		summary[k] = v
	}
	if len(result.Rows) > 0 {
		summary["pon_telnet_rows"] = len(result.Rows)
	}
}
