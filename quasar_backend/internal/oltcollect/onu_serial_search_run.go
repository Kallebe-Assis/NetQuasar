package oltcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// SerialSearchRunOpts opções da pesquisa telnet por serial.
type SerialSearchRunOpts struct {
	Serial string
	Pon    int // 0 = todas as PONs
}

// SerialSearchRunResult resultado da pesquisa telnet.
type SerialSearchRunResult struct {
	OK       bool
	Mode     string // "direct" | "list"
	Command  string
	Commands []map[string]any
	Output   string
	Matches  []SerialSearchOnuEntry
	Error    string
}

// LoadOLTPonIndexesFromSnapshot lê números de porta PON do snapshot da OLT.
func LoadOLTPonIndexesFromSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) []int {
	if pool == nil || deviceID == uuid.Nil {
		return nil
	}
	var ponsRaw []byte
	err := pool.QueryRow(ctx, `SELECT COALESCE(pons::text, '[]') FROM olt_snapshots WHERE device_id=$1`, deviceID).Scan(&ponsRaw)
	if err != nil {
		return nil
	}
	var arr []any
	if json.Unmarshal(ponsRaw, &arr) != nil {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		p := ponIndexFromRowMap(m)
		if p > 0 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// RunSerialSearchTelnet executa pesquisa directa ou por listagem conforme o perfil.
// Com PON informada, prioriza listagem na porta e comparação parcial de serial localmente.
func RunSerialSearchTelnet(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	opts SerialSearchRunOpts,
	ponIndexes []int,
	timeout time.Duration,
) SerialSearchRunResult {
	serial := strings.TrimSpace(opts.Serial)
	res := SerialSearchRunResult{Mode: "direct"}
	if serial == "" {
		res.Error = "serial em falta"
		return res
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	target := OnuReportTarget{Serial: serial, Pon: opts.Pon}
	listCmd := cfg.ListSerialSearchCommand()

	// PON específica + comando de listagem → percorre ONUs da porta e filtra serial parcial.
	if opts.Pon > 0 && listCmd != "" {
		listRes := runListSerialSearch(ctx, host, user, password, enable, cfg, secrets, target, []int{opts.Pon}, ponIndexes, timeout)
		listRes.Mode = "list"
		return listRes
	}

	if cfg.SerialSearchUsesSerialPlaceholder() {
		directRes := runDirectSerialSearch(ctx, host, user, password, enable, cfg, secrets, target, timeout)
		directRes.Matches = filterDirectSerialMatches(directRes.Matches, serial)
		directRes.OK = len(directRes.Matches) > 0
		if directRes.OK {
			return directRes
		}
		if listCmd != "" {
			pons := resolvePonsForSerialSearch(opts.Pon, ponIndexes)
			listRes := runListSerialSearch(ctx, host, user, password, enable, cfg, secrets, target, pons, ponIndexes, timeout)
			listRes.Mode = "list"
			if listRes.OK {
				return listRes
			}
			if directRes.Error == "" && listRes.Error != "" {
				directRes.Error = listRes.Error
			}
		}
		return directRes
	}

	listRes := runListSerialSearch(ctx, host, user, password, enable, cfg, secrets, target, nil, ponIndexes, timeout)
	listRes.Mode = "list"
	return listRes
}

func filterDirectSerialMatches(entries []SerialSearchOnuEntry, serial string) []SerialSearchOnuEntry {
	if len(entries) == 0 {
		return entries
	}
	var out []SerialSearchOnuEntry
	for _, e := range entries {
		if e.Serial == "" || serialPartialMatch(e.Serial, serial) {
			out = append(out, e)
		}
	}
	return out
}

func runListSerialSearch(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	target OnuReportTarget,
	ponsOverride []int,
	ponIndexes []int,
	timeout time.Duration,
) SerialSearchRunResult {
	res := SerialSearchRunResult{Mode: "list"}
	cmdTpl := cfg.ListSerialSearchCommand()
	if cmdTpl == "" {
		res.Error = "comando de listagem por PON não configurado"
		return res
	}
	res.Command = cmdTpl

	ponsToQuery := ponsOverride
	if len(ponsToQuery) == 0 {
		ponsToQuery = resolvePonsForSerialSearch(target.Pon, ponIndexes)
	}
	if len(ponsToQuery) == 0 {
		res.Error = "informe a porta PON ou actualize o snapshot da OLT para listar portas"
		return res
	}

	preRendered := cfg.RenderPreCommands(target, secrets)
	session, err := probing.OpenTelnetSession(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		PreCommands: preRendered, RawPreCommands: cfg.PreCommands,
		MaxReadBytes: 280000,
	})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer session.Close()

	var allEntries []SerialSearchOnuEntry
	var outputs []string
	cmdRead := 15 * time.Second
	if n := len(ponsToQuery); n > 0 {
		if avg := timeout / time.Duration(n); avg > 5*time.Second && avg < cmdRead {
			cmdRead = avg
		}
	}

	for _, pon := range ponsToQuery {
		if ctx.Err() != nil {
			res.Error = ctx.Err().Error()
			break
		}
		t := target
		t.Pon = pon
		cmd := cfg.RenderListSerialSearchCommand(t, secrets)
		if cmd == "" {
			continue
		}
		script := session.ExecCommands([]string{cmd}, cmdRead)
		stepOut := script.Output
		if stepOut == "" && len(script.Steps) > 0 {
			stepOut = script.Steps[len(script.Steps)-1].Output
		}
		entry := map[string]any{"command": cmd, "output": stepOut, "ok": script.OK}
		if pon > 0 {
			entry["pon"] = pon
		}
		res.Commands = append(res.Commands, entry)
		outputs = append(outputs, stepOut)
		if script.OK {
			allEntries = append(allEntries, ParseOnuListFromTelnetOutput(stepOut)...)
		}
	}

	res.Output = strings.TrimSpace(strings.Join(outputs, "\n\n"))
	res.Matches = FilterSerialSearchEntries(allEntries, target.Serial, target.Pon)
	res.OK = len(res.Matches) > 0
	if !res.OK && res.Error == "" {
		if len(allEntries) == 0 {
			res.Error = "nenhuma ONU encontrada na listagem telnet"
		} else {
			res.Error = fmt.Sprintf("serial %q não encontrado na listagem (%d ONUs analisadas)", target.Serial, len(allEntries))
		}
	}
	return res
}

func resolvePonsForSerialSearch(ponFilter int, ponIndexes []int) []int {
	if ponFilter > 0 {
		return []int{ponFilter}
	}
	if len(ponIndexes) > 0 {
		return ponIndexes
	}
	out := make([]int, 0, 16)
	for i := 1; i <= 16; i++ {
		out = append(out, i)
	}
	return out
}

func runDirectSerialSearch(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	target OnuReportTarget,
	timeout time.Duration,
) SerialSearchRunResult {
	res := SerialSearchRunResult{Mode: "direct"}
	lookupCmd := cfg.RenderSerialSearchCommand(target, secrets)
	preRendered := cfg.RenderPreCommands(target, secrets)
	tel := probing.TelnetRunCommand(ctx, probing.TelnetRunParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		Command: lookupCmd, PreCommands: preRendered, MaxReadBytes: 280000,
	})
	res.Command = lookupCmd
	res.Output = tel.Output
	res.OK = tel.OK
	if !tel.OK {
		res.Error = tel.Error
		return res
	}
	gponOnu := ParseGponOnuFromOutput(tel.Output)
	pon, onu := 0, 0
	if gponOnu != "" {
		pon, onu = ParsePonOnuFromGponOnu(gponOnu)
	}
	entry := SerialSearchOnuEntry{Serial: target.Serial, GponOnu: gponOnu, Pon: pon, Onu: onu}
	parsed := ExtractTelnetKVFieldsPublic(tel.Output)
	if v := parsed["SN"]; v != "" {
		entry.Serial = v
	}
	if v := parsed["Modelo"]; v != "" {
		entry.Model = v
	}
	if target.Pon > 0 && entry.Pon > 0 && entry.Pon != target.Pon {
		res.OK = false
		res.Error = fmt.Sprintf("ONU encontrada na PON %d, não na PON %d filtrada", entry.Pon, target.Pon)
		return res
	}
	if entry.Pon > 0 || entry.Onu > 0 || entry.Serial != "" {
		res.Matches = []SerialSearchOnuEntry{entry}
	}
	return res
}

// FirstSerialSearchMatch devolve a primeira correspondência ou entrada vazia.
func FirstSerialSearchMatch(res SerialSearchRunResult) SerialSearchOnuEntry {
	if len(res.Matches) > 0 {
		return res.Matches[0]
	}
	return SerialSearchOnuEntry{}
}
