package integrationhubsoft

import (
	"context"
	"fmt"
	"strings"
)

// --- Conferência (somente leitura) de PRODUTOS e PATRIMÔNIOS de estoque -------------------------------------------------
//
// A aba «Conferência» compara um CSV (o mesmo formato da importação) com o que está de fato na HubSoft, campo a campo, e diz
// por linha: igual, divergente (com a lista de diferenças), não encontrado ou duplicado. Nada é alterado.

// StockCheckDiff — uma diferença entre o arquivo e a HubSoft.
type StockCheckDiff struct {
	Campo   string `json:"campo"`
	Arquivo string `json:"arquivo"`
	Hubsoft string `json:"hubsoft"`
}

// StockCheckRow — resultado da conferência de UMA linha.
type StockCheckRow struct {
	Line   int    `json:"line"`
	Label  string `json:"label"`
	Status string `json:"status"` // ok | divergente | nao_encontrado | duplicado
	// ID — id do produto / patrimônio encontrado na HubSoft.
	ID     string            `json:"id,omitempty"`
	Diffs  []StockCheckDiff  `json:"diffs,omitempty"`
	Note   string            `json:"note,omitempty"`
	Aviso  string            `json:"aviso,omitempty"`  // o que a API não permite conferir
	Extras map[string]string `json:"extras,omitempty"` // dados informativos (status do patrimônio, local, cliente…)
}

const (
	StockCheckOK         = "ok"
	StockCheckDiverge    = "divergente"
	StockCheckNaoAchado  = "nao_encontrado"
	StockCheckDuplicado  = "duplicado"
	stockCheckProductMsg = "a HubSoft não devolve a configuração do produto (NF, venda, comodato, vínculos) na consulta — confira na tela do produto"
)

func eqNum(a float64, b string) bool {
	v, ok := parseMoney(b)
	if !ok {
		return false
	}
	d := a - v
	return d < 0.011 && d > -0.011
}

func yn(b bool) string {
	if b {
		return "sim"
	}
	return "não"
}

// CheckStockProducts compara cada linha do CSV de produtos com o produto da HubSoft (achado pelo código; na falta dele, pelo nome).
func CheckStockProducts(ctx context.Context, cfg Config, token string, rows []StockProductRow) ([]StockCheckRow, error) {
	items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/estoque/produto", nil, catalogMaxPages, "produtos")
	if err != nil {
		return nil, fmt.Errorf("não foi possível listar os produtos da HubSoft: %w", err)
	}
	byCode := map[string][]map[string]any{}
	byName := map[string][]map[string]any{}
	for _, m := range items {
		if c := strings.TrimSpace(pickStr(m, "codigo")); c != "" {
			byCode[c] = append(byCode[c], m)
		}
		k := alnumKey(pickStr(m, "nome"))
		byName[k] = append(byName[k], m)
	}
	out := make([]StockCheckRow, 0, len(rows))
	for _, r := range rows {
		cr := StockCheckRow{Line: r.Line, Label: strings.TrimSpace(r.Nome), Aviso: stockCheckProductMsg}
		var found []map[string]any
		how := ""
		if c := strings.TrimSpace(r.Codigo); c != "" {
			found, how = byCode[c], "código"
		}
		if len(found) == 0 {
			found, how = byName[alnumKey(r.Nome)], "nome"
		}
		switch {
		case len(found) == 0:
			cr.Status, cr.Note = StockCheckNaoAchado, "nenhum produto na HubSoft com este código ou nome"
			out = append(out, cr)
			continue
		case len(found) > 1:
			ids := make([]string, 0, len(found))
			for _, m := range found {
				ids = append(ids, pickStr(m, "id_produto"))
			}
			cr.Status, cr.Note = StockCheckDuplicado, fmt.Sprintf("mais de um produto com o mesmo %s na HubSoft (ids %s)", how, strings.Join(ids, ", "))
			out = append(out, cr)
			continue
		}
		m := found[0]
		cr.ID = pickStr(m, "id_produto")
		add := func(campo, arq, hub string) {
			cr.Diffs = append(cr.Diffs, StockCheckDiff{Campo: campo, Arquivo: arq, Hubsoft: hub})
		}
		if alnumKey(pickStr(m, "nome")) != alnumKey(r.Nome) {
			add("nome", strings.TrimSpace(r.Nome), pickStr(m, "nome"))
		}
		if c := strings.TrimSpace(r.Codigo); c != "" && strings.TrimSpace(pickStr(m, "codigo")) != c {
			add("código", c, pickStr(m, "codigo"))
		}
		if want := strings.TrimSpace(r.IDCategoria); want != "" {
			var ids []string
			switch c := m["produto_categoria"].(type) {
			case map[string]any:
				ids = append(ids, pickStr(c, "id_categoria"))
			case []any:
				for _, it := range c {
					if cm, ok := it.(map[string]any); ok {
						ids = append(ids, pickStr(cm, "id_categoria"))
					}
				}
			}
			ok := false
			for _, id := range ids {
				ok = ok || id == want
			}
			if !ok {
				add("categoria (id)", want, strings.Join(ids, ","))
			}
		}
		if want := strings.TrimSpace(r.IDMarca); want != "" {
			if mm, ok := m["produto_marca"].(map[string]any); !ok || pickStr(mm, "id_produto_marca") != want {
				got := ""
				if ok {
					got = fmt.Sprintf("%s (%s)", pickStr(mm, "id_produto_marca"), pickStr(mm, "nome"))
				}
				add("marca (id)", want, got)
			}
		}
		if t := strings.TrimSpace(r.IDTipo); t != "" {
			got := ""
			if tm, ok := m["produto_tipo"].(map[string]any); ok {
				got = pickStr(tm, "id_produto_tipo")
			}
			if got != t {
				add("tipo (id)", t, got)
			}
		}
		if !eqNum(pickFloat(m, "valor_compra"), r.ValorCompra) {
			add("valor de compra", r.ValorCompra, pickStr(m, "valor_compra"))
		}
		if !eqNum(pickFloat(m, "valor_venda"), r.ValorVenda) {
			add("valor de venda", r.ValorVenda, pickStr(m, "valor_venda"))
		}
		if want, ok := parseYesNo(r.ControlePatrimonial); ok && pickBool(m, "controle_patrimonial") != want {
			add("controle patrimonial", yn(want), yn(pickBool(m, "controle_patrimonial")))
		}
		if want, ok := parseYesNo(r.EPI); ok && pickBool(m, "epi") != want {
			add("EPI", yn(want), yn(pickBool(m, "epi")))
		}
		if u := strings.TrimSpace(r.UnidadeMedida); u != "" {
			got := ""
			if um, ok := m["unidade_medida"].(map[string]any); ok {
				got = pickStr(um, "abreviacao")
			}
			if !strings.EqualFold(got, u) {
				add("unidade", strings.ToUpper(u), got)
			}
		}
		if len(cr.Diffs) > 0 {
			cr.Status = StockCheckDiverge
		} else {
			cr.Status = StockCheckOK
		}
		out = append(out, cr)
	}
	return out, nil
}

// pickFloat lê um número (a HubSoft devolve valores como número JSON ou texto).
func pickFloat(m map[string]any, key string) float64 {
	v, _ := parseMoney(pickStr(m, key))
	return v
}

// CheckStockItems compara cada linha do CSV de patrimônios com os patrimônios da HubSoft. O patrimônio é achado pelo
// identificador (ou pelo identificador alternativo), depois pela série e pelo MAC; achar patrimônios DIFERENTES por
// campos diferentes é apontado como duplicado/inconsistente.
func CheckStockItems(ctx context.Context, cfg Config, token string, rows []StockItemRow) ([]StockCheckRow, StockItemPreflightSummary, error) {
	prods, err := ListStockProducts(ctx, cfg, token)
	if err != nil {
		return nil, StockItemPreflightSummary{}, err
	}
	ix, sum, err := LoadStockItemIndex(ctx, cfg, token, prods)
	if err != nil {
		return nil, sum, err
	}
	names := map[string]string{}
	for _, p := range prods {
		names[p.ID] = p.Nome
	}
	out := make([]StockCheckRow, 0, len(rows))
	for _, r := range rows {
		cr := StockCheckRow{Line: r.Line, Label: strings.TrimSpace(r.IdentificadorProprio)}
		if n := strings.TrimSpace(r.ProdutoNome); n != "" {
			cr.Label = n + " · " + cr.Label
		}
		ident := strings.ToLower(strings.TrimSpace(r.IdentificadorProprio))
		alt := strings.ToLower(strings.TrimSpace(r.IdentificadorAlternativo))
		serie := strings.ToUpper(strings.TrimSpace(r.NumeroSerie))
		mac, _ := NormalizeMAC(r.MacAddress)
		cands := map[string]ExistingStockItem{}
		via := map[string]string{}
		take := func(list []ExistingStockItem, why string) {
			for _, e := range list {
				if _, dup := cands[e.ID]; !dup {
					cands[e.ID] = e
					via[e.ID] = why
				}
			}
		}
		if ident != "" {
			take(ix.byIdent[ident], "identificador")
		}
		if alt != "" && alt != ident {
			take(ix.byIdent[alt], "identificador alternativo")
		}
		if len(cands) == 0 && serie != "" {
			take(ix.bySerie[serie], "série")
		}
		if len(cands) == 0 && macKey(mac) != "" {
			take(ix.byMAC[macKey(mac)], "MAC")
		}
		if len(cands) == 0 {
			cr.Status, cr.Note = StockCheckNaoAchado, "nenhum patrimônio na HubSoft com este identificador, série ou MAC"
			out = append(out, cr)
			continue
		}
		if len(cands) > 1 {
			var ds []string
			for _, e := range cands {
				ds = append(ds, e.Describe())
			}
			cr.Status, cr.Note = StockCheckDuplicado, "mais de um patrimônio corresponde a esta linha: "+strings.Join(ds, " | ")
			out = append(out, cr)
			continue
		}
		var e ExistingStockItem
		for _, v := range cands {
			e = v
		}
		cr.ID = e.ID
		cr.Extras = map[string]string{"status": e.Status, "local": e.LocalNome, "código do item": e.CodigoItem, "achado por": via[e.ID]}
		if e.Cliente != "" {
			cr.Extras["cliente (comodato)"] = e.Cliente
		}
		add := func(campo, arq, hub string) {
			cr.Diffs = append(cr.Diffs, StockCheckDiff{Campo: campo, Arquivo: arq, Hubsoft: hub})
		}
		if pid := strings.TrimSpace(r.IDProduto); pid != "" && e.ProdutoID != pid {
			add("produto (id)", fmt.Sprintf("%s (%s)", pid, names[pid]), fmt.Sprintf("%s (%s)", e.ProdutoID, e.ProdutoNome))
		}
		if ident != "" && !strings.EqualFold(e.Identificador, strings.TrimSpace(r.IdentificadorProprio)) {
			add("identificador próprio", strings.TrimSpace(r.IdentificadorProprio), e.Identificador)
		}
		if serie != "" && !strings.EqualFold(e.Serie, serie) {
			add("número de série", strings.TrimSpace(r.NumeroSerie), e.Serie)
		}
		if mac != "" && macKey(e.MAC) != macKey(mac) {
			add("MAC", mac, e.MAC)
		}
		if l := strings.TrimSpace(r.IDLocalEstoque); l != "" && e.LocalID != l {
			add("local de estoque (id)", l, fmt.Sprintf("%s (%s)", e.LocalID, e.LocalNome))
		}
		if o := strings.TrimSpace(r.Observacoes); o != "" && strings.TrimSpace(e.Observacoes) != o {
			add("observações", o, e.Observacoes)
		}
		if len(cr.Diffs) > 0 {
			cr.Status = StockCheckDiverge
		} else {
			cr.Status = StockCheckOK
		}
		out = append(out, cr)
	}
	return out, sum, nil
}
