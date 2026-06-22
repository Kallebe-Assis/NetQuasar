package oltcollect

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PonTelnetCollectResult resultado do enriquecimento telnet por PON.
type PonTelnetCollectResult struct {
	Rows    []map[string]any
	Summary map[string]any
}

// EnrichPonRowsViaTelnet executa comandos CLI por porta PON numa única sessão telnet (login uma vez).
func EnrichPonRowsViaTelnet(
	ctx context.Context,
	host string,
	creds TelnetCredentials,
	cfg PonTelnetConfig,
	pons []map[string]any,
	opts PonTelnetEnrichOpts,
	totalBudget time.Duration,
) PonTelnetCollectResult {
	res := PonTelnetCollectResult{
		Rows:    pons,
		Summary: map[string]any{"pon_telnet_enabled": true},
	}
	if !cfg.MonitorEnabled() || len(pons) == 0 {
		res.Summary["pon_telnet_skipped"] = "desactivado ou sem PONs"
		return res
	}
	if strings.TrimSpace(host) == "" {
		res.Summary["pon_telnet_error"] = "host em falta"
		return res
	}
	if totalBudget <= 0 {
		totalBudget = 10 * time.Minute
	}
	res.Summary["pon_telnet_timeout_ms"] = totalBudget.Milliseconds()

	secrets := TelnetSecrets{Password: creds.Password, Enable: creds.Enable}
	if cfg.NeedsEnablePassword() && strings.TrimSpace(creds.Enable) == "" {
		res.Summary["pon_telnet_error"] = "palavra-passe enable em falta"
		return res
	}

	sorted := make([]map[string]any, len(pons))
	copy(sorted, pons)
	sortPonRows(sorted)

	maxN := cfg.EffectiveMaxPons()
	batch, nextOffset := selectRotatingPonBatch(sorted, maxN, opts.RotateOffset)
	totalPons := len(sorted)
	res.Summary["pon_telnet_rotate_total"] = totalPons
	res.Summary["pon_telnet_rotate_offset"] = nextOffset
	res.Summary["pon_telnet_rotate_batch"] = len(batch)
	if totalPons > len(batch) {
		res.Summary["pon_telnet_truncated"] = totalPons - len(batch)
	}
	if totalPons > 0 && len(batch) > 0 {
		res.Summary["pon_telnet_rotate_note"] = fmt.Sprintf(
			"rodízio PON: %d/%d portas neste ciclo (próximo offset %d)",
			len(batch), totalPons, nextOffset,
		)
	}

	refreshKeys := make(map[string]bool, len(batch))
	for _, row := range batch {
		if k := ponStableKey(row); k != "" {
			refreshKeys[k] = true
		}
	}
	carryForwardPonTelnetFromPrev(pons, opts.PrevRows, refreshKeys)

	telCtx, cancel := context.WithTimeout(ctx, totalBudget)
	defer cancel()

	session, err := openPonTelnetSession(telCtx, host, creds, cfg, secrets, totalBudget)
	if err != nil {
		res.Summary["pon_telnet_error"] = err.Error()
		return res
	}
	defer session.Close()

	cmdRead := 12 * time.Second
	if n := len(batch) * len(cfg.Commands); n > 0 {
		if avg := totalBudget / time.Duration(n); avg > 3*time.Second && avg < cmdRead {
			cmdRead = avg
		}
	}

	reportedAt := time.Now().UTC().Format(time.RFC3339)
	okCount, errCount := 0, 0

	for _, row := range batch {
		if telCtx.Err() != nil {
			res.Summary["pon_telnet_cancelled"] = telCtx.Err().Error()
			break
		}
		if row == nil {
			continue
		}
		pon := ponIndexFromRowMap(row)
		if pon <= 0 {
			continue
		}
		cmds := cfg.RenderCommands(OnuReportTarget{Pon: pon}, secrets)
		if len(cmds) == 0 {
			errCount++
			continue
		}
		script := session.ExecCommands(cmds, cmdRead)
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
	res.Summary["pon_telnet_session_reuse"] = true
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
