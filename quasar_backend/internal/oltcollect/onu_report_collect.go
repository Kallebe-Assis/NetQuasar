package oltcollect

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// TelnetCredentials credenciais CLI da OLT.
type TelnetCredentials struct {
	User     string
	Password string
	Enable   string
}

// OnuTelnetCollectResult resultado do enriquecimento telnet por ONU.
type OnuTelnetCollectResult struct {
	Rows    []map[string]any
	Summary map[string]any
}

// LoadTelnetCredentials lê credenciais telnet de settings_connection_defaults.
func LoadTelnetCredentials(ctx context.Context, pool *pgxpool.Pool) (TelnetCredentials, error) {
	var out TelnetCredentials
	if pool == nil {
		return out, fmt.Errorf("pool indisponível")
	}
	var tu, tp, te *string
	if err := pool.QueryRow(ctx, `SELECT telnet_user, telnet_password, telnet_enable FROM settings_connection_defaults WHERE id=1`).
		Scan(&tu, &tp, &te); err != nil {
		return out, err
	}
	if tu != nil {
		out.User = strings.TrimSpace(*tu)
	}
	if tp != nil {
		out.Password = strings.TrimSpace(*tp)
	}
	if te != nil {
		out.Enable = strings.TrimSpace(*te)
	}
	if out.User == "" || out.Password == "" {
		return out, fmt.Errorf("credenciais telnet não configuradas")
	}
	return out, nil
}

// OnuRowsFromSummary extrai linhas ONU do summary (vsol_onu_rows).
func OnuRowsFromSummary(summary map[string]any) []map[string]any {
	if summary == nil {
		return nil
	}
	raw, ok := summary["vsol_onu_rows"]
	if !ok || raw == nil {
		return nil
	}
	switch x := raw.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func onuRowOnline(row map[string]any) bool {
	if row == nil {
		return false
	}
	if b, ok := row["online"].(bool); ok {
		return b
	}
	if sta, ok := row["onu_online_sta"]; ok {
		switch v := sta.(type) {
		case float64:
			return int(v) == 1 || int(v) == 3
		case int:
			return v == 1 || v == 3
		}
	}
	if label := strings.ToLower(stringFromAny(row["oper_status_label"])); label == "up" {
		return true
	}
	return false
}

func onuTargetFromRow(row map[string]any) OnuReportTarget {
	t := OnuReportTarget{}
	if row == nil {
		return t
	}
	t.Pon = intFromRow(row, "pon")
	t.Onu = intFromRow(row, "onu")
	t.Serial = stringFromAny(row["serial"])
	t.IfIndex = intFromRow(row, "if_index")
	t.IfName = stringFromAny(row["if_name"])
	if t.IfName == "" {
		t.IfName = stringFromAny(row["if_descr"])
	}
	t.GponOnu = ResolveGponOnu(t)
	return t
}

func intFromRow(row map[string]any, key string) int {
	v, ok := row[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

// EnrichOnuRowsViaTelnet executa comandos do perfil por ONU e funde campos na tabela.
func EnrichOnuRowsViaTelnet(
	ctx context.Context,
	host string,
	creds TelnetCredentials,
	cfg OnuReportConfig,
	rows []map[string]any,
	telnetTimeout time.Duration,
) OnuTelnetCollectResult {
	res := OnuTelnetCollectResult{
		Summary: map[string]any{
			"onu_telnet_enabled": true,
		},
	}
	if !cfg.MonitorEnabled() {
		res.Summary["onu_telnet_skipped"] = "desactivado no perfil"
		res.Rows = rows
		return res
	}
	if len(rows) == 0 {
		res.Summary["onu_telnet_skipped"] = "sem ONUs na coleta SNMP"
		return res
	}
	if telnetTimeout <= 0 {
		telnetTimeout = 45 * time.Second
	}
	if telnetTimeout > 90*time.Second {
		telnetTimeout = 90 * time.Second
	}

	secrets := TelnetSecrets{Password: creds.Password, Enable: creds.Enable}
	if cfg.NeedsEnablePassword() && creds.Enable == "" {
		res.Summary["onu_telnet_error"] = "enable telnet em falta para pré-comandos"
		res.Rows = rows
		return res
	}

	maxN := cfg.EffectiveMaxOnus()
	candidates := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if cfg.MonitorOnlineOnly && !onuRowOnline(row) {
			continue
		}
		candidates = append(candidates, row)
	}
	if len(candidates) == 0 {
		candidates = rows
	}
	if len(candidates) > maxN {
		candidates = candidates[:maxN]
		res.Summary["onu_telnet_truncated"] = len(rows) - maxN
	}

	reportedAt := time.Now().UTC().Format(time.RFC3339)
	okCount, errCount := 0, 0
	outRows := make([]map[string]any, len(rows))
	copyMaps := func(dst []map[string]any) {
		for i, r := range rows {
			cp := map[string]any{}
			for k, v := range r {
				cp[k] = v
			}
			dst[i] = cp
		}
	}
	copyMaps(outRows)
	rowIndex := map[string]int{}
	for i, r := range outRows {
		key := fmt.Sprintf("%d.%d", intFromRow(r, "pon"), intFromRow(r, "onu"))
		rowIndex[key] = i
	}

	perOnuBudget := telnetTimeout
	if n := len(candidates); n > 0 {
		if dl, ok := ctx.Deadline(); ok {
			if left := time.Until(dl) - 3*time.Second; left > 10*time.Second {
				per := left / time.Duration(n)
				if per < perOnuBudget {
					perOnuBudget = per
				}
				if perOnuBudget < 12*time.Second {
					perOnuBudget = 12 * time.Second
				}
			}
		}
	}

	for _, row := range candidates {
		if ctx.Err() != nil {
			res.Summary["onu_telnet_cancelled"] = ctx.Err().Error()
			break
		}
		target := onuTargetFromRow(row)
		if target.GponOnu == "" && cfg.NeedsGponOnu() && strings.TrimSpace(target.Serial) != "" {
			lookupCmd := cfg.RenderSerialSearchCommand(target, secrets)
			pre := cfg.RenderPreCommands(target, secrets)
			tel := probing.TelnetRunCommand(ctx, probing.TelnetRunParams{
				Host: host, Port: "23", Timeout: perOnuBudget,
				User: creds.User, Password: creds.Password, Enable: creds.Enable,
				Command: lookupCmd, PreCommands: pre, MaxReadBytes: 120000,
			})
			if tel.OK {
				if g := ParseGponOnuFromOutput(tel.Output); g != "" {
					target.GponOnu = g
				}
			}
		}

		preRendered := cfg.RenderPreCommands(target, secrets)
		cmds := cfg.RenderCommands(target, secrets)
		if len(cmds) == 0 {
			errCount++
			continue
		}

		script := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
			Host: host, Port: "23", Timeout: perOnuBudget,
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

		key := fmt.Sprintf("%d.%d", target.Pon, target.Onu)
		idx, ok := rowIndex[key]
		if !ok {
			continue
		}
		mergeTelnetFieldsIntoOnuRow(outRows[idx], fields, reportedAt)
		okCount++
	}

	res.Rows = outRows
	res.Summary["onu_telnet_collected"] = okCount
	res.Summary["onu_telnet_errors"] = errCount
	res.Summary["onu_telnet_candidates"] = len(candidates)
	res.Summary["onu_telnet_at"] = reportedAt
	return res
}

// ApplyOnuTelnetResultToSummary actualiza vsol_onu_rows no summary.
func ApplyOnuTelnetResultToSummary(summary map[string]any, result OnuTelnetCollectResult) {
	if summary == nil {
		return
	}
	for k, v := range result.Summary {
		summary[k] = v
	}
	if len(result.Rows) == 0 {
		return
	}
	arr := make([]any, 0, len(result.Rows))
	for _, r := range result.Rows {
		arr = append(arr, r)
	}
	summary["vsol_onu_rows"] = arr
	summary["onu_telnet_enriched"] = true
}
