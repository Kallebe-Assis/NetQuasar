package oltcollect

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// --- Atualização das ONUs de UMA PON, por comando telnet configurável no perfil da OLT -----------------------------
//
// O perfil (Configurações → OLT) guarda `pon_refresh_command`: um ou mais comandos (um por linha, ou separados por
// ";") com o placeholder {pon}, que listam as ONUs de uma porta — por exemplo
//
//	ZTE : show gpon onu baseinfo gpon-olt_1/1/{pon}      (serial e modelo de TODAS as ONUs, mesmo offline)
//	      show gpon onu state gpon-olt_1/1/{pon}          (estado/fase de cada ONU)
//	VSOL: show onu info {pon}
//
// A saída é interpretada linha a linha (ParsePonRefreshOutput) e fundida só nas ONUs dessa PON do snapshot
// (MergePonRefreshIntoSummary) — sem refazer a coleta completa da OLT.

// PonRefreshCommandList separa pon_refresh_command em comandos individuais (uma linha por comando ou separados por ";").
func (c OnuReportConfig) PonRefreshCommandList() []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(c.PonRefreshCommand, ";", "\n"), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// HasPonRefresh indica se o perfil tem comando para atualizar as ONUs de uma PON.
func (c OnuReportConfig) HasPonRefresh() bool { return len(c.PonRefreshCommandList()) > 0 }

// PonRefreshNeedsEnable — os pré-comandos do perfil pedem a senha enable.
func (c OnuReportConfig) PonRefreshNeedsEnable() bool { return c.NeedsEnablePassword() }

// RenderPonRefreshCommands troca {pon} (e as senhas) em cada comando.
func (c OnuReportConfig) RenderPonRefreshCommands(pon int, sec TelnetSecrets) []string {
	t := OnuReportTarget{Pon: pon}
	var out []string
	for _, tpl := range c.PonRefreshCommandList() {
		if cmd := SubstituteTelnetTemplate(tpl, t, sec); cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

// PonRefreshEntry uma ONU lida da saída do comando por PON.
type PonRefreshEntry struct {
	Pon    int
	Onu    int
	Serial string
	Model  string
	State  string // texto bruto do estado/fase (working, LOS, Online…)
	Online *bool  // nil = a saída não informou o estado
}

var (
	// ZTE baseinfo: gpon-onu_1/1/9:3   ZTE-F670   sn   SN:ZTEGC1234567
	zteOnuIndexLineRE = regexp.MustCompile(`(?i)^(?:gpon[-_]onu[-_])?(\d+)/(\d+)/(\d+):(\d+)\s+(.+)$`)
	zteSnTokenRE      = regexp.MustCompile(`(?i)\bSN:([A-Za-z0-9]{8,})`)
)

// phase → online. Valores da coluna "Phase State" do `show gpon onu state` da ZTE (e equivalentes).
var onuPhaseOnline = map[string]bool{
	"working": true, "online": true, "operation": true,
	"los": false, "dyinggasp": false, "dying-gasp": false, "offline": false, "authfailed": false,
	"syncmib": false, "logging": false, "unknown": false, "disable": false, "disabled": false,
}

func onlineFromState(state string) *bool {
	v, ok := onuPhaseOnline[strings.ToLower(strings.TrimSpace(state))]
	if !ok {
		return nil
	}
	return &v
}

// ParsePonRefreshOutput interpreta a saída dos comandos de listagem por PON: formatos VSOL (show onu info, onu search),
// ZTE baseinfo (serial/modelo) e ZTE state (fase). Entradas da mesma ONU são combinadas.
func ParsePonRefreshOutput(output string) []PonRefreshEntry {
	text := cleanTelnetCLIOutput(output)
	byKey := map[string]*PonRefreshEntry{}
	var order []string
	get := func(pon, onu int) *PonRefreshEntry {
		k := strconv.Itoa(pon) + ":" + strconv.Itoa(onu)
		if e, ok := byKey[k]; ok {
			return e
		}
		e := &PonRefreshEntry{Pon: pon, Onu: onu}
		byKey[k] = e
		order = append(order, k)
		return e
	}

	// 1) formatos já suportados (VSOL e outros): serial, modelo e, quando existir, o estado.
	for _, en := range ParseOnuListFromTelnetOutput(output) {
		if en.Pon <= 0 || en.Onu <= 0 {
			continue
		}
		e := get(en.Pon, en.Onu)
		if IsPlausibleOnuSerial(en.Serial) {
			e.Serial = strings.ToUpper(strings.TrimSpace(en.Serial))
		}
		if e.Model == "" {
			e.Model = strings.TrimSpace(en.Model)
		}
		if o := onlineFromState(en.Mode); o != nil {
			e.State, e.Online = strings.TrimSpace(en.Mode), o
		}
	}

	// 2) ZTE: baseinfo (SN:…) e state (coluna de fase), reconhecidas pelo índice 1/1/9:3 no começo da linha.
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		m := zteOnuIndexLineRE.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		pon, _ := strconv.Atoi(m[3])
		onu, _ := strconv.Atoi(m[4])
		if pon <= 0 || onu <= 0 {
			continue
		}
		rest := strings.TrimSpace(m[5])
		if sn := zteSnTokenRE.FindStringSubmatch(rest); sn != nil {
			e := get(pon, onu)
			e.Serial = strings.ToUpper(sn[1])
			if f := strings.Fields(rest); len(f) > 0 && !strings.HasPrefix(strings.ToUpper(f[0]), "SN:") && e.Model == "" {
				e.Model = f[0]
			}
			continue
		}
		for _, tok := range strings.Fields(rest) {
			if o := onlineFromState(tok); o != nil {
				e := get(pon, onu)
				e.State, e.Online = tok, o
				break
			}
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		a, b := byKey[order[i]], byKey[order[j]]
		if a.Pon != b.Pon {
			return a.Pon < b.Pon
		}
		return a.Onu < b.Onu
	})
	out := make([]PonRefreshEntry, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// PonRefreshRunResult resultado da execução telnet.
type PonRefreshRunResult struct {
	OK       bool
	Commands []map[string]any
	Output   string
	Entries  []PonRefreshEntry
	Error    string
}

// RunPonRefreshTelnet abre UMA sessão telnet (com os pré-comandos do perfil), executa os comandos da PON e interpreta a saída.
func RunPonRefreshTelnet(
	ctx context.Context,
	host, user, password, enable string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	pon int,
	timeout time.Duration,
) PonRefreshRunResult {
	var res PonRefreshRunResult
	cmds := cfg.RenderPonRefreshCommands(pon, secrets)
	if len(cmds) == 0 {
		res.Error = "comando de atualização por PON não configurado no perfil da OLT"
		return res
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	pre := cfg.RenderPreCommands(OnuReportTarget{Pon: pon}, secrets)
	session, err := probing.OpenTelnetSession(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: timeout,
		User: user, Password: password, Enable: enable,
		PreCommands: pre, RawPreCommands: cfg.PreCommands,
		MaxReadBytes: 400000,
	})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer session.Close()

	cmdRead := 20 * time.Second
	if avg := timeout / time.Duration(len(cmds)); avg > 5*time.Second && avg < cmdRead {
		cmdRead = avg
	}
	var outputs []string
	var all []PonRefreshEntry
	for _, cmd := range cmds {
		if ctx.Err() != nil {
			res.Error = ctx.Err().Error()
			break
		}
		script := session.ExecCommands([]string{cmd}, cmdRead)
		out := script.Output
		if out == "" && len(script.Steps) > 0 {
			out = script.Steps[len(script.Steps)-1].Output
		}
		res.Commands = append(res.Commands, map[string]any{"command": cmd, "output": out, "ok": script.OK})
		outputs = append(outputs, out)
		if script.OK {
			all = append(all, ParsePonRefreshOutput(out)...)
		}
	}
	res.Output = strings.TrimSpace(strings.Join(outputs, "\n\n"))
	res.Entries = mergePonRefreshEntries(all, pon)
	res.OK = len(res.Entries) > 0
	if !res.OK && res.Error == "" {
		res.Error = fmt.Sprintf("nenhuma ONU da PON %d reconhecida na saída do comando — confira o comando e o formato da resposta", pon)
	}
	return res
}

// mergePonRefreshEntries junta as leituras de vários comandos (serial de um, fase de outro) e descarta ONUs de outra PON.
func mergePonRefreshEntries(in []PonRefreshEntry, pon int) []PonRefreshEntry {
	byKey := map[int]*PonRefreshEntry{}
	var onus []int
	for _, e := range in {
		if pon > 0 && e.Pon != pon {
			continue
		}
		cur, ok := byKey[e.Onu]
		if !ok {
			c := e
			byKey[e.Onu] = &c
			onus = append(onus, e.Onu)
			continue
		}
		if cur.Serial == "" {
			cur.Serial = e.Serial
		}
		if cur.Model == "" {
			cur.Model = e.Model
		}
		if cur.Online == nil && e.Online != nil {
			cur.Online, cur.State = e.Online, e.State
		}
	}
	sort.Ints(onus)
	out := make([]PonRefreshEntry, 0, len(onus))
	for _, o := range onus {
		out = append(out, *byKey[o])
	}
	return out
}

// PonRefreshMergeStats o que a fusão mudou no snapshot.
type PonRefreshMergeStats struct {
	Entries       int `json:"entries"`
	Updated       int `json:"updated"`        // ONUs existentes que mudaram
	Added         int `json:"added"`          // ONUs novas no snapshot
	SerialsFilled int `json:"serials_filled"` // ONUs que ganharam/trocaram serial
	StateChanged  int `json:"state_changed"`  // ONUs cujo online/offline mudou
	Online        int `json:"online"`         // total da PON depois da fusão
	Offline       int `json:"offline"`
	Total         int `json:"total"`
}

// MergePonRefreshIntoSummary funde as ONUs lidas numa PON em summary["vsol_onu_rows"] e atualiza os totais.
// Só toca nas linhas dessa PON. O estado só é alterado quando a saída o informou; serial só quando plausível.
func MergePonRefreshIntoSummary(summary map[string]any, pon int, entries []PonRefreshEntry, at time.Time) PonRefreshMergeStats {
	var st PonRefreshMergeStats
	st.Entries = len(entries)
	if summary == nil || pon <= 0 || len(entries) == 0 {
		return st
	}
	rows := OnuRowsFromSummary(summary)
	byOnu := map[int]map[string]any{}
	for _, r := range rows {
		if r != nil && intFromRow(r, "pon") == pon {
			byOnu[intFromRow(r, "onu")] = r
		}
	}
	stamp := at.UTC().Format(time.RFC3339)
	for _, e := range entries {
		if e.Onu <= 0 {
			continue
		}
		row, exists := byOnu[e.Onu]
		changed := false
		if !exists {
			row = map[string]any{"pon": pon, "onu": e.Onu}
			rows = append(rows, row)
			byOnu[e.Onu] = row
			st.Added++
		}
		if IsPlausibleOnuSerial(e.Serial) {
			if cur, _ := row["serial"].(string); !strings.EqualFold(strings.TrimSpace(cur), e.Serial) {
				row["serial"] = strings.ToUpper(strings.TrimSpace(e.Serial))
				row["serial_source"] = "pon_refresh"
				st.SerialsFilled++
				changed = true
			}
		}
		if strings.TrimSpace(e.Model) != "" {
			if cur, _ := row["model"].(string); strings.TrimSpace(cur) == "" {
				row["model"] = strings.TrimSpace(e.Model)
				changed = true
			}
		}
		if e.Online != nil {
			if cur, ok := row["online"].(bool); !ok || cur != *e.Online {
				row["online"] = *e.Online
				row["status_source"] = "pon_refresh"
				if *e.Online {
					row["oper_status_label"] = "up"
				} else {
					row["oper_status_label"] = "down"
				}
				st.StateChanged++
				changed = true
			}
			if e.State != "" {
				row["phase_state"] = e.State
			}
		}
		row["pon_refresh_at"] = stamp
		if changed && exists {
			st.Updated++
		}
	}
	arr := make([]any, len(rows))
	for i, r := range rows {
		arr[i] = r
	}
	summary["vsol_onu_rows"] = arr

	// Totais da PON e da OLT, a partir das linhas.
	totalAll, onlineAll, withoutState := 0, 0, 0
	for _, r := range rows {
		if r == nil {
			continue
		}
		totalAll++
		on, hasState := r["online"].(bool)
		if !hasState {
			withoutState++
		}
		if on {
			onlineAll++
		}
		if intFromRow(r, "pon") == pon {
			st.Total++
			if on {
				st.Online++
			}
		}
	}
	st.Offline = st.Total - st.Online
	if _, has := summary["vsol_onu_count"]; has && withoutState == 0 {
		summary["vsol_onu_count"] = totalAll
		summary["vsol_onu_online"] = onlineAll
		summary["vsol_onu_offline"] = totalAll - onlineAll
	}
	m, _ := summary["pon_refresh_at"].(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	m[strconv.Itoa(pon)] = stamp
	summary["pon_refresh_at"] = m
	return st
}

// ApplyPonRefreshToPons acerta os contadores da PON (onu_total/online/offline) no array `pons` do snapshot.
func ApplyPonRefreshToPons(pons []map[string]any, pon int, st PonRefreshMergeStats) {
	if st.Total == 0 {
		return
	}
	for _, p := range pons {
		if p == nil || intFromAny(p["pon"]) != pon {
			continue
		}
		p["onu_total"] = st.Total
		p["onu_online"] = st.Online
		p["onu_offline"] = st.Offline
		p["onu_counts_source"] = "pon_refresh"
	}
}
