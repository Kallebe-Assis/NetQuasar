package oltcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// OnuReportResult é a leitura telnet de UMA ONU (campos já interpretados), pronta para ser gravada no snapshot.
type OnuReportResult struct {
	Pon    int
	Onu    int
	Serial string
	Fields map[string]string
}

// ParseOnuReportSteps interpreta os passos do relatório telnet de uma ONU nos campos padrão (RX, TX, Temperatura…).
func ParseOnuReportSteps(steps []struct {
	Command string
	Output  string
}) map[string]string {
	return ParseTelnetReportSteps(steps)
}

// ApplyOnuReportResults grava no summary (vsol_onu_rows) as leituras feitas à mão/em lote por ONU. Diferente da coleta
// automática, aqui a leitura nova SUBSTITUI a antiga: telnet_fields é trocado por inteiro e RX/TX/temperatura/tensão
// são sobrescritos. Só uma ONU marcada explicitamente como offline não recebe leitura óptica (e perde a que tinha).
// Devolve quantas linhas foram atualizadas.
func ApplyOnuReportResults(summary map[string]any, results []OnuReportResult, at time.Time) int {
	rows := OnuRowsFromSummary(summary)
	if len(rows) == 0 || len(results) == 0 {
		return 0
	}
	byKey := make(map[string]map[string]any, len(rows))
	bySerial := make(map[string]map[string]any, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		if k := onuRowKey(r); k != "0.0" {
			byKey[k] = r
		}
		if sn := strings.ToUpper(strings.TrimSpace(stringFromAny(r["serial"]))); sn != "" {
			bySerial[sn] = r
		}
	}
	reportedAt := at.UTC().Format(time.RFC3339)
	updated := 0
	for _, res := range results {
		if len(res.Fields) == 0 {
			continue
		}
		row := byKey[fmt.Sprintf("%d.%d", res.Pon, res.Onu)]
		if row == nil && res.Pon <= 0 {
			row = bySerial[strings.ToUpper(strings.TrimSpace(res.Serial))]
		}
		if row == nil {
			continue
		}
		applyOnuReportToRow(row, res.Fields, reportedAt)
		updated++
	}
	return updated
}

func applyOnuReportToRow(row map[string]any, fields map[string]string, reportedAt string) {
	if v := strings.TrimSpace(fields["SN"]); v != "" && strings.TrimSpace(stringFromAny(row["serial"])) == "" {
		row["serial"] = v
	}
	if v := strings.TrimSpace(fields["Modelo"]); v != "" {
		row["model"] = v
	}
	if v := strings.TrimSpace(fields["Profile"]); v != "" {
		row["profile_name"] = v
	}
	if v := strings.TrimSpace(fields["Estado"]); v != "" {
		row["phase_sta"] = v
	}
	if v := strings.TrimSpace(fields["Canal"]); v != "" {
		row["channel"] = v
	}

	// O Estado lido agora na própria OLT (working/LOS/DyingGasp…) manda no status da linha — a leitura de RX sozinha NÃO
	// prova que a ONU está online (a OLT devolve a última potência válida de ONU já caída).
	if online := onlineFromState(fields["Estado"]); online != nil {
		row["online"] = *online
	}
	explicitOffline := false
	if b, ok := row["online"].(bool); ok && !b {
		explicitOffline = true
	}

	tf := make(map[string]any, len(fields))
	for k, v := range fields {
		if explicitOffline && isLiveTelnetField(k) {
			continue
		}
		tf[k] = v
	}
	row["telnet_fields"] = tf

	if explicitOffline {
		for _, k := range onuLiveMetricsKeys {
			delete(row, k)
		}
	} else {
		if v := strings.TrimSpace(fields["RX"]); v != "" {
			row["rx_pwr"] = v
			if dbm := parseDbmValue(v); dbm != nil {
				row["rx_dbm"] = *dbm
			} else {
				delete(row, "rx_dbm")
			}
		}
		if v := strings.TrimSpace(fields["TX"]); v != "" {
			row["tx_pwr"] = v
			if dbm := parseDbmValue(v); dbm != nil {
				row["tx_dbm"] = *dbm
			} else {
				delete(row, "tx_dbm")
			}
		}
		if v := strings.TrimSpace(fields["Temperatura"]); v != "" {
			row["temp"] = v
		}
		if v := strings.TrimSpace(fields["Voltagem"]); v != "" {
			row["voltage"] = v
		}
	}
	row["telnet_report_at"] = reportedAt
	row["data_source_telnet"] = true
}

// PonRxRefreshHooks são os ganchos de progresso/gravação do loop ONU a ONU.
type PonRxRefreshHooks struct {
	Progress func(done, total int, current string)
	// Flush recebe as leituras acumuladas (em lotes) para gravar no snapshot.
	Flush func(batch []OnuReportResult) error
}

// RefreshPonOnuReports roda o relatório telnet do perfil ONU a ONU (uma sessão telnet só), de forma sequencial —
// algumas OLTs não têm um comando que traga o RX de todas as ONUs de uma vez. `rows` são as ONUs a ler.
// Para ao esgotar a lista, ao cancelar o contexto ou após 6 falhas seguidas em ONUs online (sessão provavelmente quebrada).
func RefreshPonOnuReports(
	ctx context.Context,
	host string,
	creds TelnetCredentials,
	cfg OnuReportConfig,
	rows []map[string]any,
	hooks PonRxRefreshHooks,
) (okCount, errCount int, err error) {
	if len(rows) == 0 {
		return 0, 0, nil
	}
	secrets := TelnetSecrets{Password: creds.Password, Enable: creds.Enable}
	if cfg.NeedsEnablePassword() && creds.Enable == "" {
		return 0, 0, fmt.Errorf("enable telnet em falta para os pré-comandos do perfil")
	}
	first := onuTargetFromRow(rows[0])
	session, oerr := probing.OpenTelnetSession(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: 20 * time.Minute,
		User: creds.User, Password: creds.Password, Enable: creds.Enable,
		PreCommands:    cfg.RenderPreCommands(first, secrets),
		RawPreCommands: cfg.PreCommands,
		MaxReadBytes:   120000,
	})
	if oerr != nil {
		return 0, 0, oerr
	}
	defer session.Close()

	const cmdRead = 10 * time.Second
	const flushEvery = 8
	var pending []OnuReportResult
	flush := func() error {
		if len(pending) == 0 || hooks.Flush == nil {
			pending = nil
			return nil
		}
		batch := pending
		pending = nil
		return hooks.Flush(batch)
	}

	consecutive := 0
	for i, row := range rows {
		if ctx.Err() != nil {
			break
		}
		target := onuTargetFromRow(row)
		if hooks.Progress != nil {
			hooks.Progress(i, len(rows), fmt.Sprintf("ONU %d", target.Onu))
		}
		if target.GponOnu == "" && cfg.NeedsGponOnu() && strings.TrimSpace(target.Serial) != "" {
			if lookupCmd := cfg.RenderSerialSearchCommand(target, secrets); lookupCmd != "" {
				script := session.ExecCommands([]string{lookupCmd}, cmdRead)
				if script.OK {
					listMode := !cfg.SerialSearchUsesSerialPlaceholder()
					if g := ResolveGponOnuFromSerialSearchOutput(script.Output, target.Serial, listMode); g != "" {
						target.GponOnu = g
					}
				}
			}
		}
		cmds := cfg.RenderCommands(target, secrets)
		if len(cmds) == 0 {
			errCount++
			continue
		}
		script := session.ExecCommands(cmds, cmdRead)
		var fields map[string]string
		if script.OK {
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
			fields = ParseTelnetReportSteps(steps)
		}
		if len(fields) == 0 {
			errCount++
			// ONU offline costuma não responder ao relatório: não conta para a detecção de sessão caída.
			if b, ok := row["online"].(bool); ok && !b {
				continue
			}
			consecutive++
			if consecutive >= 6 {
				_ = flush()
				return okCount, errCount, fmt.Errorf("6 falhas seguidas — a sessão telnet parece ter caído")
			}
			continue
		}
		consecutive = 0
		okCount++
		pending = append(pending, OnuReportResult{Pon: target.Pon, Onu: target.Onu, Serial: target.Serial, Fields: fields})
		if len(pending) >= flushEvery {
			if ferr := flush(); ferr != nil {
				return okCount, errCount, ferr
			}
		}
	}
	if hooks.Progress != nil {
		hooks.Progress(len(rows), len(rows), "")
	}
	return okCount, errCount, flush()
}
