package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Cadastro em massa de PATRIMÔNIOS de estoque (permissão integrations.hubsoft_bulk) --------------------------------
//
// Passo 2 da migração do IXC: criar cada patrimônio (um roteador/ONU/ONT/switch físico, com série e MAC próprios) no
// local de estoque. A API da HubSoft NÃO tem «criar patrimônio»: ele nasce da ENTRADA MANUAL de estoque de um produto com
// controle patrimonial (POST /estoque/movimento_estoque/entrada) — uma unidade vira um patrimônio com código automático e
// série/MAC vazios — e só depois recebe série, MAC e identificador por PUT /estoque/produto_item/{id}.
//
// Por isso cada linha (um patrimônio) faz uma entrada de QUANTIDADE 1 (relação 1:1 entre a linha do arquivo e o patrimônio
// criado, sem depender de ordem) e, em seguida, grava os identificadores e confere. Antes de criar, procura duplicidade por
// identificador próprio, número de série e MAC em TODA a HubSoft — nunca cria um patrimônio que já exista.
//
// Se a entrada der certo mas a gravação dos identificadores falhar, o patrimônio fica «sem identificação» na HubSoft; o
// resultado traz o id_produto_item e a linha pode ser reenviada com a coluna id_produto_item preenchida para só identificá-lo.

// StockItemRow é uma linha do CSV de patrimônios (um patrimônio por linha).
type StockItemRow struct {
	Line      int    `json:"line"`
	IDProduto string `json:"id_produto"` // produto da HubSoft (já com controle patrimonial)
	// Nome do produto — só para exibição/relatório; não é enviado à HubSoft.
	ProdutoNome          string `json:"produto_nome,omitempty"`
	IdentificadorProprio string `json:"identificador_proprio"`
	NumeroSerie          string `json:"numero_serie"`
	MacAddress           string `json:"mac_address"`
	Observacoes          string `json:"observacoes"`
	// IDProdutoItem — opcional: patrimônio JÁ criado (ex.: a gravação dos identificadores falhou numa execução anterior).
	// Com ele a linha NÃO faz entrada de estoque; só identifica esse patrimônio.
	IDProdutoItem string `json:"id_produto_item"`
	// IdentificadorAlternativo — identificador que o patrimônio pode JÁ ter na HubSoft (ex.: o Nº patrimônio do IXC, que a equipe
	// usou em cadastros manuais antigos). Serve só para NÃO duplicar: se existir (no mesmo produto), a linha é «já existe».
	IdentificadorAlternativo string `json:"identificador_alternativo"`
	// Referencia — texto livre (ex.: código do IXC) só para os relatórios.
	Referencia string `json:"referencia,omitempty"`
	// IDLocalEstoque — local de estoque da entrada (definido pela tela para todas as linhas).
	IDLocalEstoque string `json:"id_local_estoque"`
}

var macHexRe = regexp.MustCompile(`[^0-9A-Fa-f]`)
var macAlnumRe = regexp.MustCompile(`[^0-9A-Za-z]`)
var macSepOnlyRe = regexp.MustCompile(`^[0-9A-Fa-f:.\-\s]+$`)

// NormalizeMAC devolve o valor para gravar no campo MAC. Um MAC de verdade (12 dígitos hexadecimais, com ou sem : - . ou
// espaços) vira AA:BB:CC:DD:EE:FF. Qualquer OUTRO texto (ex.: o identificador «ZTE3QJNMCM36439» que o IXC guarda nesse campo
// para algumas ONTs) é gravado EXATAMENTE como está — decisão do operador: não é MAC inválido, é um padrão do equipamento.
// ok=false só para texto com caracteres de controle ou longo demais.
func NormalizeMAC(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	if len([]rune(s)) > 80 || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", false
	}
	if macSepOnlyRe.MatchString(s) {
		h := strings.ToUpper(macHexRe.ReplaceAllString(s, ""))
		if len(h) == 12 {
			parts := make([]string, 6)
			for i := range parts {
				parts[i] = h[i*2 : i*2+2]
			}
			return strings.Join(parts, ":"), true
		}
	}
	return s, true
}

// macKey — chave de comparação do campo MAC: só letras e dígitos, em maiúsculas («D8:38:0D…» e «d8380d…» são iguais; um texto
// como «ZTE3QJNMCM36439» também é comparado por inteiro).
func macKey(s string) string { return strings.ToUpper(macAlnumRe.ReplaceAllString(s, "")) }

// StockItemProduct — o que a validação precisa saber de cada produto da HubSoft.
type StockItemProduct struct {
	Nome       string
	Patrimonio bool
}

// validateStockItemRow confere o formato de UMA linha (sem consultar a HubSoft).
func validateStockItemRow(r StockItemRow, prods map[string]StockItemProduct) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }
	pid := strings.TrimSpace(r.IDProduto)
	switch {
	case pid == "":
		add("id_produto é obrigatório")
	case !isPosInt(pid):
		add("id_produto precisa ser um número positivo (veio %q)", pid)
	case prods != nil:
		if pr, ok := prods[pid]; !ok {
			add("id_produto %s não existe na HubSoft", pid)
		} else if !pr.Patrimonio {
			add("o produto %s («%s») não tem controle patrimonial — não gera patrimônio", pid, pr.Nome)
		}
	}
	if !isPosInt(strings.TrimSpace(r.IDLocalEstoque)) {
		add("id_local_estoque precisa ser um número positivo (veio %q)", r.IDLocalEstoque)
	}
	if strings.TrimSpace(r.IdentificadorProprio) == "" {
		add("identificador_proprio é obrigatório (é ele que impede criar o mesmo patrimônio duas vezes)")
	} else if len([]rune(strings.TrimSpace(r.IdentificadorProprio))) > 120 {
		add("identificador_proprio muito longo")
	}
	if len([]rune(strings.TrimSpace(r.IdentificadorAlternativo))) > 120 {
		add("identificador_alternativo muito longo")
	}
	if len([]rune(strings.TrimSpace(r.NumeroSerie))) > 120 {
		add("numero_serie muito longo")
	}
	if _, ok := NormalizeMAC(r.MacAddress); !ok {
		add("mac_address inválido (%q) — texto longo demais ou com caracteres de controle", r.MacAddress)
	}
	if len([]rune(r.Observacoes)) > 4000 {
		add("observacoes muito longas")
	}
	if it := strings.TrimSpace(r.IDProdutoItem); it != "" && !isPosInt(it) {
		add("id_produto_item precisa ser um número positivo (veio %q)", it)
	}
	return p
}

// ValidateStockItemRows valida o lote e aponta identificador/série/MAC repetidos DENTRO do arquivo (as linhas envolvidas
// ficam de fora: não dá para saber qual é a certa).
func ValidateStockItemRows(rows []StockItemRow, prods map[string]StockItemProduct) ImportValidationResult {
	res := ImportValidationResult{OK: true, Rows: make([]ImportRowValidation, 0, len(rows))}
	type key struct{ kind, val string }
	seen := map[key][]int{}
	lineOf := func(i int) int {
		if rows[i].Line > 0 {
			return rows[i].Line
		}
		return i + 2
	}
	for i, r := range rows {
		if v := strings.ToLower(strings.TrimSpace(r.IdentificadorProprio)); v != "" {
			seen[key{"identificador_proprio", v}] = append(seen[key{"identificador_proprio", v}], lineOf(i))
		}
		if v := strings.ToUpper(strings.TrimSpace(r.NumeroSerie)); v != "" {
			seen[key{"numero_serie", v}] = append(seen[key{"numero_serie", v}], lineOf(i))
		}
		if m := macKey(r.MacAddress); m != "" {
			seen[key{"mac_address", m}] = append(seen[key{"mac_address", m}], lineOf(i))
		}
	}
	for i, r := range rows {
		probs := validateStockItemRow(r, prods)
		check := func(kind, val string) {
			if val == "" {
				return
			}
			if ls := seen[key{kind, val}]; len(ls) > 1 {
				var others []string
				for _, l := range ls {
					if l != lineOf(i) {
						others = append(others, fmt.Sprint(l))
					}
				}
				probs = append(probs, fmt.Sprintf("%s repetido no arquivo (também nas linhas %s) — vai para as pendências", kind, strings.Join(others, ", ")))
			}
		}
		check("identificador_proprio", strings.ToLower(strings.TrimSpace(r.IdentificadorProprio)))
		check("numero_serie", strings.ToUpper(strings.TrimSpace(r.NumeroSerie)))
		if m := macKey(r.MacAddress); m != "" {
			check("mac_address", m)
		}
		label := strings.TrimSpace(r.IdentificadorProprio)
		if n := strings.TrimSpace(r.ProdutoNome); n != "" {
			label = n + " · " + label
		}
		v := ImportRowValidation{Line: lineOf(i), Valid: len(probs) == 0, Problems: probs, Label: label}
		if v.Valid {
			res.Valid++
		} else {
			res.Invalid++
		}
		res.Rows = append(res.Rows, v)
	}
	return res
}

// ExistingStockItem — patrimônio já cadastrado na HubSoft.
type ExistingStockItem struct {
	ID            string
	ProdutoID     string
	ProdutoNome   string
	LocalID       string
	LocalNome     string
	StatusPrefix  string
	Status        string
	Identificador string
	Serie         string
	MAC           string
	CodigoItem    string
	Cliente       string
	Observacoes   string
}

func itemFromMap(m map[string]any) ExistingStockItem {
	it := ExistingStockItem{
		ID: pickStr(m, "id_produto_item"), Identificador: strings.TrimSpace(pickStr(m, "identificador_proprio")), Serie: strings.TrimSpace(pickStr(m, "numero_serie")),
		MAC: strings.TrimSpace(pickStr(m, "mac_address")), CodigoItem: pickStr(m, "codigo_item"), Observacoes: strings.TrimSpace(pickStr(m, "observacoes")),
	}
	if p, ok := m["produto"].(map[string]any); ok {
		it.ProdutoID, it.ProdutoNome = pickStr(p, "id_produto"), pickStr(p, "nome")
	}
	if l, ok := m["local_estoque"].(map[string]any); ok {
		it.LocalID, it.LocalNome = pickStr(l, "id_local_estoque"), pickStr(l, "descricao")
	}
	if s, ok := m["produto_item_status"].(map[string]any); ok {
		it.StatusPrefix, it.Status = pickStr(s, "prefixo"), pickStr(s, "descricao")
	}
	if cs, ok := m["cliente_servico"].(map[string]any); ok {
		if c, ok := cs["cliente"].(map[string]any); ok {
			it.Cliente = pickStr(c, "display", "nome_razaosocial")
		}
	}
	return it
}

// Describe resume onde o patrimônio está (para as mensagens de conflito).
func (e ExistingStockItem) Describe() string {
	s := fmt.Sprintf("id_produto_item %s (código %s, produto «%s», local «%s», status %s", e.ID, e.CodigoItem, e.ProdutoNome, e.LocalNome, e.Status)
	if e.Cliente != "" {
		s += ", cliente " + e.Cliente
	}
	return s + ")"
}

// StockItemIndex — patrimônios conhecidos, indexados por identificador, série, MAC e id.
type StockItemIndex struct {
	byID, byIdent, bySerie, byMAC map[string][]ExistingStockItem
}

func NewStockItemIndex() *StockItemIndex {
	return &StockItemIndex{byID: map[string][]ExistingStockItem{}, byIdent: map[string][]ExistingStockItem{}, bySerie: map[string][]ExistingStockItem{}, byMAC: map[string][]ExistingStockItem{}}
}

// Add indexa um patrimônio (ignora repetidos pelo mesmo id).
func (ix *StockItemIndex) Add(it ExistingStockItem) {
	if it.ID != "" {
		if len(ix.byID[it.ID]) > 0 {
			return
		}
		ix.byID[it.ID] = []ExistingStockItem{it}
	}
	if v := strings.ToLower(it.Identificador); v != "" {
		ix.byIdent[v] = append(ix.byIdent[v], it)
	}
	if v := strings.ToUpper(it.Serie); v != "" {
		ix.bySerie[v] = append(ix.bySerie[v], it)
	}
	if m := macKey(it.MAC); m != "" {
		ix.byMAC[m] = append(ix.byMAC[m], it)
	}
}

// Stock item verdicts (preflight / apply).
const (
	StockItemNovo               = "novo"
	StockItemJaExiste           = "ja_existe"
	StockItemJaExisteDivergente = "ja_existe_divergente"
	StockItemConflitoIdent      = "conflito_identificador"
	StockItemConflitoSerie      = "conflito_serie"
	StockItemConflitoMAC        = "conflito_mac"
	StockItemRetomar            = "retomar"
	StockItemRetomarInvalido    = "retomar_invalido"
)

// classifyStockItem decide o que fazer com uma linha, olhando o índice de patrimônios da HubSoft.
func classifyStockItem(r StockItemRow, ix *StockItemIndex) (verdict, msg string, found *ExistingStockItem) {
	pid := strings.TrimSpace(r.IDProduto)
	mac, _ := NormalizeMAC(r.MacAddress)
	ident := strings.ToLower(strings.TrimSpace(r.IdentificadorProprio))
	serie := strings.ToUpper(strings.TrimSpace(r.NumeroSerie))

	// Retomar um patrimônio que já foi criado (gravação dos identificadores falhou antes).
	if it := strings.TrimSpace(r.IDProdutoItem); it != "" {
		got := ix.byID[it]
		if len(got) == 0 {
			return StockItemRetomarInvalido, "o id_produto_item " + it + " não foi encontrado na HubSoft", nil
		}
		e := got[0]
		switch {
		case e.ProdutoID != pid:
			return StockItemRetomarInvalido, fmt.Sprintf("o patrimônio %s é do produto «%s» (id %s), não do id %s da linha", it, e.ProdutoNome, e.ProdutoID, pid), &e
		case e.Identificador != "" && !strings.EqualFold(e.Identificador, strings.TrimSpace(r.IdentificadorProprio)):
			return StockItemRetomarInvalido, fmt.Sprintf("o patrimônio %s já tem outro identificador («%s») — não será sobrescrito", it, e.Identificador), &e
		case e.Serie != "" && serie != "" && !strings.EqualFold(e.Serie, serie):
			return StockItemRetomarInvalido, fmt.Sprintf("o patrimônio %s já tem outra série («%s») — não será sobrescrita", it, e.Serie), &e
		}
		if serie != "" {
			for _, o := range ix.bySerie[serie] {
				if o.ID != e.ID {
					return StockItemConflitoSerie, fmt.Sprintf("o número de série «%s» já existe em outro patrimônio: %s", r.NumeroSerie, o.Describe()), &o
				}
			}
		}
		if mac != "" {
			for _, o := range ix.byMAC[macKey(mac)] {
				if o.ID != e.ID {
					return StockItemConflitoMAC, fmt.Sprintf("o MAC %s já existe em outro patrimônio: %s", mac, o.Describe()), &o
				}
			}
		}
		return StockItemRetomar, "patrimônio já criado — só será identificado (sem nova entrada de estoque)", &e
	}

	// Mesmo identificador: já foi importado (ou conflito se for de outro produto).
	if list := ix.byIdent[ident]; ident != "" && len(list) > 0 {
		e := list[0]
		if e.ProdutoID != pid {
			return StockItemConflitoIdent, fmt.Sprintf("o identificador «%s» já pertence a outro produto: %s", r.IdentificadorProprio, e.Describe()), &e
		}
		var diffs []string
		if serie != "" && !strings.EqualFold(e.Serie, serie) {
			diffs = append(diffs, fmt.Sprintf("série na HubSoft «%s» × arquivo «%s»", e.Serie, serie))
		}
		if mac != "" && macKey(e.MAC) != macKey(mac) {
			diffs = append(diffs, fmt.Sprintf("MAC na HubSoft «%s» × arquivo «%s»", e.MAC, mac))
		}
		if len(diffs) > 0 {
			return StockItemJaExisteDivergente, fmt.Sprintf("já existe %s, mas com dados diferentes (%s) — nada será alterado", e.Describe(), strings.Join(diffs, "; ")), &e
		}
		return StockItemJaExiste, "já existe na HubSoft — " + e.Describe() + " — nada será criado", &e
	}
	// Identificador alternativo (ex.: Nº patrimônio do IXC usado em cadastros antigos): se já existe, é o mesmo patrimônio.
	if alt := strings.ToLower(strings.TrimSpace(r.IdentificadorAlternativo)); alt != "" && alt != ident {
		for _, e := range ix.byIdent[alt] {
			if e.ProdutoID != pid {
				return StockItemConflitoIdent, fmt.Sprintf("o identificador antigo «%s» já pertence a outro produto: %s", r.IdentificadorAlternativo, e.Describe()), &e
			}
			var diffs []string
			if serie != "" && !strings.EqualFold(e.Serie, serie) {
				diffs = append(diffs, fmt.Sprintf("série na HubSoft «%s» × arquivo «%s»", e.Serie, serie))
			}
			if mac != "" && macKey(e.MAC) != macKey(mac) {
				diffs = append(diffs, fmt.Sprintf("MAC na HubSoft «%s» × arquivo «%s»", e.MAC, mac))
			}
			if len(diffs) > 0 {
				return StockItemJaExisteDivergente, fmt.Sprintf("já existe com o identificador antigo «%s»: %s, mas com dados diferentes (%s) — nada será alterado", r.IdentificadorAlternativo, e.Describe(), strings.Join(diffs, "; ")), &e
			}
			return StockItemJaExiste, fmt.Sprintf("já existe com o identificador antigo «%s» — %s — nada será criado", r.IdentificadorAlternativo, e.Describe()), &e
		}
	}
	if serie != "" {
		if list := ix.bySerie[serie]; len(list) > 0 {
			e := list[0]
			return StockItemConflitoSerie, fmt.Sprintf("o número de série «%s» já existe na HubSoft: %s — vai para as pendências", r.NumeroSerie, e.Describe()), &e
		}
	}
	if mac != "" {
		if list := ix.byMAC[macKey(mac)]; len(list) > 0 {
			e := list[0]
			return StockItemConflitoMAC, fmt.Sprintf("o MAC %s já existe na HubSoft: %s — vai para as pendências", mac, e.Describe()), &e
		}
	}
	return StockItemNovo, "patrimônio novo — será criado e identificado", nil
}

// StockItemPreRow — o que a importação VAI fazer com a linha (somente leitura).
type StockItemPreRow struct {
	Line      int    `json:"line"`
	Label     string `json:"label"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	IDItem    string `json:"id_produto_item,omitempty"`
	ProdutoOK bool   `json:"produto_ok"`
}

// StockItemPreflightSummary — visão geral do estoque atual da HubSoft (para o operador conferir antes de criar).
type StockItemPreflightSummary struct {
	ProdutosPatrimoniais int            `json:"produtos_patrimoniais"`
	PatrimoniosNaHubsoft int            `json:"patrimonios_na_hubsoft"`
	SemIdentificacao     int            `json:"sem_identificacao"` // patrimônios sem identificador, série e MAC (ex.: sobras de entradas)
	PorStatus            map[string]int `json:"por_status"`
}

// LoadStockItemIndex lê TODOS os patrimônios da HubSoft (produto a produto) — para achar duplicidade em qualquer produto.
func LoadStockItemIndex(ctx context.Context, cfg Config, token string, prods []ExistingStockProduct) (*StockItemIndex, StockItemPreflightSummary, error) {
	ix := NewStockItemIndex()
	sum := StockItemPreflightSummary{PorStatus: map[string]int{}}
	for _, p := range prods {
		if !p.Patrimonial {
			continue
		}
		sum.ProdutosPatrimoniais++
		items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/estoque/produto_item", map[string]string{"id_produto": p.ID}, 1000, "produto_itens", "produto_item")
		if err != nil {
			return nil, sum, fmt.Errorf("não foi possível listar os patrimônios do produto %s («%s»): %w", p.ID, p.Nome, err)
		}
		for _, m := range items {
			it := itemFromMap(m)
			if it.ProdutoID == "" {
				it.ProdutoID, it.ProdutoNome = p.ID, p.Nome
			}
			ix.Add(it)
			sum.PatrimoniosNaHubsoft++
			sum.PorStatus[firstNonEmpty(it.Status, "?")]++
			if it.Identificador == "" && it.Serie == "" && macKey(it.MAC) == "" {
				sum.SemIdentificacao++
			}
		}
	}
	return ix, sum, nil
}

// PreflightStockItems classifica cada linha contra os patrimônios existentes. Nada é alterado.
func PreflightStockItems(ctx context.Context, cfg Config, token string, rows []StockItemRow) ([]StockItemPreRow, StockItemPreflightSummary, error) {
	prods, err := ListStockProducts(ctx, cfg, token)
	if err != nil {
		return nil, StockItemPreflightSummary{}, err
	}
	meta := map[string]StockItemProduct{}
	for _, p := range prods {
		meta[p.ID] = StockItemProduct{Nome: p.Nome, Patrimonio: p.Patrimonial}
	}
	ix, sum, err := LoadStockItemIndex(ctx, cfg, token, prods)
	if err != nil {
		return nil, sum, err
	}
	out := make([]StockItemPreRow, 0, len(rows))
	for _, r := range rows {
		pr := StockItemPreRow{Line: r.Line, Label: strings.TrimSpace(r.IdentificadorProprio)}
		if n := strings.TrimSpace(r.ProdutoNome); n != "" {
			pr.Label = n + " · " + pr.Label
		}
		if probs := validateStockItemRow(r, meta); len(probs) > 0 {
			pr.Status, pr.Message = "erro_produto", strings.Join(probs, " | ")
			out = append(out, pr)
			continue
		}
		pr.ProdutoOK = true
		v, msg, e := classifyStockItem(r, ix)
		pr.Status, pr.Message = v, msg
		if e != nil {
			pr.IDItem = e.ID
		}
		out = append(out, pr)
	}
	return out, sum, nil
}

// macErrRe — mensagem de erro da HubSoft que fala do MAC (ex.: «O campo mac_address é inválido»).
var macErrRe = regexp.MustCompile(`(?i)(^|[^a-z])mac([^a-z]|$)`)

// notFoundMsg — mensagens da HubSoft que significam «não existe» (e não «falhei»).
var notFoundRe = regexp.MustCompile(`(?i)n[ãa]o\s+(foi\s+)?(encontrad|localizad)|nenhum|n[ãa]o\s+existe|sem\s+resultado`)

// lookupStockItems procura patrimônios por UM campo (GET produto_item/consultar). Um erro de comunicação/validação NUNCA
// vira «não encontrado» (isso levaria a criar duplicado): só vazio de verdade ou mensagem de «não encontrado».
func lookupStockItems(ctx context.Context, cfg Config, token, field, term string) ([]ExistingStockItem, error) {
	body, errMsg := stockGetQuery(ctx, cfg, token, "/api/v1/integracao/estoque/produto_item/consultar", map[string]string{"busca": field, "termo_busca": term})
	if errMsg != "" {
		if notFoundRe.MatchString(errMsg) {
			return nil, nil
		}
		return nil, fmt.Errorf("consulta por %s falhou: %s", field, errMsg)
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, fmt.Errorf("consulta por %s: resposta inválida", field)
	}
	var out []ExistingStockItem
	switch v := doc["produto"].(type) {
	case []any:
		for _, x := range v {
			if m, ok := x.(map[string]any); ok {
				out = append(out, itemFromMap(m))
			}
		}
	case map[string]any:
		out = append(out, itemFromMap(v))
	}
	return out, nil
}

// lookupRowDuplicates consulta a HubSoft por identificador, série e MAC desta linha e monta um mini-índice só com o que achou
// (comparando de forma EXATA — a busca da HubSoft pode ser por trecho).
func lookupRowDuplicates(ctx context.Context, cfg Config, token string, r StockItemRow) (*StockItemIndex, error) {
	ix := NewStockItemIndex()
	add := func(list []ExistingStockItem, match func(ExistingStockItem) bool) {
		for _, it := range list {
			if match(it) {
				ix.Add(it)
			}
		}
	}
	ident := strings.TrimSpace(r.IdentificadorProprio)
	list, err := lookupStockItems(ctx, cfg, token, "identificador_proprio", ident)
	if err != nil {
		return nil, err
	}
	add(list, func(it ExistingStockItem) bool { return strings.EqualFold(it.Identificador, ident) })
	if alt := strings.TrimSpace(r.IdentificadorAlternativo); alt != "" && !strings.EqualFold(alt, ident) {
		list, err = lookupStockItems(ctx, cfg, token, "identificador_proprio", alt)
		if err != nil {
			return nil, err
		}
		add(list, func(it ExistingStockItem) bool { return strings.EqualFold(it.Identificador, alt) })
	}
	if s := strings.TrimSpace(r.NumeroSerie); s != "" {
		list, err = lookupStockItems(ctx, cfg, token, "numero_serie", s)
		if err != nil {
			return nil, err
		}
		add(list, func(it ExistingStockItem) bool { return strings.EqualFold(it.Serie, s) })
	}
	if mac, _ := NormalizeMAC(r.MacAddress); mac != "" {
		list, err = lookupStockItems(ctx, cfg, token, "mac_address", mac)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 && !strings.EqualFold(macKey(mac), mac) { // a HubSoft pode ter o MAC gravado sem os dois-pontos
			if list, err = lookupStockItems(ctx, cfg, token, "mac_address", macKey(mac)); err != nil {
				return nil, err
			}
		}
		add(list, func(it ExistingStockItem) bool { return macKey(it.MAC) == macKey(mac) })
	}
	if it := strings.TrimSpace(r.IDProdutoItem); it != "" {
		body, errMsg := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto_item/"+it, nil)
		if errMsg != "" {
			if !notFoundRe.MatchString(errMsg) {
				return nil, fmt.Errorf("consulta do patrimônio %s falhou: %s", it, errMsg)
			}
		} else {
			var doc map[string]any
			if json.Unmarshal(body, &doc) == nil {
				if m, ok := doc["produto"].(map[string]any); ok {
					ix.Add(itemFromMap(m))
				}
			}
		}
	}
	return ix, nil
}

// StockItemResult — resultado de UMA linha, com as etapas separadas.
type StockItemResult struct {
	Line    int    `json:"line"`
	Label   string `json:"label"`
	OK      bool   `json:"ok"`
	Action  string `json:"action"`
	Message string `json:"message"`
	// IDProdutoItem/CodigoItem — o patrimônio criado (ou o que já existia).
	IDProdutoItem string `json:"id_produto_item,omitempty"`
	CodigoItem    string `json:"codigo_item,omitempty"`
	Created       bool   `json:"created,omitempty"`
	Rejected      bool   `json:"rejected,omitempty"`
	// Pending — a linha vai para o arquivo de pendências (conflito de série/MAC/identificador).
	Pending bool `json:"pending,omitempty"`
	// MacOmitted — a HubSoft recusou o MAC (formato inválido para ela): o patrimônio foi salvo SEM o MAC e o valor original foi
	// para as observações. MacOmittedReason traz a mensagem da HubSoft.
	MacOmitted       bool   `json:"mac_omitted,omitempty"`
	MacOmittedReason string `json:"mac_omitted_reason,omitempty"`
	IdentifyOK       *bool  `json:"identify_ok,omitempty"`
	IdentifyMsg      string `json:"identify_message,omitempty"`
	Verified         *bool  `json:"verified,omitempty"`
	VerifyMessage    string `json:"verify_message,omitempty"`
	Detail           string `json:"detail,omitempty"`
}

const (
	StockItemActionCreated        = "created"
	StockItemActionAlreadyExists  = "already_exists"
	StockItemActionConflict       = "conflict"
	StockItemActionRejected       = "rejected_local"
	StockItemActionFailed         = "failed"
	StockItemActionIdentifyFailed = "identify_failed"
	StockItemActionResumed        = "resumed"
)

func stockItemEntryBody(r StockItemRow) map[string]any {
	return map[string]any{
		"id_local_estoque": atoiSafe(r.IDLocalEstoque),
		"observacao":       "Importação de patrimônio do IXC via NetQuasar — " + strings.TrimSpace(r.IdentificadorProprio),
		"produtos":         []any{map[string]any{"quantidade": 1, "produto": map[string]any{"id_produto": atoiSafe(r.IDProduto)}}},
	}
}

func stockItemIdentifyBody(r StockItemRow) map[string]any {
	b := map[string]any{"identificador_proprio": strings.TrimSpace(r.IdentificadorProprio)}
	if s := strings.TrimSpace(r.NumeroSerie); s != "" {
		b["numero_serie"] = s
	}
	if mac, _ := NormalizeMAC(r.MacAddress); mac != "" {
		b["mac_address"] = mac
	}
	if o := strings.TrimSpace(r.Observacoes); o != "" {
		b["observacoes"] = o
	}
	return b
}

// ApplyStockItem cria (se preciso), identifica e confere UM patrimônio. `meta` (id_produto → produto) pode ser nil.
func ApplyStockItem(ctx context.Context, cfg Config, token string, r StockItemRow, meta map[string]StockItemProduct) StockItemResult {
	res := StockItemResult{Line: r.Line, Label: strings.TrimSpace(r.IdentificadorProprio)}
	if n := strings.TrimSpace(r.ProdutoNome); n != "" {
		res.Label = n + " · " + res.Label
	}
	if probs := validateStockItemRow(r, meta); len(probs) > 0 {
		res.Rejected, res.Action, res.Message = true, StockItemActionRejected, "recusada antes de enviar: "+strings.Join(probs, " | ")
		return res
	}

	// 0) o produto precisa existir e ter controle patrimonial — senão a entrada de estoque só aumentaria um saldo agrupado
	// (sem criar patrimônio) e deixaria uma movimentação indevida.
	if meta == nil {
		pbody, perr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto/"+strings.TrimSpace(r.IDProduto), nil)
		if perr != "" {
			res.Action, res.Message, res.Detail = StockItemActionFailed, "nada foi criado — não foi possível conferir o produto "+strings.TrimSpace(r.IDProduto)+" na HubSoft: "+perr, snippet(pbody)
			return res
		}
		var pd map[string]any
		_ = json.Unmarshal(pbody, &pd)
		pm, _ := pd["produto"].(map[string]any)
		if pm == nil || !pickBool(pm, "controle_patrimonial") {
			res.Rejected, res.Action = true, StockItemActionRejected
			res.Message = "recusada antes de enviar: o produto " + strings.TrimSpace(r.IDProduto) + " não existe ou não tem controle patrimonial (não gera patrimônio)"
			return res
		}
	}

	// 1) duplicidade em toda a HubSoft (consultas exatas)
	ix, err := lookupRowDuplicates(ctx, cfg, token, r)
	if err != nil {
		res.Action, res.Message = StockItemActionFailed, "nada foi criado — não foi possível conferir duplicidade na HubSoft: "+err.Error()
		return res
	}
	verdict, msg, found := classifyStockItem(r, ix)
	switch verdict {
	case StockItemJaExiste, StockItemJaExisteDivergente:
		res.OK, res.Action, res.Message = true, StockItemActionAlreadyExists, msg
		if verdict == StockItemJaExisteDivergente {
			res.OK, res.Pending = false, true
		}
		if found != nil {
			res.IDProdutoItem, res.CodigoItem = found.ID, found.CodigoItem
		}
		return res
	case StockItemConflitoIdent, StockItemConflitoSerie, StockItemConflitoMAC, StockItemRetomarInvalido:
		res.Action, res.Pending, res.Message = StockItemActionConflict, true, msg
		if found != nil {
			res.IDProdutoItem = found.ID
		}
		return res
	}

	// 2) entrada de estoque (cria o patrimônio vazio) — salvo quando é para só identificar um já criado
	if verdict == StockItemRetomar && found != nil {
		res.IDProdutoItem, res.CodigoItem = found.ID, found.CodigoItem
	} else {
		body, errMsg := stockCall(ctx, cfg, token, "POST", "/api/v1/integracao/estoque/movimento_estoque/entrada", stockItemEntryBody(r))
		if errMsg != "" {
			res.Action, res.Message, res.Detail = StockItemActionFailed, "a HubSoft recusou a entrada de estoque: "+errMsg, snippet(body)
			return res
		}
		ids := patrimonioIDsFromEntry(body)
		if len(ids) != 1 {
			res.Action, res.Detail = StockItemActionFailed, snippet(body)
			res.Message = fmt.Sprintf("a entrada de estoque foi aceita, mas devolveu %d patrimônio(s) em vez de 1 (ids: %s) — confira no estoque do local antes de repetir", len(ids), joinItemIDs(ids))
			return res
		}
		res.IDProdutoItem, res.Created = ids[0].ID, true
		res.CodigoItem = ids[0].Codigo
	}

	// 3) identificar (série/MAC/identificador/observações) — com até 3 tentativas
	var ibody []byte
	var ierr string
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(800 * time.Millisecond)
		}
		ibody, ierr = stockCall(ctx, cfg, token, "PUT", "/api/v1/integracao/estoque/produto_item/"+res.IDProdutoItem, stockItemIdentifyBody(r))
		if ierr == "" {
			break
		}
	}
	// A HubSoft rejeita alguns MACs (formato). Decisão do operador: salvar o MESMO patrimônio sem o MAC, com o valor original nas
	// observações. Só quando o erro fala de MAC — qualquer outro erro continua sendo erro (nunca é mascarado).
	vrow := r
	if ierr != "" && macErrRe.MatchString(ierr) {
		if mac, _ := NormalizeMAC(r.MacAddress); mac != "" {
			r2 := r
			r2.MacAddress = ""
			note := "MAC não aceito pela HubSoft (" + ierr + "): " + mac
			if o := strings.TrimSpace(r.Observacoes); o != "" {
				r2.Observacoes = o + " | " + note
			} else {
				r2.Observacoes = note
			}
			if b2, e2 := stockCall(ctx, cfg, token, "PUT", "/api/v1/integracao/estoque/produto_item/"+res.IDProdutoItem, stockItemIdentifyBody(r2)); e2 == "" {
				ibody, ierr, vrow = b2, "", r2
				res.MacOmitted, res.MacOmittedReason = true, note
			}
		}
	}
	iok := ierr == ""
	res.IdentifyOK = &iok
	if !iok {
		res.Action, res.IdentifyMsg, res.Detail = StockItemActionIdentifyFailed, ierr, snippet(ibody)
		what := "patrimônio JÁ CRIADO"
		if !res.Created {
			what = "patrimônio existente"
		}
		res.Message = fmt.Sprintf("%s (id_produto_item %s), mas a HubSoft recusou gravar os identificadores: %s — reenvie a linha com a coluna id_produto_item=%s para só identificá-lo", what, res.IDProdutoItem, ierr, res.IDProdutoItem)
		return res
	}
	res.IdentifyMsg = firstNonEmpty(hubsoftActionMessageWithErrors(ibody), "identificadores gravados")

	// 4) conferir
	vbody, verr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto_item/"+res.IDProdutoItem, nil)
	vok, vmsg := verifyStockItem(vbody, verr, vrow)
	res.Verified, res.VerifyMessage, res.OK = &vok, vmsg, vok
	res.Action = StockItemActionCreated
	if !res.Created {
		res.Action = StockItemActionResumed
	}
	if vok {
		res.Message = fmt.Sprintf("patrimônio %s (id_produto_item %s, código %s)", map[bool]string{true: "criado e identificado", false: "identificado"}[res.Created], res.IDProdutoItem, res.CodigoItem)
		if res.MacOmitted {
			res.Message += " — SEM o MAC: a HubSoft não aceitou o valor «" + strings.TrimSpace(r.MacAddress) + "»; ele ficou nas observações"
		}
	} else {
		res.Message = fmt.Sprintf("patrimônio gravado (id_produto_item %s), mas a conferência encontrou diferença: %s", res.IDProdutoItem, vmsg)
		res.Detail = snippet(vbody)
	}
	return res
}

type createdItem struct{ ID, Codigo string }

// patrimonioIDsFromEntry extrai os patrimônios criados da resposta da entrada de estoque.
func patrimonioIDsFromEntry(body []byte) []createdItem {
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil
	}
	mv, _ := doc["movimento_estoque"].(map[string]any)
	var out []createdItem
	for _, pr := range extractArray(mv, "produtos") {
		pm, ok := pr.(map[string]any)
		if !ok {
			continue
		}
		for _, it := range extractArray(pm, "patrimonios") {
			if m, ok := it.(map[string]any); ok {
				if id := pickStr(m, "id_produto_item"); id != "" {
					out = append(out, createdItem{ID: id, Codigo: pickStr(m, "codigo_item")})
				}
			}
		}
	}
	return out
}

// verifyStockItem compara o patrimônio relido com o que foi pedido.
func verifyStockItem(body []byte, callErr string, r StockItemRow) (bool, string) {
	if callErr != "" {
		return false, "não foi possível reler o patrimônio: " + callErr
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return false, "resposta inválida ao reler o patrimônio"
	}
	m, _ := doc["produto"].(map[string]any)
	if m == nil {
		return false, "a HubSoft não devolveu o patrimônio"
	}
	it := itemFromMap(m)
	var diffs []string
	if it.ProdutoID != strings.TrimSpace(r.IDProduto) {
		diffs = append(diffs, fmt.Sprintf("produto gravado «%s» (id %s) em vez do id %s", it.ProdutoNome, it.ProdutoID, strings.TrimSpace(r.IDProduto)))
	}
	if it.LocalID != strings.TrimSpace(r.IDLocalEstoque) {
		diffs = append(diffs, fmt.Sprintf("local «%s» (id %s) em vez do id %s", it.LocalNome, it.LocalID, strings.TrimSpace(r.IDLocalEstoque)))
	}
	if !strings.EqualFold(it.Identificador, strings.TrimSpace(r.IdentificadorProprio)) {
		diffs = append(diffs, fmt.Sprintf("identificador gravado «%s»", it.Identificador))
	}
	if s := strings.TrimSpace(r.NumeroSerie); s != "" && !strings.EqualFold(it.Serie, s) {
		diffs = append(diffs, fmt.Sprintf("série gravada «%s» em vez de «%s»", it.Serie, s))
	}
	if mac, _ := NormalizeMAC(r.MacAddress); mac != "" && macKey(it.MAC) != macKey(mac) {
		diffs = append(diffs, fmt.Sprintf("MAC gravado «%s» em vez de «%s»", it.MAC, mac))
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		return false, strings.Join(diffs, "; ")
	}
	extra := ""
	if it.StatusPrefix != "" && it.StatusPrefix != "estoque" {
		extra = fmt.Sprintf(" (atenção: status «%s»)", it.Status)
	}
	return true, "produto, local, identificador, série e MAC conferidos" + extra
}

// stockGetQuery faz um GET com parâmetros de consulta (mesmas regras de erro de stockCall).
func stockGetQuery(ctx context.Context, cfg Config, token, path string, q map[string]string) ([]byte, string) {
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{Method: "GET", Path: path, QueryParams: paramKVs(q)})
	b := ResponseBodyBytes(res)
	if !res.OK {
		if res.StatusCode == 0 {
			return b, firstNonEmpty(res.ErrorMessage, "sem resposta da HubSoft")
		}
		return b, firstNonEmpty(hubsoftActionMessageWithErrors(b), res.ErrorMessage, fmt.Sprintf("HTTP %d", res.StatusCode))
	}
	var doc struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(b, &doc) == nil && strings.EqualFold(doc.Status, "error") {
		return b, firstNonEmpty(hubsoftActionMessageWithErrors(b), "a HubSoft respondeu status=error")
	}
	return b, ""
}

func joinItemIDs(ids []createdItem) string {
	out := make([]string, 0, len(ids))
	for _, c := range ids {
		out = append(out, c.ID)
	}
	return strings.Join(out, ", ")
}
