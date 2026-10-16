package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Abrir ATENDIMENTO (com Ordem de Serviço junto) na HubSoft — um a um ou em lote (CSV).
//
// A HubSoft não cria O.S. avulsa: toda O.S. nasce de um atendimento. A rota usada é POST /atendimento com abrir_os=true e
// parametros_ordem_servico (tipo da O.S., técnicos, status, agenda). Fluxo seguro, igual às outras ferramentas de escrita:
// conferir (somente leitura) → confirmar → enviar em lotes pequenos → conferir a resposta da HubSoft. O id da O.S. de origem
// (ex.: do IXC) vai em `parametros` do atendimento e também no log de auditoria, para a mesma O.S. nunca ser aberta duas vezes.

const soBase = "/api/v1/integracao"

// SOCatalogItem — item de catálogo (tipo, status, técnico…).
type SOCatalogItem struct {
	ID    string `json:"id"`
	Nome  string `json:"nome"`
	Extra string `json:"extra,omitempty"` // prefixo do status, destino do tipo de O.S.…
}

// ServiceOrderCatalogs — IDs que o cadastro de atendimento/O.S. exige.
type ServiceOrderCatalogs struct {
	TiposAtendimento  []SOCatalogItem `json:"tipos_atendimento"`
	StatusAtendimento []SOCatalogItem `json:"status_atendimento"`
	TiposOS           []SOCatalogItem `json:"tipos_os"`
	Tecnicos          []SOCatalogItem `json:"tecnicos"`
	Disponibilidades  []SOCatalogItem `json:"disponibilidades"`
	Origens           []SOCatalogItem `json:"origens_contato"`
	Erros             []string        `json:"erros,omitempty"`
}

func soCatalog(ctx context.Context, cfg Config, token, path, arrayKey, idKey, nameKey, extraKey string) ([]SOCatalogItem, string) {
	body, errMsg := stockGetQuery(ctx, cfg, token, soBase+path, nil)
	if errMsg != "" {
		return nil, errMsg
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, "resposta inválida"
	}
	var out []SOCatalogItem
	for _, it := range extractArray(doc, arrayKey) {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, SOCatalogItem{ID: pickStr(m, idKey), Nome: strings.TrimSpace(pickStr(m, nameKey)), Extra: pickStr(m, extraKey)})
	}
	sort.SliceStable(out, func(i, j int) bool { return soNorm(out[i].Nome) < soNorm(out[j].Nome) })
	return out, ""
}

// LoadServiceOrderCatalogs lê os catálogos (somente leitura). Um catálogo que falha vira aviso em Erros; os outros seguem.
func LoadServiceOrderCatalogs(ctx context.Context, cfg Config, token string) ServiceOrderCatalogs {
	var c ServiceOrderCatalogs
	add := func(label string, items []SOCatalogItem, err string, dst *[]SOCatalogItem) {
		if err != "" {
			c.Erros = append(c.Erros, label+": "+err)
			return
		}
		*dst = items
	}
	it, e := soCatalog(ctx, cfg, token, "/configuracao/tipo_atendimento", "tipos_atendimento", "id_tipo_atendimento", "descricao", "")
	add("tipos de atendimento", it, e, &c.TiposAtendimento)
	it, e = soCatalog(ctx, cfg, token, "/configuracao/status_atendimento", "status_atendimento", "id_atendimento_status", "descricao", "prefixo")
	add("status de atendimento", it, e, &c.StatusAtendimento)
	it, e = soCatalog(ctx, cfg, token, "/configuracao/tipo_ordem_servico", "tipo_ordem_servicos", "id_tipo_ordem_servico", "descricao", "destino")
	add("tipos de O.S.", it, e, &c.TiposOS)
	it, e = soCatalog(ctx, cfg, token, "/configuracao/tecnico", "tecnicos", "id", "name", "")
	add("técnicos", it, e, &c.Tecnicos)
	it, e = soCatalog(ctx, cfg, token, "/configuracao/disponibilidade", "periodos", "id_periodo_dia", "descricao", "")
	add("disponibilidades", it, e, &c.Disponibilidades)
	it, e = soCatalog(ctx, cfg, token, "/configuracao/origem_contato", "origem_contatos", "id_origem_contato", "descricao", "")
	add("origens de contato", it, e, &c.Origens)
	return c
}

func soNorm(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(strings.TrimSpace(s))) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) || r == '-' || r == '_' || r == '/' {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// resolveCatalog acha o id por id exato ou por nome (sem acento/caixa; igual, ou — se único — «contém»).
func resolveCatalog(list []SOCatalogItem, v string) (item SOCatalogItem, ok bool, why string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return item, false, ""
	}
	if isPosInt(v) {
		for _, it := range list {
			if it.ID == v {
				return it, true, ""
			}
		}
	}
	n := soNorm(v)
	var eq, contains []SOCatalogItem
	for _, it := range list {
		nn := soNorm(it.Nome)
		switch {
		case nn == n:
			eq = append(eq, it)
		case n != "" && (strings.Contains(nn, n) || strings.Contains(n, nn)) && nn != "":
			contains = append(contains, it)
		}
	}
	if len(eq) == 1 {
		return eq[0], true, ""
	}
	if len(eq) > 1 {
		return item, false, fmt.Sprintf("«%s» corresponde a mais de um cadastro (%s) — use o id", v, joinNames(eq))
	}
	if len(contains) == 1 {
		return contains[0], true, ""
	}
	if len(contains) > 1 {
		return item, false, fmt.Sprintf("«%s» corresponde a mais de um cadastro (%s) — use o nome completo ou o id", v, joinNames(contains))
	}
	return item, false, fmt.Sprintf("«%s» não existe no cadastro da HubSoft", v)
}

func joinNames(l []SOCatalogItem) string {
	var p []string
	for i, it := range l {
		if i >= 4 {
			p = append(p, "…")
			break
		}
		p = append(p, it.Nome+" ("+it.ID+")")
	}
	return strings.Join(p, ", ")
}

func splitList(s string) []string {
	var out []string
	for _, p := range regexp.MustCompile(`[;,|\n]`).Split(s, -1) {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ServiceOrderRow — uma linha de importação. Os campos de catálogo aceitam o NOME ou o id.
type ServiceOrderRow struct {
	Linha int `json:"linha"`
	// Referencia — id da O.S. de origem (ex.: IXC). Vai em `parametros` e impede abrir a mesma duas vezes.
	Referencia string `json:"referencia"`
	// Serviço do cliente: por id, por login da HubSoft, ou por nome do cliente (+ login do IXC para desempatar).
	IDClienteServico string `json:"id_cliente_servico"`
	LoginHubsoft     string `json:"login_hubsoft"`
	Cliente          string `json:"cliente"`
	LoginIXC         string `json:"login_ixc"`

	TipoAtendimento   string `json:"tipo_atendimento"`
	StatusAtendimento string `json:"status_atendimento"`
	Responsaveis      string `json:"responsaveis"`
	OrigemContato     string `json:"origem_contato"`
	Descricao         string `json:"descricao"`
	Nome              string `json:"nome"`
	Telefone          string `json:"telefone"`
	Email             string `json:"email"`

	AbrirOS         string `json:"abrir_os"` // sim / nao / vazio = padrão da tela
	TipoOS          string `json:"tipo_os"`
	StatusOS        string `json:"status_os"` // aguardando_agendamento | pendente
	Tecnicos        string `json:"tecnicos"`
	Disponibilidade string `json:"disponibilidade"`
	Agenda          string `json:"agenda"`
	Parametros      string `json:"parametros"` // chave=valor;chave=valor
}

// ServiceOrderOptions — padrões da tela (a linha vence quando preenchida) e travas.
type ServiceOrderOptions struct {
	TipoAtendimento   string `json:"tipo_atendimento"`
	StatusAtendimento string `json:"status_atendimento"`
	Responsaveis      string `json:"responsaveis"`
	OrigemContato     string `json:"origem_contato"`
	AbrirOS           bool   `json:"abrir_os"`
	TipoOS            string `json:"tipo_os"`
	StatusOS          string `json:"status_os"`
	Tecnicos          string `json:"tecnicos"`
	Disponibilidade   string `json:"disponibilidade"`
	Agenda            string `json:"agenda"`
	ValidarLimite     bool   `json:"validar_limite_agenda"`
	ValidarHorario    bool   `json:"validar_horario_agenda"`
	// PermitirDuplicadas — abre mesmo se o cliente já tiver atendimento ABERTO do mesmo tipo na HubSoft (padrão: bloqueia).
	PermitirDuplicadas bool   `json:"permitir_duplicadas"`
	TelefonePadrao     string `json:"telefone_padrao"`
	NomePadrao         string `json:"nome_padrao"`
	Origem             string `json:"origem"` // texto gravado em parametros.origem (ex.: «importação IXC»)
	// Done — referências já abertas antes (do log de auditoria): referência → «atendimento X em …».
	Done map[string]string `json:"-"`
}

// ServiceOrderPreview — resultado da conferência de uma linha.
type ServiceOrderPreview struct {
	Linha       int            `json:"linha"`
	OK          bool           `json:"ok"`
	AlreadyDone bool           `json:"already_done,omitempty"`
	Problems    []string       `json:"problems,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
	Service     *ServiceInfo   `json:"service,omitempty"`
	Resolved    []string       `json:"resolved,omitempty"` // «Tipo de atendimento: SUPORTE (63)»…
	Body        map[string]any `json:"body,omitempty"`     // o que será enviado
}

var phoneDigits = regexp.MustCompile(`\D`)

// normPhone devolve só dígitos com DDD (10 ou 11), tirando +55/0 inicial; "" se não der.
func normPhone(s string) string {
	d := phoneDigits.ReplaceAllString(s, "")
	d = strings.TrimPrefix(d, "55")
	if len(d) == 12 && strings.HasPrefix(d, "0") {
		d = d[1:]
	}
	if len(d) == 11 && d[2] == '0' && d[3] != '9' { // 0 solto no meio — improvável; descarta
		return ""
	}
	if len(d) < 10 || len(d) > 11 || strings.Trim(d, "0") == "" {
		return ""
	}
	if strings.Trim(d[2:], "0") == "" { // (22) 00000-0000
		return ""
	}
	return d
}

// findServicesByName — serviços (inclusive cancelados) dos clientes cujo nome bate com `nome`.
func findServicesByName(ctx context.Context, cfg Config, token, nome string) ([]ServiceInfo, error) {
	return findServicesBy(ctx, cfg, token, "nome_razaosocial", nome, func(c map[string]any) bool {
		return sameNamePrefix(pickStr(c, "nome_razaosocial"), nome)
	}, nil)
}

func sameNamePrefix(a, b string) bool {
	x, y := strings.Fields(soNorm(a)), strings.Fields(soNorm(b))
	if len(x) == 0 || len(y) == 0 {
		return false
	}
	s, l := x, y
	if len(x) > len(y) {
		s, l = y, x
	}
	if len(s) < 2 {
		return false
	}
	for i := range s {
		if s[i] != l[i] {
			return false
		}
	}
	return true
}

func findServicesBy(ctx context.Context, cfg Config, token, busca, termo string, keepClient func(map[string]any) bool, keepService func(ServiceInfo) bool) ([]ServiceInfo, error) {
	body, errMsg := stockGetQuery(ctx, cfg, token, soBase+"/cliente", map[string]string{
		"busca": busca, "termo_busca": termo, "limit": "20", "cancelado": "todos", "inativo": "todos",
	})
	if errMsg != "" {
		if notFoundRe.MatchString(errMsg) {
			return nil, nil
		}
		return nil, fmt.Errorf("consulta de cliente falhou: %s", errMsg)
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, fmt.Errorf("resposta inválida da HubSoft ao procurar o cliente")
	}
	var out []ServiceInfo
	for _, it := range extractArray(doc, "clientes") {
		c, ok := it.(map[string]any)
		if !ok || (keepClient != nil && !keepClient(c)) {
			continue
		}
		arr, _ := c["servicos"].([]any)
		for _, sv := range arr {
			sm, ok := sv.(map[string]any)
			if !ok {
				continue
			}
			si := ServiceInfo{
				IDCliente: pickStr(c, "id_cliente"), Cliente: pickStr(c, "nome_razaosocial"), CodigoCliente: pickStr(c, "codigo_cliente"),
				IDClienteServico: pickStr(sm, "id_cliente_servico"), Login: pickStr(sm, "login"), Plano: pickStr(sm, "nome"), Status: pickStr(sm, "status"),
			}
			if keepService == nil || keepService(si) {
				out = append(out, si)
			}
		}
	}
	return out, nil
}

// stripLoginPrefix remove o prefixo de migração (ex.: «c2-6mariacelia1» → «mariacelia1»).
var loginPrefixRE = regexp.MustCompile(`^[a-z]\d+-\d+`)

func loginMatches(hubLogin, ixcLogin string) bool {
	h, x := strings.ToLower(strings.TrimSpace(hubLogin)), strings.ToLower(strings.TrimSpace(ixcLogin))
	if h == "" || x == "" {
		return false
	}
	return h == x || strings.HasSuffix(h, x) || strings.TrimPrefix(loginPrefixRE.ReplaceAllString(h, ""), "-") == x
}

// resolveService acha O serviço da linha, sem adivinhar: 0 ou mais de 1 candidato vira problema.
func resolveService(ctx context.Context, cfg Config, token string, r ServiceOrderRow) (*ServiceInfo, []string, []string) {
	var probs, warns []string
	var cands []ServiceInfo
	var err error
	switch {
	case strings.TrimSpace(r.IDClienteServico) != "":
		cands, err = FindServices(ctx, cfg, token, strings.TrimSpace(r.IDClienteServico))
	case strings.TrimSpace(r.LoginHubsoft) != "":
		cands, err = FindServices(ctx, cfg, token, strings.TrimSpace(r.LoginHubsoft))
	case strings.TrimSpace(r.Cliente) != "":
		var all []ServiceInfo
		t := strings.Fields(soNorm(r.Cliente))
		for cut := 0; cut < 3 && len(all) == 0; cut++ {
			if len(t)-cut < 2 {
				break
			}
			all, err = findServicesByName(ctx, cfg, token, strings.Join(t[:len(t)-cut], " "))
			if err != nil {
				break
			}
			var keep []ServiceInfo
			for _, s := range all {
				if sameNamePrefix(s.Cliente, r.Cliente) {
					keep = append(keep, s)
				}
			}
			all = keep
		}
		cands = all
		if l := strings.TrimSpace(r.LoginIXC); l != "" && len(cands) > 1 {
			var m []ServiceInfo
			for _, s := range cands {
				if loginMatches(s.Login, l) {
					m = append(m, s)
				}
			}
			if len(m) > 0 {
				cands = m
			}
		} else if l != "" && len(cands) == 1 && !loginMatches(cands[0].Login, l) {
			warns = append(warns, fmt.Sprintf("o login do serviço na HubSoft («%s») não corresponde ao login informado («%s») — confira se é o serviço certo", cands[0].Login, l))
		}
	default:
		return nil, []string{"informe id_cliente_servico, login_hubsoft ou o nome do cliente"}, nil
	}
	if err != nil {
		return nil, []string{err.Error()}, nil
	}
	switch len(cands) {
	case 0:
		return nil, []string{"serviço/cliente não encontrado na HubSoft"}, warns
	case 1:
		s := cands[0]
		if st := strings.ToLower(s.Status); strings.Contains(st, "cancel") {
			warns = append(warns, "o serviço está CANCELADO na HubSoft — a HubSoft pode recusar o atendimento")
		}
		if n := strings.TrimSpace(r.Cliente); n != "" && (strings.TrimSpace(r.IDClienteServico) != "" || strings.TrimSpace(r.LoginHubsoft) != "") && !sameNamePrefix(s.Cliente, n) {
			probs = append(probs, fmt.Sprintf("o serviço pertence a «%s», não a «%s» — confira o id/login", s.Cliente, n))
		}
		return &s, probs, warns
	default:
		var d []string
		for i, s := range cands {
			if i >= 4 {
				d = append(d, "…")
				break
			}
			d = append(d, fmt.Sprintf("%s (%s, %s)", s.IDClienteServico, s.Login, s.Status))
		}
		return nil, []string{"mais de um serviço possível: " + strings.Join(d, "; ") + " — informe o id_cliente_servico"}, warns
	}
}

func parseKV(s string) map[string]any {
	out := map[string]any{}
	for _, p := range splitList(s) {
		if i := strings.Index(p, "="); i > 0 {
			out[strings.TrimSpace(p[:i])] = strings.TrimSpace(p[i+1:])
		}
	}
	return out
}

func yesNo(v string, def bool) bool {
	switch soNorm(v) {
	case "sim", "s", "true", "1", "yes", "y":
		return true
	case "nao", "n", "false", "0", "no":
		return false
	}
	return def
}

var statusOSMap = map[string]string{
	"aguardando agendamento": "aguardando_agendamento", "aguardando agendameto": "aguardando_agendamento", "aguardando": "aguardando_agendamento",
	"pendente": "pendente",
}

func pick(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

// PreviewServiceOrder confere UMA linha e monta o corpo que seria enviado (nada é gravado).
func PreviewServiceOrder(ctx context.Context, cfg Config, token string, r ServiceOrderRow, opt ServiceOrderOptions, cat ServiceOrderCatalogs) ServiceOrderPreview {
	pv := ServiceOrderPreview{Linha: r.Linha}
	ref := strings.TrimSpace(r.Referencia)
	if ref != "" {
		if info, ok := opt.Done[ref]; ok {
			pv.AlreadyDone, pv.OK = true, true
			pv.Warnings = append(pv.Warnings, "esta referência já foi aberta antes ("+info+") — nada será enviado")
			return pv
		}
	}
	svc, probs, warns := resolveService(ctx, cfg, token, r)
	pv.Problems, pv.Warnings, pv.Service = append(pv.Problems, probs...), append(pv.Warnings, warns...), svc

	desc := strings.TrimSpace(r.Descricao)
	if desc == "" {
		pv.Problems = append(pv.Problems, "descrição do atendimento vazia")
	}
	nome := pick(r.Nome, opt.NomePadrao)
	if nome == "" && svc != nil {
		nome = svc.Cliente
	}
	if nome == "" {
		pv.Problems = append(pv.Problems, "nome do solicitante vazio")
	}
	tel := normPhone(r.Telefone)
	if tel == "" {
		if tel = normPhone(opt.TelefonePadrao); tel != "" {
			pv.Warnings = append(pv.Warnings, "telefone da linha inválido/vazio — usando o telefone padrão")
		}
	}
	if tel == "" {
		pv.Problems = append(pv.Problems, "telefone do solicitante inválido (precisa de DDD + número) e não há telefone padrão")
	}

	body := map[string]any{"descricao": desc, "nome": nome, "telefone": tel}
	if svc != nil {
		body["id_cliente_servico"] = atoiSafe(svc.IDClienteServico)
	}
	if e := strings.TrimSpace(r.Email); e != "" {
		if strings.Contains(e, "@") {
			body["email"] = e
		} else {
			pv.Warnings = append(pv.Warnings, "e-mail inválido ignorado")
		}
	}
	res := func(label string, list []SOCatalogItem, v string, required bool) (SOCatalogItem, bool) {
		if strings.TrimSpace(v) == "" {
			if required {
				pv.Problems = append(pv.Problems, label+" não informado")
			}
			return SOCatalogItem{}, false
		}
		it, ok, why := resolveCatalog(list, v)
		if !ok {
			pv.Problems = append(pv.Problems, label+": "+why)
			return it, false
		}
		pv.Resolved = append(pv.Resolved, fmt.Sprintf("%s: %s (%s)", label, it.Nome, it.ID))
		return it, true
	}
	var tipoNomes []string // nomes do tipo de atendimento e do tipo de O.S. escolhidos — base da checagem de duplicidade
	if it, ok := res("Tipo de atendimento", cat.TiposAtendimento, pick(r.TipoAtendimento, opt.TipoAtendimento), false); ok {
		body["id_tipo_atendimento"] = atoiSafe(it.ID)
		tipoNomes = append(tipoNomes, it.Nome)
	}
	if it, ok := res("Status do atendimento", cat.StatusAtendimento, pick(r.StatusAtendimento, opt.StatusAtendimento), false); ok {
		body["id_atendimento_status"] = atoiSafe(it.ID)
	}
	if it, ok := res("Origem do contato", cat.Origens, pick(r.OrigemContato, opt.OrigemContato), false); ok {
		body["id_origem_contato"] = atoiSafe(it.ID)
	}
	if rs := splitList(pick(r.Responsaveis, opt.Responsaveis)); len(rs) > 0 {
		var ids []string
		for _, v := range rs {
			if it, ok := res("Responsável", cat.Tecnicos, v, false); ok {
				ids = append(ids, it.ID)
			}
		}
		if len(ids) > 0 {
			body["id_usuario_responsavel"] = strings.Join(ids, ",")
		}
	}

	abrir := yesNo(r.AbrirOS, opt.AbrirOS)
	body["abrir_os"] = abrir
	par := map[string]any{"origem": firstNonEmpty(strings.TrimSpace(opt.Origem), "NetQuasar")}
	if ref != "" {
		par["referencia_origem"] = ref
	}
	for k, v := range parseKV(r.Parametros) {
		par[k] = v
	}
	body["parametros"] = par
	if abrir {
		po := map[string]any{}
		if it, ok := res("Tipo de O.S.", cat.TiposOS, pick(r.TipoOS, opt.TipoOS), true); ok {
			po["id_tipo_ordem_servico"] = atoiSafe(it.ID)
			tipoNomes = append(tipoNomes, it.Nome)
			if it.Extra != "" && it.Extra != "cliente_servico" {
				pv.Problems = append(pv.Problems, fmt.Sprintf("o tipo de O.S. «%s» é de destino «%s», não de cliente/serviço", it.Nome, it.Extra))
			}
		}
		if st := soNorm(pick(r.StatusOS, opt.StatusOS)); st != "" {
			if v, ok := statusOSMap[st]; ok {
				po["status"] = v
			} else {
				pv.Problems = append(pv.Problems, "status da O.S. «"+st+"» inválido — use aguardando_agendamento ou pendente")
			}
		}
		if ts := splitList(pick(r.Tecnicos, opt.Tecnicos)); len(ts) > 0 {
			var ids []int
			for _, v := range ts {
				if it, ok := res("Técnico", cat.Tecnicos, v, false); ok {
					ids = append(ids, atoiSafe(it.ID))
				}
			}
			if len(ids) > 0 {
				po["ids_tecnicos"] = ids
			}
		}
		if ds := splitList(pick(r.Disponibilidade, opt.Disponibilidade)); len(ds) > 0 {
			var ids []string
			for _, v := range ds {
				if it, ok := res("Disponibilidade", cat.Disponibilidades, v, false); ok {
					ids = append(ids, it.ID)
				}
			}
			if len(ids) > 0 {
				po["id_disponibilidade"] = strings.Join(ids, ",")
			}
		}
		if ag := pick(r.Agenda, opt.Agenda); ag != "" {
			if !isPosInt(ag) {
				pv.Problems = append(pv.Problems, "agenda precisa ser o id numérico da agenda de O.S.")
			} else {
				po["id_agenda_ordem_servico"] = atoiSafe(ag)
				po["validar_limite_agenda"], po["validar_horario_agenda"] = opt.ValidarLimite, opt.ValidarHorario
			}
		}
		body["parametros_ordem_servico"] = po
	}
	if svc != nil && len(pv.Problems) == 0 {
		if dup := openDuplicate(ctx, cfg, token, svc, tipoNomes); dup != "" {
			if opt.PermitirDuplicadas {
				pv.Warnings = append(pv.Warnings, dup+" — abrindo mesmo assim (duplicidade permitida)")
			} else {
				pv.Problems = append(pv.Problems, dup+" — para não duplicar, nada será aberto (marque «permitir duplicadas» para abrir mesmo assim)")
			}
		}
	}
	pv.Body = body
	pv.OK = len(pv.Problems) == 0 && svc != nil
	return pv
}

// atendimentoAberto — o status do atendimento (texto da HubSoft) indica que ainda não foi encerrado.
func atendimentoAberto(status, closedAt string) bool {
	s := soNorm(status)
	for _, f := range []string{"resolvido", "fechado", "encerrado", "cancelado", "concluido", "finalizado"} {
		if strings.Contains(s, f) {
			return false
		}
	}
	return true
}

// openDuplicate procura, nos atendimentos do cliente, um ABERTO com o mesmo assunto do que está sendo aberto
// (assunto igual ao tipo de atendimento/tipo de O.S. escolhido, ou um contendo o outro). Devolve a descrição, ou "".
// Falha de consulta não bloqueia (só não consegue checar) — a conferência mostra o aviso.
func openDuplicate(ctx context.Context, cfg Config, token string, svc *ServiceInfo, tipoNomes []string) string {
	if svc == nil || strings.TrimSpace(svc.CodigoCliente) == "" || len(tipoNomes) == 0 {
		return ""
	}
	r, _ := SearchAttendance(ctx, cfg, token, "codigo_cliente", svc.CodigoCliente)
	for _, it := range r.Items {
		sub := soNorm(it.Subject)
		if sub == "" || !atendimentoAberto(it.Status, it.ClosedAt) {
			continue
		}
		for _, n := range tipoNomes {
			nn := soNorm(n)
			if nn != "" && len(nn) >= 5 && (sub == nn || strings.Contains(sub, nn) || strings.Contains(nn, sub)) {
				return fmt.Sprintf("o cliente já tem o atendimento aberto «%s» (protocolo %s, %s, status «%s»)", it.Subject, it.Protocol, it.OpenedAt, it.Status)
			}
		}
	}
	return ""
}

// ServiceOrderResult — resultado de uma linha enviada.
type ServiceOrderResult struct {
	Linha         int                 `json:"linha"`
	OK            bool                `json:"ok"`
	Action        string              `json:"action"` // created | already_done | blocked | failed | verify_failed | skipped
	Message       string              `json:"message"`
	IDAtendimento string              `json:"id_atendimento,omitempty"`
	Protocolo     string              `json:"protocolo,omitempty"`
	OrdensServico []string            `json:"ordens_servico,omitempty"` // «#numero (id)»
	Detail        string              `json:"detail,omitempty"`
	Problems      []string            `json:"problems,omitempty"`
	Preview       ServiceOrderPreview `json:"preview"`
}

// ApplyServiceOrder confere de novo, envia e confere a resposta.
func ApplyServiceOrder(ctx context.Context, cfg Config, token string, r ServiceOrderRow, opt ServiceOrderOptions, cat ServiceOrderCatalogs) ServiceOrderResult {
	pv := PreviewServiceOrder(ctx, cfg, token, r, opt, cat)
	res := ServiceOrderResult{Linha: r.Linha, Preview: pv}
	if pv.AlreadyDone {
		res.OK, res.Action = true, "already_done"
		res.Message = "referência " + strings.TrimSpace(r.Referencia) + " já aberta antes — nada foi enviado"
		return res
	}
	if !pv.OK {
		res.Action, res.Problems = "blocked", pv.Problems
		res.Message = "nada foi enviado — a conferência encontrou problema(s): " + strings.Join(pv.Problems, " | ")
		return res
	}
	rb, errMsg := stockCall(ctx, cfg, token, "POST", soBase+"/atendimento", pv.Body)
	if errMsg != "" {
		res.Action, res.Message, res.Detail = "failed", "a HubSoft recusou o atendimento: "+errMsg, snippet(rb)
		return res
	}
	var doc struct {
		Atendimento struct {
			ID        json.Number `json:"id_atendimento"`
			Protocolo string      `json:"protocolo"`
			Cliente   struct {
				Codigo json.Number `json:"codigo_cliente"`
			} `json:"cliente"`
			OS []struct {
				ID     json.Number `json:"id_ordem_servico"`
				Numero string      `json:"numero_ordem_servico"`
				Tipo   string      `json:"tipo"`
			} `json:"ordens_servico"`
		} `json:"atendimento"`
	}
	_ = json.Unmarshal(rb, &doc)
	a := doc.Atendimento
	res.IDAtendimento, res.Protocolo = a.ID.String(), a.Protocolo
	for _, o := range a.OS {
		res.OrdensServico = append(res.OrdensServico, fmt.Sprintf("#%s (id %s) %s", o.Numero, o.ID.String(), o.Tipo))
	}
	var diffs []string
	if res.IDAtendimento == "" || res.IDAtendimento == "0" {
		diffs = append(diffs, "a resposta não trouxe o id do atendimento")
	}
	if cod := a.Cliente.Codigo.String(); pv.Service != nil && pv.Service.CodigoCliente != "" && cod != "" && cod != pv.Service.CodigoCliente {
		diffs = append(diffs, fmt.Sprintf("atendimento aberto para o cliente %s em vez de %s", cod, pv.Service.CodigoCliente))
	}
	if abrir, _ := pv.Body["abrir_os"].(bool); abrir && len(a.OS) == 0 {
		diffs = append(diffs, "era para abrir a O.S., mas a resposta não trouxe nenhuma ordem de serviço")
	}
	if len(diffs) > 0 {
		res.Action, res.Detail = "verify_failed", snippet(rb)
		res.Message = "a HubSoft aceitou (atendimento " + res.IDAtendimento + "), mas a conferência encontrou diferença: " + strings.Join(diffs, "; ")
		return res
	}
	res.OK, res.Action = true, "created"
	res.Message = fmt.Sprintf("atendimento %s (protocolo %s) aberto para %s", res.IDAtendimento, res.Protocolo, svcLabel(pv.Service))
	if len(res.OrdensServico) > 0 {
		res.Message += " com a O.S. " + strings.Join(res.OrdensServico, ", ")
	}
	return res
}

func svcLabel(s *ServiceInfo) string {
	if s == nil {
		return "?"
	}
	return strconv.Quote(s.Cliente)[1:len(strconv.Quote(s.Cliente))-1] + " (serviço " + s.IDClienteServico + ")"
}

// PreviewServiceOrderRows confere várias linhas (leitura) — referência repetida no mesmo lote só vale na primeira.
func PreviewServiceOrderRows(ctx context.Context, cfg Config, token string, rows []ServiceOrderRow, opt ServiceOrderOptions, cat ServiceOrderCatalogs) []ServiceOrderPreview {
	out := make([]ServiceOrderPreview, len(rows))
	seen := map[string]int{}
	for i, r := range rows {
		out[i] = PreviewServiceOrder(ctx, cfg, token, r, opt, cat)
		if ref := strings.TrimSpace(r.Referencia); ref != "" && !out[i].AlreadyDone {
			if first, dup := seen[ref]; dup {
				out[i].OK = false
				out[i].Problems = append(out[i].Problems, fmt.Sprintf("referência %s repetida neste arquivo (linha %d) — só a primeira é enviada", ref, first))
			} else {
				seen[ref] = r.Linha
			}
		}
	}
	return out
}

// ApplyServiceOrderRows envia em ordem; para depois de 3 falhas seguidas (o resto volta «skipped»).
func ApplyServiceOrderRows(ctx context.Context, cfg Config, token string, rows []ServiceOrderRow, opt ServiceOrderOptions, cat ServiceOrderCatalogs) (results []ServiceOrderResult, halted bool) {
	results = make([]ServiceOrderResult, len(rows))
	seen := map[string]bool{}
	fails := 0
	for i, r := range rows {
		if halted {
			results[i] = ServiceOrderResult{Linha: r.Linha, Action: "skipped", Message: "não enviada: o envio foi interrompido por falhas seguidas"}
			continue
		}
		if ref := strings.TrimSpace(r.Referencia); ref != "" {
			if seen[ref] {
				results[i] = ServiceOrderResult{Linha: r.Linha, Action: "blocked", Message: "referência " + ref + " repetida neste lote — só a primeira é enviada", Problems: []string{"referência repetida"}}
				continue
			}
			seen[ref] = true
		}
		results[i] = ApplyServiceOrder(ctx, cfg, token, r, opt, cat)
		switch results[i].Action {
		case "failed", "verify_failed":
			fails++
			if fails >= 3 {
				halted = true
			}
		case "created", "already_done":
			fails = 0
		}
	}
	return results, halted
}
