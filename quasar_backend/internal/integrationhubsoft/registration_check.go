package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Conferência de cadastros (somente leitura) -------------------------------------------------------
//
// Recebe as MESMAS linhas do CSV de importação (clientes ou serviços adicionais) e, para cada uma,
// consulta a HubSoft e compara campo a campo o que está cadastrado com o que o CSV diz. Nunca altera
// nada. Cada campo sai como:
//   ok          — a HubSoft tem exatamente o valor esperado (texto sem diferenciar caixa/acento/espaços;
//                 login e senha COM diferenciação de maiúsculas/minúsculas);
//   diff        — a HubSoft tem outro valor;
//   unverified  — a consulta da HubSoft não devolve esse dado (ou o catálogo para traduzir o ID não
//                 pôde ser lido): nunca é contado como certo nem como errado.

const (
	CheckOK         = "ok"
	CheckDiff       = "diff"
	CheckUnverified = "unverified"

	RowOK           = "ok"
	RowDivergent    = "divergente"
	RowNotFound     = "nao_encontrado"
	RowAmbiguous    = "ambiguo"
	RowLookupFailed = "erro"
)

// Modos de conferência: "completa" confere todos os campos; "especifica" confere só nome, CPF/CNPJ, login,
// senha, valor e velocidade do plano.
const (
	ModeComplete = "completa"
	ModeSpecific = "especifica"
)

var specificFields = map[string]bool{
	"nome_razaosocial": true, "cpf_cnpj": true, "login": true, "senha": true, "valor": true, "velocidade_plano": true,
}

func filterChecksByMode(checks []CheckItem, mode string) []CheckItem {
	if mode != ModeSpecific {
		return checks
	}
	out := checks[:0:0]
	for _, c := range checks {
		if specificFields[c.Field] {
			out = append(out, c)
		}
	}
	return out
}

type CheckItem struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Group    string `json:"group"` // cliente | endereco | servico | acesso
	Expected string `json:"expected"`
	Found    string `json:"found"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
}

type RowCheck struct {
	Line       int         `json:"line"`
	Label      string      `json:"label"`
	Status     string      `json:"status"`
	Message    string      `json:"message,omitempty"`
	IDCliente  string      `json:"id_cliente,omitempty"`
	IDServico  string      `json:"id_cliente_servico,omitempty"`
	Checks     []CheckItem `json:"checks,omitempty"`
	OKCount    int         `json:"ok_count"`
	DiffCount  int         `json:"diff_count"`
	Unverified int         `json:"unverified_count"`
}

// CheckLabels traduz os IDs de catálogo do CSV (plano, status, vendedor, interface) para os textos que a
// consulta de cliente devolve. Cada ID aponta para uma lista de textos aceitáveis. Missing lista os
// catálogos que não deu para ler — os campos correspondentes ficam "unverified".
type CheckLabels struct {
	Servico   map[string][]string `json:"-"`
	Status    map[string][]string `json:"-"`
	Vendedor  map[string][]string `json:"-"`
	Interface map[string][]string `json:"-"` // id_interface_conexao -> [nome da interface, nome do equipamento]
	Missing   []string            `json:"missing,omitempty"`
}

var checkLabelKeys = map[string][]string{
	"servico":        {"descricao", "nome", "name", "numero_plano"},
	"servico_status": {"descricao", "nome", "prefixo", "name"},
	"vendedor":       {"name", "nome", "descricao"},
}

func catalogLabelsFromJSON(which string, body []byte) map[string][]string {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return nil
	}
	idKey, arrKey := idKeyByCatalog[which], arrKeyByCatalog[which]
	arr, present := root[arrKey].([]any)
	if idKey == "" || !present {
		return nil
	}
	out := map[string][]string{}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id := pickStr(m, idKey)
		if id == "" {
			continue
		}
		for _, k := range checkLabelKeys[which] {
			if v := pickStr(m, k); v != "" {
				out[id] = append(out[id], v)
			}
		}
	}
	return out
}

func interfaceLabelsFromJSON(body []byte) map[string][]string {
	var doc struct {
		Equipamentos []struct {
			Nome       string `json:"nome"`
			Interfaces []struct {
				ID   json.Number `json:"id_interface_conexao"`
				Nome string      `json:"nome"`
			} `json:"interfaces"`
		} `json:"equipamentos"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.Equipamentos == nil {
		return nil
	}
	out := map[string][]string{}
	for _, eq := range doc.Equipamentos {
		for _, ifc := range eq.Interfaces {
			out[ifc.ID.String()] = []string{ifc.Nome, eq.Nome}
		}
	}
	return out
}

var (
	checkLabelsMu    sync.Mutex
	checkLabelsCache = map[string]checkLabelsEntry{}
)

type checkLabelsEntry struct {
	at     time.Time
	labels CheckLabels
}

// LoadCheckLabels lê (com cache de 10 min por conta) os catálogos usados para traduzir IDs.
func LoadCheckLabels(ctx context.Context, cfg Config, token string) CheckLabels {
	checkLabelsMu.Lock()
	if e, ok := checkLabelsCache[cfg.BaseURL]; ok && time.Since(e.at) < 10*time.Minute {
		checkLabelsMu.Unlock()
		return e.labels
	}
	checkLabelsMu.Unlock()

	var l CheckLabels
	load := func(which, human string) map[string][]string {
		_, body, err := FetchCatalog(ctx, cfg, token, which)
		if err != nil {
			l.Missing = append(l.Missing, human)
			return nil
		}
		var m map[string][]string
		if which == "equipamento" {
			m = interfaceLabelsFromJSON(body)
		} else {
			m = catalogLabelsFromJSON(which, body)
		}
		if m == nil {
			l.Missing = append(l.Missing, human)
		}
		return m
	}
	l.Servico = load("servico", "planos")
	l.Status = load("servico_status", "status de serviço")
	l.Vendedor = load("vendedor", "vendedores")
	l.Interface = load("equipamento", "interfaces de conexão")
	if len(l.Missing) == 0 { // não guarda resultado incompleto: a próxima chamada tenta de novo
		checkLabelsMu.Lock()
		checkLabelsCache[cfg.BaseURL] = checkLabelsEntry{at: time.Now(), labels: l}
		checkLabelsMu.Unlock()
	}
	return l
}

// --- normalização ------------------------------------------------------------------------------------

func normText(s string) string {
	s = strings.ToLower(stripAccents(strings.TrimSpace(s)))
	return strings.Join(strings.Fields(s), " ")
}

func sameDigitsPhone(a, b string) bool {
	a, b = onlyDigitsBulk(a), onlyDigitsBulk(b)
	trim := func(x string) string {
		if len(x) > 11 && strings.HasPrefix(x, "55") {
			return x[2:]
		}
		return x
	}
	return trim(a) == trim(b)
}

// parseYesNo — "sim/não/true/false/1/0" (com ou sem acento/caixa) → bool.
func parseYesNo(s string) (val, ok bool) {
	switch normText(s) {
	case "sim", "s", "true", "1", "yes":
		return true, true
	case "nao", "n", "false", "0", "no":
		return false, true
	}
	return false, false
}

func parseFloatLoose(s string) (float64, bool) {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func sameFloat(a, b string, tol float64) (bool, bool) {
	x, ok1 := parseFloatLoose(a)
	y, ok2 := parseFloatLoose(b)
	if !ok1 || !ok2 {
		return false, false
	}
	return math.Abs(x-y) <= tol, true
}

// labelMatches — algum texto aceitável do catálogo bate (igual ou contido, sem caixa/acento) com algum
// dos textos que a consulta devolveu. Tolerante de propósito: "(10) 200MB FIBRA" ≈ "200MB FIBRA".
func labelMatches(cands []string, found ...string) bool {
	for _, c := range cands {
		nc := normText(c)
		if nc == "" {
			continue
		}
		for _, f := range found {
			nf := normText(f)
			if nf != "" && (nf == nc || strings.Contains(nf, nc) || strings.Contains(nc, nf)) {
				return true
			}
		}
	}
	return false
}

// --- esperado (o que o CSV diz) ----------------------------------------------------------------------

type expectedReg struct {
	IsClient bool
	// cliente
	Nome, TipoPessoa, CPFCNPJ, Telefone, Email, RG, IE string
	DataNasc                                           []string // datas aceitáveis (AAAA-MM-DD)
	DataNascNote                                       string
	// endereço
	AddrInstalacao                                                    bool // serviço adicional compara com o endereço de instalação
	CEP, Bairro, Logradouro, Numero, Complemento, Latitude, Longitude string
	// serviço
	IDServico, IDVencimento, IDVendedor, IDFormaCobranca, IDStatus, IDInterface string
	Valor, DataVenda, Carne, Referencia, Login, Senha                           string
}

func expectedFromClientRow(r ClientImportRow, now time.Time) expectedReg {
	orig := strings.TrimSpace(r.DataNascimento)
	r, defaulted := applyClientDefaults(r)
	e := expectedReg{
		IsClient: true, Nome: r.NomeRazaoSocial, TipoPessoa: r.TipoPessoa, CPFCNPJ: r.CPFCNPJ,
		Telefone: r.TelefonePrimario, Email: r.EmailPrincipal, RG: r.RG, IE: r.InscricaoEstadual,
		CEP: r.EnderecoCEP, Bairro: r.EnderecoBairro, Logradouro: r.EnderecoLogradouro, Numero: r.EnderecoNumero,
		Complemento: r.EnderecoComplemento, Latitude: r.EnderecoLatitude, Longitude: r.EnderecoLongitude,
		IDServico: r.IDServico, IDVencimento: r.IDVencimento, IDVendedor: r.IDUsuarioVendedor,
		IDFormaCobranca: r.IDFormaCobranca, IDStatus: r.IDServicoStatus, IDInterface: r.IDInterfaceConexao,
		Valor: r.Valor, DataVenda: r.DataVenda, Carne: r.Carne, Referencia: r.Referencia, Login: r.Login, Senha: r.Senha,
	}
	if d, _, ok := normalizeDate(r.DataNascimento); ok {
		e.DataNasc = []string{d}
	}
	switch {
	case defaulted:
		e.DataNascNote = "CSV sem data — o sistema usa 01/01/1900"
	case isUnderage(orig, now):
		e.DataNasc = append(e.DataNasc, FallbackDataNascimentoMenor)
		e.DataNascNote = "menor de 18 anos — a HubSoft pode ter recebido 01/01/1990"
	}
	return e
}

func expectedFromServiceRow(r ServiceImportRow) expectedReg {
	return expectedReg{
		AddrInstalacao: true,
		CEP:            r.EnderecoInstalacaoCEP, Bairro: r.EnderecoInstalacaoBairro, Logradouro: r.EnderecoInstalacaoLogradouro,
		Numero: r.EnderecoInstalacaoNumero, Complemento: r.EnderecoInstalacaoComplemento,
		Latitude: r.EnderecoInstalacaoLatitude, Longitude: r.EnderecoInstalacaoLongitude,
		IDServico: r.IDServico, IDVencimento: r.IDVencimento, IDVendedor: r.IDUsuarioVendedor,
		IDFormaCobranca: r.IDFormaCobranca, IDStatus: r.IDServicoStatus, IDInterface: r.IDInterfaceConexao,
		Valor: r.Valor, DataVenda: r.DataVenda, Carne: r.Carne, Referencia: r.Referencia, Login: r.Login, Senha: r.Senha,
	}
}

// --- consulta ----------------------------------------------------------------------------------------

// fetchClientsRaw — GET /cliente (busca=<campo>) devolvendo os mapas crus da HubSoft, incluindo
// cancelados/inativos, para o comparador poder ler qualquer campo.
func fetchClientsRaw(ctx context.Context, cfg Config, token, busca, termo string) ([]map[string]any, error) {
	// Mesmos parâmetros da consulta "detalhada" do sistema: SEM relacoes=endereco_instalacao a HubSoft
	// não devolve o endereço do serviço (a conferência chegou a mostrar todo endereço como vazio).
	q := SearchClientsQueryOverrides(false)
	q["busca"], q["termo_busca"] = busca, termo
	q["cancelado"] = "todos" // conferir também serviços cancelados
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente", QueryParams: paramKVs(q),
	})
	body := ResponseBodyBytes(res)
	if !res.OK {
		if res.StatusCode == 0 {
			return nil, fmt.Errorf("sem resposta da HubSoft")
		}
		return nil, fmt.Errorf("%s", firstNonEmpty(hubsoftActionMessageWithErrors(body), fmt.Sprintf("HTTP %d", res.StatusCode)))
	}
	var doc struct {
		Clientes []map[string]any `json:"clientes"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("resposta inesperada da HubSoft: %w", err)
	}
	return doc.Clientes, nil
}

func servicesOf(client map[string]any) []map[string]any {
	arr, _ := client["servicos"].([]any)
	var out []map[string]any
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// pickService escolhe, entre os serviços do cliente, o que corresponde à linha: login exato → login
// com outra caixa → (sem login no CSV) o único serviço → o único ainda com o login padrão da máscara.
// ambiguous=true quando não dá para escolher com segurança.
func pickService(svcs []map[string]any, login string) (svc map[string]any, ambiguous bool) {
	login = strings.TrimSpace(login)
	if login != "" {
		for _, s := range svcs {
			if pickStr(s, "login") == login {
				return s, false
			}
		}
		for _, s := range svcs {
			if strings.EqualFold(pickStr(s, "login"), login) {
				return s, false
			}
		}
	} else if len(svcs) == 1 {
		return svcs[0], false
	} else if len(svcs) > 1 {
		return nil, true
	}
	var ph []map[string]any
	for _, s := range svcs {
		if strings.EqualFold(pickStr(s, "login"), PlaceholderLogin) {
			ph = append(ph, s)
		}
	}
	if len(ph) == 1 {
		return ph[0], false
	}
	if len(ph) > 1 {
		return nil, true
	}
	return nil, false
}

func serviceLogins(svcs []map[string]any) string {
	var l []string
	for _, s := range svcs {
		if v := pickStr(s, "login"); v != "" {
			l = append(l, v)
		}
	}
	if len(l) == 0 {
		return "nenhum"
	}
	return strings.Join(l, ", ")
}

// CheckClientRow confere uma linha do CSV de "clientes novos" (conferência completa).
func CheckClientRow(ctx context.Context, cfg Config, token string, r ClientImportRow, labels CheckLabels) RowCheck {
	return CheckClientRowMode(ctx, cfg, token, r, labels, ModeComplete)
}

// CheckClientRowMode — como CheckClientRow, no modo pedido (ModeComplete | ModeSpecific).
func CheckClientRowMode(ctx context.Context, cfg Config, token string, r ClientImportRow, labels CheckLabels, mode string) RowCheck {
	rc := RowCheck{Line: r.Line, Label: r.NomeRazaoSocial}
	e := expectedFromClientRow(r, time.Now())
	cpf, _ := normalizeCPFCNPJ(r.TipoPessoa, r.CPFCNPJ)
	if cpf == "" {
		rc.Status, rc.Message = RowLookupFailed, "linha sem CPF/CNPJ — não dá para localizar o cliente"
		return rc
	}
	clients, err := fetchClientsRaw(ctx, cfg, token, "cpf_cnpj", cpf)
	if err != nil {
		rc.Status, rc.Message = RowLookupFailed, "falha ao consultar a HubSoft: "+err.Error()
		return rc
	}
	var client map[string]any
	for _, c := range clients {
		if onlyDigitsBulk(pickStr(c, "cpf_cnpj")) == cpf {
			client = c
			break
		}
	}
	foundBy := ""
	if client == nil && strings.TrimSpace(r.Login) != "" {
		// Não achou pelo CPF — o cadastro pode existir com outro CPF. Procura pelo login e compara mesmo
		// assim: o campo CPF/CNPJ vai aparecer como divergente.
		byLogin, lerr := fetchClientsRaw(ctx, cfg, token, "login_radius", strings.TrimSpace(r.Login))
		if lerr == nil {
			for _, c := range byLogin {
				for _, s := range servicesOf(c) {
					if strings.EqualFold(pickStr(s, "login"), strings.TrimSpace(r.Login)) {
						client, foundBy = c, "login"
					}
				}
				if client != nil {
					break
				}
			}
		}
	}
	if client == nil {
		rc.Status, rc.Message = RowNotFound, "nenhum cliente com este CPF/CNPJ na HubSoft"
		return rc
	}
	return finishCheck(rc, client, foundBy, r.Login, e, labels, mode)
}

// CheckServiceRow confere uma linha do CSV de "serviços adicionais" (cliente já existente), modo completo.
func CheckServiceRow(ctx context.Context, cfg Config, token string, r ServiceImportRow, labels CheckLabels) RowCheck {
	return CheckServiceRowMode(ctx, cfg, token, r, labels, ModeComplete)
}

// CheckServiceRowMode — como CheckServiceRow, no modo pedido.
func CheckServiceRowMode(ctx context.Context, cfg Config, token string, r ServiceImportRow, labels CheckLabels, mode string) RowCheck {
	rc := RowCheck{Line: r.Line, Label: "id_cliente " + r.IDCliente}
	if r.Login != "" {
		rc.Label += " · " + r.Login
	}
	idc := strings.TrimSpace(r.IDCliente)
	if idc == "" {
		rc.Status, rc.Message = RowLookupFailed, "linha sem id_cliente"
		return rc
	}
	clients, err := fetchClientsRaw(ctx, cfg, token, "id_cliente", idc)
	if err != nil {
		rc.Status, rc.Message = RowLookupFailed, "falha ao consultar a HubSoft: "+err.Error()
		return rc
	}
	var client map[string]any
	for _, c := range clients {
		if pickStr(c, "id_cliente") == idc {
			client = c
			break
		}
	}
	if client == nil {
		rc.Status, rc.Message = RowNotFound, "nenhum cliente com este id_cliente na HubSoft"
		return rc
	}
	if n := pickStr(client, "nome_razaosocial"); n != "" {
		rc.Label = n + " · " + firstNonEmpty(r.Login, "id_cliente "+idc)
	}
	return finishCheck(rc, client, "", r.Login, expectedFromServiceRow(r), labels, mode)
}

func finishCheck(rc RowCheck, client map[string]any, foundBy, login string, e expectedReg, labels CheckLabels, mode string) RowCheck {
	rc.IDCliente = pickStr(client, "id_cliente")
	svcs := servicesOf(client)
	svc, ambiguous := pickService(svcs, login)
	switch {
	case ambiguous:
		rc.Status = RowAmbiguous
		rc.Message = fmt.Sprintf("o cliente tem mais de um serviço possível e não dá para escolher com segurança (logins: %s)", serviceLogins(svcs))
		rc.Checks = buildChecks(e, client, nil, labels, foundBy)
	case svc == nil:
		rc.Status = RowNotFound
		rc.Message = fmt.Sprintf("o cliente existe (id_cliente %s), mas nenhum serviço tem o login %q (logins no cadastro: %s)", rc.IDCliente, strings.TrimSpace(login), serviceLogins(svcs))
		rc.Checks = buildChecks(e, client, nil, labels, foundBy)
	default:
		rc.IDServico = pickStr(svc, "id_cliente_servico")
		rc.Checks = buildChecks(e, client, svc, labels, foundBy)
	}
	rc.Checks = filterChecksByMode(rc.Checks, mode)
	for _, c := range rc.Checks {
		switch c.Status {
		case CheckOK:
			rc.OKCount++
		case CheckDiff:
			rc.DiffCount++
		default:
			rc.Unverified++
		}
	}
	if rc.Status == "" {
		if rc.DiffCount > 0 {
			rc.Status = RowDivergent
		} else {
			rc.Status = RowOK
		}
	}
	if foundBy == "login" {
		rc.Message = strings.TrimSpace(rc.Message + " Cliente localizado pelo login (o CPF/CNPJ do cadastro é outro).")
	}
	return rc
}

// --- comparação campo a campo ------------------------------------------------------------------------

func addrOf(svc, client map[string]any, instalacao bool) map[string]any {
	keys := []string{"endereco_cadastral", "endereco_instalacao", "endereco"}
	if instalacao {
		keys = []string{"endereco_instalacao", "endereco_cadastral", "endereco"}
	}
	for _, src := range []map[string]any{svc, client} {
		if src == nil {
			continue
		}
		for _, k := range keys {
			if m, ok := src[k].(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

func buildChecks(e expectedReg, client, svc map[string]any, labels CheckLabels, foundBy string) []CheckItem {
	var out []CheckItem
	add := func(group, field, label, expected, found, status, note string) {
		out = append(out, CheckItem{Field: field, Label: label, Group: group, Expected: expected, Found: found, Status: status, Note: note})
	}
	textCheck := func(group, field, label, expected, found string) {
		if strings.TrimSpace(expected) == "" {
			return
		}
		if strings.TrimSpace(found) == "" {
			add(group, field, label, expected, "", CheckDiff, "vazio na HubSoft")
			return
		}
		if normText(expected) == normText(found) {
			add(group, field, label, expected, found, CheckOK, "")
		} else {
			add(group, field, label, expected, found, CheckDiff, "")
		}
	}
	unverified := func(group, field, label, expected, why string) {
		if strings.TrimSpace(expected) != "" {
			add(group, field, label, expected, "", CheckUnverified, why)
		}
	}

	if e.IsClient {
		textCheck("cliente", "nome_razaosocial", "Nome / razão social", e.Nome, pickStr(client, "nome_razaosocial"))
		if e.TipoPessoa != "" {
			f := pickStr(client, "tipo_pessoa")
			st := CheckDiff
			if strings.EqualFold(strings.TrimSpace(e.TipoPessoa), f) {
				st = CheckOK
			}
			add("cliente", "tipo_pessoa", "Tipo de pessoa", e.TipoPessoa, f, st, "")
		}
		if e.CPFCNPJ != "" {
			exp, _ := normalizeCPFCNPJ(e.TipoPessoa, e.CPFCNPJ)
			f := pickStr(client, "cpf_cnpj")
			st := CheckDiff
			if onlyDigitsBulk(f) == exp {
				st = CheckOK
			}
			note := ""
			if foundBy == "login" {
				note = "cadastro localizado pelo login"
			}
			add("cliente", "cpf_cnpj", "CPF/CNPJ", exp, f, st, note)
		}
		if e.Telefone != "" {
			f := pickStr(client, "telefone_primario")
			st := CheckDiff
			if sameDigitsPhone(e.Telefone, f) {
				st = CheckOK
			}
			add("cliente", "telefone_primario", "Telefone", onlyDigitsBulk(e.Telefone), f, st, "")
		}
		if e.Email != "" {
			f := pickStr(client, "email_principal")
			st := CheckDiff
			if strings.EqualFold(strings.TrimSpace(e.Email), f) {
				st = CheckOK
			}
			add("cliente", "email_principal", "E-mail", e.Email, f, st, "")
		}
		if len(e.DataNasc) > 0 {
			f := dvNormDate(firstNonEmpty(pickStr(client, "data_nascimento"), pickStr(client, "data_nascmento")))
			st := CheckDiff
			for _, d := range e.DataNasc {
				if f == d {
					st = CheckOK
				}
			}
			add("cliente", "data_nascimento", "Data de nascimento", strings.Join(e.DataNasc, " ou "), f, st, e.DataNascNote)
		}
		textCheck("cliente", "rg", "RG", e.RG, pickStr(client, "rg"))
		textCheck("cliente", "inscricao_estadual", "Inscrição estadual", e.IE, pickStr(client, "inscricao_estadual"))
	}

	// Endereço
	if addr := addrOf(svc, client, e.AddrInstalacao); addr != nil || e.CEP != "" {
		var coords map[string]any
		if addr != nil {
			coords, _ = addr["coordenadas"].(map[string]any)
		}
		if e.CEP != "" {
			f := pickStr(addr, "cep")
			st := CheckDiff
			if onlyDigitsBulk(f) == onlyDigitsBulk(e.CEP) {
				st = CheckOK
			}
			add("endereco", "endereco_cep", "CEP", onlyDigitsBulk(e.CEP), f, st, "")
		}
		textCheck("endereco", "endereco_bairro", "Bairro", e.Bairro, pickStr(addr, "bairro"))
		textCheck("endereco", "endereco_logradouro", "Logradouro", e.Logradouro, pickStr(addr, "endereco"))
		textCheck("endereco", "endereco_numero", "Número", e.Numero, pickStr(addr, "numero"))
		textCheck("endereco", "endereco_complemento", "Complemento", e.Complemento, pickStr(addr, "complemento"))
		for _, c := range []struct{ f, l, exp, key string }{
			{"endereco_latitude", "Latitude", e.Latitude, "latitude"}, {"endereco_longitude", "Longitude", e.Longitude, "longitude"},
		} {
			if strings.TrimSpace(c.exp) == "" {
				continue
			}
			f := pickStr(coords, c.key)
			if eq, ok := sameFloat(c.exp, f, 0.00005); !ok {
				add("endereco", c.f, c.l, c.exp, f, CheckDiff, "vazio ou inválido na HubSoft")
			} else if eq {
				add("endereco", c.f, c.l, c.exp, f, CheckOK, "")
			} else {
				add("endereco", c.f, c.l, c.exp, f, CheckDiff, "")
			}
		}
	}

	// Serviço
	const none = "serviço não localizado"
	if svc == nil {
		unverified("servico", "login", "Login", e.Login, none)
		return out
	}
	// plano / status / vendedor / interface: ID do CSV → texto do catálogo → texto do cadastro
	idCheck := func(field, label, id string, cat map[string][]string, missingName string, found ...string) {
		if strings.TrimSpace(id) == "" {
			return
		}
		cands, ok := cat[id]
		if cat == nil {
			unverified("servico", field, label, "id "+id, "catálogo de "+missingName+" não pôde ser lido")
			return
		}
		if !ok || len(cands) == 0 {
			add("servico", field, label, "id "+id, strings.Join(found, " / "), CheckDiff, "este id não existe no catálogo da HubSoft")
			return
		}
		st := CheckDiff
		if labelMatches(cands, found...) {
			st = CheckOK
		}
		add("servico", field, label, "id "+id+" ("+strings.Join(cands, " / ")+")", strings.Join(found, " / "), st, "")
	}
	vend, _ := svc["vendedor"].(map[string]any)
	iface, _ := svc["interface"].(map[string]any)
	eqc, _ := svc["equipamento_conexao"].(map[string]any)
	idCheck("id_servico", "Plano", e.IDServico, labels.Servico, "planos", pickStr(svc, "nome"), pickStr(svc, "numero_plano"))
	speedCheck(add, e.IDServico, labels.Servico, svc)
	idCheck("id_servico_status", "Status do serviço", e.IDStatus, labels.Status, "status de serviço", pickStr(svc, "status"), pickStr(svc, "status_prefixo"))
	idCheck("id_usuario_vendedor", "Vendedor", e.IDVendedor, labels.Vendedor, "vendedores", pickStr(vend, "nome"))
	if strings.TrimSpace(e.IDInterface) != "" {
		if iface == nil && eqc == nil {
			unverified("servico", "id_interface_conexao", "Interface de conexão", "id "+e.IDInterface, "a consulta não trouxe a interface do serviço")
		} else {
			idCheck("id_interface_conexao", "Interface de conexão", e.IDInterface, labels.Interface, "interfaces de conexão", pickStr(iface, "nome"))
			// o equipamento (OLT/POP) também precisa bater: a mesma interface pode existir em outro equipamento
			if cands := labels.Interface[e.IDInterface]; len(cands) == 2 && pickStr(eqc, "nome") != "" {
				st := CheckDiff
				if normText(cands[1]) == normText(pickStr(eqc, "nome")) {
					st = CheckOK
				}
				add("servico", "equipamento_conexao", "Equipamento de conexão", cands[1], pickStr(eqc, "nome"), st, "")
			}
		}
	}
	// vencimento / forma de cobrança: só confere se a consulta devolver o ID
	for _, c := range []struct {
		field, label, id string
		keys             []string
	}{
		{"id_vencimento", "Vencimento", e.IDVencimento, []string{"id_vencimento"}},
		{"id_forma_cobranca", "Forma de cobrança", e.IDFormaCobranca, []string{"id_forma_cobranca"}},
	} {
		if strings.TrimSpace(c.id) == "" {
			continue
		}
		f := pickStr(svc, c.keys...)
		switch {
		case f == "":
			add("servico", c.field, c.label, "id "+c.id, "", CheckUnverified, "a consulta de cliente da HubSoft não devolve este dado")
		case f == strings.TrimSpace(c.id):
			add("servico", c.field, c.label, "id "+c.id, f, CheckOK, "")
		default:
			add("servico", c.field, c.label, "id "+c.id, f, CheckDiff, "")
		}
	}
	if e.Valor != "" {
		f := pickStr(svc, "valor")
		if eq, ok := sameFloat(e.Valor, f, 0.005); !ok {
			add("servico", "valor", "Valor", e.Valor, f, CheckDiff, "vazio ou inválido na HubSoft")
		} else if eq {
			add("servico", "valor", "Valor", e.Valor, f, CheckOK, "")
		} else {
			add("servico", "valor", "Valor", e.Valor, f, CheckDiff, "")
		}
	}
	if d, _, ok := normalizeDate(e.DataVenda); ok {
		f := dvNormDate(pickStr(svc, "data_venda"))
		st := CheckDiff
		if f == d {
			st = CheckOK
		}
		add("servico", "data_venda", "Data da venda", d, f, st, "")
	}
	if e.Carne != "" {
		f := pickStr(svc, "carne") // a HubSoft devolve "Sim"/"Não" (texto), não true/false
		want, wok := parseYesNo(e.Carne)
		got, gok := parseYesNo(f)
		switch {
		case f == "":
			add("servico", "carne", "Carnê", e.Carne, "", CheckUnverified, "a consulta não devolve este dado")
		case wok && gok && want == got:
			add("servico", "carne", "Carnê", e.Carne, f, CheckOK, "")
		case !wok || !gok:
			add("servico", "carne", "Carnê", e.Carne, f, CheckUnverified, "valor em formato não reconhecido")
		default:
			add("servico", "carne", "Carnê", e.Carne, f, CheckDiff, "")
		}
	}
	textCheck("servico", "referencia", "Referência", e.Referencia, pickStr(svc, "referencia"))

	// Acesso PPPoE — login e senha EXATOS (maiúsculas/minúsculas contam).
	if e.Login != "" {
		f := pickStr(svc, "login")
		switch {
		case f == e.Login:
			add("acesso", "login", "Login", e.Login, f, CheckOK, "")
		case strings.EqualFold(f, e.Login):
			// A HubSoft padroniza em minúsculas e o Radius dela não diferencia caixa — não é divergência.
			add("acesso", "login", "Login", e.Login, f, CheckOK, "a HubSoft padronizou em minúsculas (o Radius não diferencia maiúsculas)")
			obs := pickStr(svc, "observacoes_autenticacao")
			want := loginObservationText(e.Login)
			if strings.Contains(obs, want) {
				add("acesso", "observacoes_login", "Observação do login original", want, obs, CheckOK, "")
			} else {
				add("acesso", "observacoes_login", "Observação do login original", want, obs, CheckDiff, "o login original ainda não foi registrado nas Observações da autenticação")
			}
		case strings.EqualFold(f, PlaceholderLogin):
			add("acesso", "login", "Login", e.Login, f, CheckDiff, "ainda está com o login padrão da máscara")
		default:
			add("acesso", "login", "Login", e.Login, f, CheckDiff, "")
		}
	}
	if e.Senha != "" {
		f := pickStr(svc, "senha")
		switch {
		case f == "":
			add("acesso", "senha", "Senha", "(informada no CSV)", "", CheckUnverified, "a HubSoft não devolveu a senha na consulta")
		case senhaLooksHashed(f):
			add("acesso", "senha", "Senha", "(informada no CSV)", "(oculta)", CheckUnverified, "a HubSoft devolveu a senha mascarada")
		case f == e.Senha:
			add("acesso", "senha", "Senha", e.Senha, f, CheckOK, "")
		case strings.EqualFold(f, e.Senha):
			add("acesso", "senha", "Senha", e.Senha, f, CheckDiff, "só difere em maiúsculas/minúsculas")
		case f == PlaceholderPassword:
			add("acesso", "senha", "Senha", e.Senha, f, CheckDiff, "ainda está com a senha padrão da máscara")
		default:
			add("acesso", "senha", "Senha", e.Senha, f, CheckDiff, "")
		}
	}
	return out
}

// --- velocidade do plano ----------------------------------------------------------------------------------

var (
	speedGigaRe = regexp.MustCompile(`(?i)(\d+)\s*(?:GIGA|GB|G)\b`)
	speedMegaRe = regexp.MustCompile(`(?i)(\d+)\s*(?:MB|MEGAS?|MBPS|M)\b`)
	speedHubRe  = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(Gbits?|Mbits?|Kbits?)`)
)

// speedFromPlanName — velocidade (Mbps) pelo nome do plano: "300 MB - PÓS PAGO (G2)" → 300; "1 GIGA" → 1000.
func speedFromPlanName(name string) (int, bool) {
	if m := speedGigaRe.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n * 1000, n > 0
	}
	if m := speedMegaRe.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, n > 0
	}
	return 0, false
}

// speedFromHubsoft — "350 Mbits" → 350 (Mbps).
func speedFromHubsoft(s string) (float64, bool) {
	m := speedHubRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[2][:1]) {
	case "g":
		v *= 1000
	case "k":
		v /= 1000
	}
	return v, true
}

// speedMargin — a HubSoft configura a velocidade real do plano acima da do nome (ex.: "300 MB" → "350 Mbits").
const speedMargin = 50.0

// speedCheck compara a velocidade do plano escolhido no CSV (pelo nome no catálogo) com a velocidade que o
// serviço tem na HubSoft. Aceita de 0 até speedMargin Mbps acima da do nome.
func speedCheck(add func(group, field, label, expected, found, status, note string), idServico string, planos map[string][]string, svc map[string]any) {
	if strings.TrimSpace(idServico) == "" {
		return
	}
	const field, label = "velocidade_plano", "Velocidade do plano"
	if planos == nil {
		add("servico", field, label, "plano id "+idServico, "", CheckUnverified, "catálogo de planos não pôde ser lido")
		return
	}
	var want int
	var planName string
	for _, c := range planos[idServico] {
		if n, ok := speedFromPlanName(c); ok {
			want, planName = n, c
			break
		}
	}
	if want == 0 {
		add("servico", field, label, "plano id "+idServico, "", CheckUnverified, "não deu para ler a velocidade no nome do plano")
		return
	}
	expected := fmt.Sprintf("%d Mbps (plano %q)", want, planName)
	down, okd := speedFromHubsoft(pickStr(svc, "velocidade_download"))
	up, oku := speedFromHubsoft(pickStr(svc, "velocidade_upload"))
	if !okd {
		add("servico", field, label, expected, "", CheckUnverified, "a consulta não devolveu a velocidade do serviço")
		return
	}
	found := fmt.Sprintf("%s ↓ / %s ↑", pickStr(svc, "velocidade_download"), pickStr(svc, "velocidade_upload"))
	inRange := func(v float64) bool { return v >= float64(want) && v <= float64(want)+speedMargin }
	if inRange(down) && (!oku || inRange(up)) {
		add("servico", field, label, expected, found, CheckOK, "a HubSoft soma até 50 Mbits de margem ao plano")
	} else {
		add("servico", field, label, expected, found, CheckDiff, "")
	}
}
