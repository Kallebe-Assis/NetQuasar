package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Cadastro em massa de PRODUTOS de estoque (permissão integrations.hubsoft_bulk) ----------------------------------
//
// Passo 1 da migração de patrimônios do IXC: criar os produtos (ex.: «ROTEADOR MERCUSYS MR30G AC1200») que depois
// receberão os patrimônios. Cada produto tem a sua CONFIGURAÇÃO (incluir na NF, permite venda/comodato ao cliente,
// vínculo com POP/projeto/usuário/composição) — que a API só aceita no PUT, depois do POST. Por isso cada linha faz:
//  1. confere se já existe (por código ou por nome) — nunca cria duplicado;
//  2. POST /estoque/produto;
//  3. PUT  /estoque/produto/{id} com produto_configuracao;
//  4. GET  /estoque/produto/{id} para conferir o que ficou gravado.
// O resultado de cada etapa é devolvido separado (criação × configuração × conferência) para que uma falha na
// configuração nunca pareça «produto não criado» — e vice-versa.

// StockProductRow é uma linha do CSV de produtos. Tudo texto: a validação decide o que é válido.
type StockProductRow struct {
	Line          int    `json:"line"`
	Codigo        string `json:"codigo"` // gravado no campo «código» do produto (aqui: o ID do produto no IXC)
	Nome          string `json:"nome"`
	IDCategoria   string `json:"id_categoria"`
	IDMarca       string `json:"id_marca"`
	IDTipo        string `json:"id_tipo"` // opcional
	UnidadeMedida string `json:"unidade_medida"`
	// Controle patrimonial: sim = cada unidade vira um patrimônio (série/MAC próprios).
	ControlePatrimonial string `json:"controle_patrimonial"`
	EPI                 string `json:"epi"` // opcional (padrão: não)
	ValorCompra         string `json:"valor_compra"`
	ValorVenda          string `json:"valor_venda"`
	// RepairBrand — preenchido pelo handler (não vem do CSV): em produto que JÁ existe com este código, tenta corrigir a marca.
	RepairBrand   bool   `json:"repair_brand,omitempty"`
	EstoqueMinimo string `json:"estoque_minimo"` // opcional no CSV (vazio = 0); a HubSoft o exige

	// Configuração do produto — as 7 são obrigatórias (nada é gravado «por omissão» sem o operador ver).
	IncluirNotaFiscal               string `json:"incluir_nota_fiscal"`
	PermiteVendaCliente             string `json:"permite_venda_cliente"`
	PermiteComodatoCliente          string `json:"permite_comodato_cliente"`
	PermiteVinculoPop               string `json:"permite_vinculo_pop"`
	PermiteVinculoProjetoMapeamento string `json:"permite_vinculo_projeto_mapeamento"`
	PermiteVinculoUsuario           string `json:"permite_vinculo_usuario"`
	PermiteVinculoComposicao        string `json:"permite_vinculo_composicao"`
}

// stockConfigFields — nome na API (produto_configuracao.*) → acesso ao campo da linha, na ordem exibida.
func (r StockProductRow) configFields() []struct{ Key, Val string } {
	return []struct{ Key, Val string }{
		{"incluir_nota_fiscal", r.IncluirNotaFiscal},
		{"permite_venda_cliente", r.PermiteVendaCliente},
		{"permite_comodato_cliente", r.PermiteComodatoCliente},
		{"permite_vinculo_pop", r.PermiteVinculoPop},
		{"permite_vinculo_projeto_mapeamento", r.PermiteVinculoProjetoMapeamento},
		{"permite_vinculo_usuario", r.PermiteVinculoUsuario},
		{"permite_vinculo_composicao", r.PermiteVinculoComposicao},
	}
}

// StockCatalogSets — IDs válidos de categoria, marca e tipo de produto (nil = «não consegui conferir», nunca «vazio»).
type StockCatalogSets struct {
	Categoria map[string]bool
	Marca     map[string]bool
	Tipo      map[string]bool
}

// LoadStockCatalogSets lê as listas reais da HubSoft. Falha ao ler um catálogo não bloqueia: ele vem nil e o nome vai
// para `unchecked`, para a tela avisar que aquele ID só passou na checagem de formato.
func LoadStockCatalogSets(ctx context.Context, cfg Config, token string) (sets StockCatalogSets, unchecked []string) {
	load := func(which, label string) map[string]bool {
		code, body, err := FetchCatalog(ctx, cfg, token, which)
		if err != nil || code < 200 || code >= 300 {
			unchecked = append(unchecked, label)
			return nil
		}
		set := CatalogSetsFromJSON(which, body)
		if set == nil {
			unchecked = append(unchecked, label)
		}
		return set
	}
	sets.Categoria = load("produto_categoria", "categorias de produto")
	sets.Marca = load("produto_marca", "marcas de produto")
	sets.Tipo = load("produto_tipo", "tipos de produto")
	return sets, unchecked
}

// parseMoney aceita «79,90», «79.90» e «1.234,56».
func parseMoney(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func isUnitSigla(s string) bool {
	if len(s) < 1 || len(s) > 6 {
		return false
	}
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}

// validateStockProductRow confere o formato de UMA linha (sem consultar a HubSoft). É a mesma checagem usada na fase de
// validação e, de novo, antes de enviar cada produto — nenhuma linha malformada chega à API.
func validateStockProductRow(r StockProductRow, cat StockCatalogSets) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }
	if strings.TrimSpace(r.Nome) == "" {
		add("nome é obrigatório")
	} else if len([]rune(strings.TrimSpace(r.Nome))) > 190 {
		add("nome muito longo (%d caracteres)", len([]rune(strings.TrimSpace(r.Nome))))
	}
	checkID := func(label, id string, set map[string]bool, required bool) {
		id = strings.TrimSpace(id)
		if id == "" {
			if required {
				add("%s é obrigatório", label)
			}
			return
		}
		if !isPosInt(id) {
			add("%s precisa ser um número positivo (veio %q)", label, id)
			return
		}
		if set != nil && !set[id] {
			add("%s %s não existe na HubSoft (confira a aba Catálogos)", label, id)
		}
	}
	checkID("id_categoria", r.IDCategoria, cat.Categoria, true)
	checkID("id_marca", r.IDMarca, cat.Marca, true)
	checkID("id_tipo", r.IDTipo, cat.Tipo, false)
	if !isUnitSigla(strings.TrimSpace(r.UnidadeMedida)) {
		add("unidade_medida precisa ser a sigla da unidade (ex.: UN) — veio %q", r.UnidadeMedida)
	}
	if _, ok := parseYesNo(r.ControlePatrimonial); !ok {
		add("controle_patrimonial precisa ser sim ou não (veio %q)", r.ControlePatrimonial)
	}
	if strings.TrimSpace(r.EPI) != "" {
		if _, ok := parseYesNo(r.EPI); !ok {
			add("epi precisa ser sim ou não (veio %q)", r.EPI)
		}
	}
	for _, f := range []struct{ n, v string }{{"valor_compra", r.ValorCompra}, {"valor_venda", r.ValorVenda}} {
		if _, ok := parseMoney(f.v); !ok {
			add("%s precisa ser um valor maior ou igual a 0 (veio %q)", f.n, f.v)
		}
	}
	if strings.TrimSpace(r.EstoqueMinimo) != "" {
		if _, ok := parseMoney(r.EstoqueMinimo); !ok {
			add("estoque_minimo inválido (veio %q)", r.EstoqueMinimo)
		}
	}
	for _, f := range r.configFields() {
		if _, ok := parseYesNo(f.Val); !ok {
			add("%s precisa ser sim ou não (veio %q) — a configuração do produto é obrigatória", f.Key, f.Val)
		}
	}
	return p
}

// ValidateStockProductRows valida o lote inteiro e ainda aponta nome/código repetidos DENTRO do arquivo.
func ValidateStockProductRows(rows []StockProductRow, cat StockCatalogSets) ImportValidationResult {
	res := ImportValidationResult{OK: true, Rows: make([]ImportRowValidation, 0, len(rows))}
	seenName, seenCode := map[string]int{}, map[string]int{}
	for i, r := range rows {
		line := r.Line
		if line <= 0 {
			line = i + 2
		}
		probs := validateStockProductRow(r, cat)
		if k := normText(r.Nome); k != "" {
			if prev, dup := seenName[k]; dup {
				probs = append(probs, fmt.Sprintf("nome repetido no arquivo (já na linha %d)", prev))
			} else {
				seenName[k] = line
			}
		}
		if c := strings.TrimSpace(r.Codigo); c != "" {
			if prev, dup := seenCode[c]; dup {
				probs = append(probs, fmt.Sprintf("código repetido no arquivo (já na linha %d)", prev))
			} else {
				seenCode[c] = line
			}
		}
		v := ImportRowValidation{Line: line, Valid: len(probs) == 0, Problems: probs, Label: strings.TrimSpace(r.Nome)}
		if v.Valid {
			res.Valid++
		} else {
			res.Invalid++
		}
		res.Rows = append(res.Rows, v)
	}
	return res
}

// ExistingStockProduct — produto já cadastrado na HubSoft.
type ExistingStockProduct struct {
	ID     string
	Nome   string
	Codigo string
}

// ListStockProducts lê TODOS os produtos da HubSoft (paginado).
func ListStockProducts(ctx context.Context, cfg Config, token string) ([]ExistingStockProduct, error) {
	items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/estoque/produto", nil, catalogMaxPages, "produtos")
	if err != nil {
		return nil, fmt.Errorf("não foi possível listar os produtos da HubSoft: %w", err)
	}
	out := make([]ExistingStockProduct, 0, len(items))
	for _, m := range items {
		out = append(out, ExistingStockProduct{ID: pickStr(m, "id_produto"), Nome: strings.TrimSpace(pickStr(m, "nome")), Codigo: strings.TrimSpace(pickStr(m, "codigo"))})
	}
	return out, nil
}

// StockProductIndex permite achar produto existente por código (exato) ou por nome (sem acento/caixa/pontuação).
type StockProductIndex struct {
	byCode map[string]ExistingStockProduct
	byName map[string][]ExistingStockProduct
}

func newStockProductIndex(list []ExistingStockProduct) *StockProductIndex {
	ix := &StockProductIndex{byCode: map[string]ExistingStockProduct{}, byName: map[string][]ExistingStockProduct{}}
	for _, p := range list {
		if p.Codigo != "" {
			ix.byCode[p.Codigo] = p
		}
		k := alnumKey(p.Nome)
		ix.byName[k] = append(ix.byName[k], p)
	}
	return ix
}

// add registra um produto recém-criado, para que uma linha repetida mais adiante no mesmo lote não seja criada de novo.
func (ix *StockProductIndex) add(p ExistingStockProduct) {
	if p.Codigo != "" {
		ix.byCode[p.Codigo] = p
	}
	k := alnumKey(p.Nome)
	ix.byName[k] = append(ix.byName[k], p)
}

// find devolve o produto existente que corresponde à linha e como casou: "codigo" | "nome".
func (ix *StockProductIndex) find(r StockProductRow) (ExistingStockProduct, string, bool) {
	if c := strings.TrimSpace(r.Codigo); c != "" {
		if p, ok := ix.byCode[c]; ok {
			return p, "codigo", true
		}
	}
	if v := ix.byName[alnumKey(r.Nome)]; len(v) > 0 {
		return v[0], "nome", true
	}
	return ExistingStockProduct{}, "", false
}

// StockPreflightRow — o que a importação VAI fazer com a linha (somente leitura).
type StockPreflightRow struct {
	Line    int    `json:"line"`
	Label   string `json:"label"`
	Status  string `json:"status"` // novo | ja_existe_codigo | ja_existe_nome
	Message string `json:"message"`
	// IDProduto — id do produto que já existe (quando houver).
	IDProduto string `json:"id_produto,omitempty"`
}

// PreflightStockProducts compara as linhas com os produtos já cadastrados. Nada é alterado.
func PreflightStockProducts(ctx context.Context, cfg Config, token string, rows []StockProductRow) ([]StockPreflightRow, error) {
	list, err := ListStockProducts(ctx, cfg, token)
	if err != nil {
		return nil, err
	}
	ix := newStockProductIndex(list)
	out := make([]StockPreflightRow, 0, len(rows))
	for _, r := range rows {
		pr := StockPreflightRow{Line: r.Line, Label: strings.TrimSpace(r.Nome), Status: "novo", Message: "produto novo — será criado e configurado"}
		if p, how, ok := ix.find(r); ok {
			pr.IDProduto = p.ID
			if how == "codigo" {
				pr.Status = "ja_existe_codigo"
				pr.Message = fmt.Sprintf("já existe um produto com o código %s (id %s, «%s») — não será criado de novo", r.Codigo, p.ID, p.Nome)
			} else {
				pr.Status = "ja_existe_nome"
				pr.Message = fmt.Sprintf("já existe um produto com este nome (id %s, código «%s») — não será criado de novo", p.ID, p.Codigo)
			}
		}
		out = append(out, pr)
	}
	return out, nil
}

// StockApplyResult — resultado de UMA linha, com as três etapas separadas.
type StockApplyResult struct {
	Line      int    `json:"line"`
	Label     string `json:"label"`
	OK        bool   `json:"ok"` // criado (ou já existia) E configurado/conferido sem problema
	Action    string `json:"action"`
	Message   string `json:"message"`
	IDProduto string `json:"id_produto,omitempty"`
	// Rejected — recusada localmente (formato), nada foi enviado à HubSoft.
	Rejected bool `json:"rejected,omitempty"`
	// Created — o POST criou o produto agora.
	Created bool `json:"created,omitempty"`
	// ConfigOK/ConfigMessage — resultado do PUT de configuração (nil = não houve tentativa).
	ConfigOK      *bool  `json:"config_ok,omitempty"`
	ConfigMessage string `json:"config_message,omitempty"`
	// Verified/VerifyMessage — conferência (GET) depois de gravar.
	Verified      *bool  `json:"verified,omitempty"`
	VerifyMessage string `json:"verify_message,omitempty"`
	// BrandRepair — o que foi tentado/feito para corrigir a marca (vazio se não foi preciso).
	BrandRepair string `json:"brand_repair,omitempty"`
	// Detail — trecho do corpo bruto da resposta da HubSoft quando houve erro (para diagnóstico).
	Detail string `json:"detail,omitempty"`
}

const (
	StockActionCreated           = "created"
	StockActionAlreadyExists     = "already_exists"
	StockActionRejected          = "rejected_local"
	StockActionFailed            = "failed"
	StockActionConfigFailed      = "config_failed"
	StockActionBrandRepaired     = "brand_repaired"
	StockActionBrandRepairFailed = "brand_repair_failed"
)

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

// stockCreateBody monta o corpo do POST /estoque/produto. A documentação da HubSoft é inconsistente sobre a chave do ID
// da categoria (tabela: id_protudo_categoria; exemplo: id_categoria) — envia as duas, com o mesmo valor.
func stockCreateBody(r StockProductRow, marcaNome string) map[string]any {
	compra, _ := parseMoney(r.ValorCompra)
	venda, _ := parseMoney(r.ValorVenda)
	patr, _ := parseYesNo(r.ControlePatrimonial)
	epi, _ := parseYesNo(r.EPI)
	cat := atoiSafe(r.IDCategoria)
	body := map[string]any{
		"nome": strings.TrimSpace(r.Nome), "unidade_medida": strings.ToUpper(strings.TrimSpace(r.UnidadeMedida)),
		"valor_compra": compra, "valor_venda": venda, "controle_patrimonial": patr, "epi": epi,
		"produto_categoria": map[string]any{"id_categoria": cat, "id_protudo_categoria": cat},
		// A HubSoft IGNOROU o id_produto_marca sozinho e criou uma marca nova «API» («Criado via API»). Envia também o nome
		// EXATO da marca existente (vem do catálogo), que é o que ela usa para localizar a marca.
		"produto_marca":    map[string]any{"id_produto_marca": atoiSafe(r.IDMarca), "nome": marcaNome},
		"id_produto_marca": atoiSafe(r.IDMarca), // algumas versões da API leem a marca na raiz do corpo
	}
	if c := strings.TrimSpace(r.Codigo); c != "" {
		body["codigo"] = c
	}
	if t := strings.TrimSpace(r.IDTipo); t != "" {
		body["produto_tipo"] = map[string]any{"id_produto_tipo": atoiSafe(t)}
	}
	// A HubSoft EXIGE estoque_minimo ("O campo estoque minimo é obrigatório"), embora a documentação o dê como opcional.
	// Coluna vazia = 0.
	estMin := 0.0
	if m := strings.TrimSpace(r.EstoqueMinimo); m != "" {
		estMin, _ = parseMoney(m)
	}
	body["estoque_minimo"] = estMin
	return body
}

func stockConfigBody(r StockProductRow) map[string]any {
	cfg := map[string]any{}
	for _, f := range r.configFields() {
		v, _ := parseYesNo(f.Val)
		cfg[f.Key] = v
	}
	return map[string]any{"produto_configuracao": cfg}
}

// stockCall executa uma chamada JSON e devolve (corpo, mensagem de erro legível — vazia se status 2xx e status≠error).
func stockCall(ctx context.Context, cfg Config, token, method, path string, body any) ([]byte, string) {
	req := integrationhttp.RequestConfig{Method: method, Path: path}
	if body != nil {
		raw, _ := json.Marshal(body)
		req.BodyType = "json"
		req.BodyTemplate = string(raw)
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), req)
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

// ApplyStockProduct cria (se preciso), configura e confere UM produto. `list` é a lista atual de produtos da HubSoft
// (para a checagem de duplicidade); o chamador a carrega UMA vez por lote.
func ApplyStockProduct(ctx context.Context, cfg Config, token string, r StockProductRow, ix *StockProductIndex, brands map[string]string) StockApplyResult {
	res := StockApplyResult{Line: r.Line, Label: strings.TrimSpace(r.Nome)}
	if probs := validateStockProductRow(r, StockCatalogSets{}); len(probs) > 0 {
		res.Rejected, res.Action = true, StockActionRejected
		res.Message = "recusada antes de enviar: " + strings.Join(probs, " | ")
		return res
	}
	marcaNome := strings.TrimSpace(brands[strings.TrimSpace(r.IDMarca)])
	if marcaNome == "" {
		// Sem o nome da marca a HubSoft poderia criar uma marca nova (já aconteceu: «API»). Melhor não enviar nada.
		res.Rejected, res.Action = true, StockActionRejected
		res.Message = "recusada antes de enviar: a marca id " + strings.TrimSpace(r.IDMarca) + " não foi encontrada no catálogo de marcas da HubSoft (evita criar uma marca nova por engano)"
		return res
	}
	if p, how, ok := ix.find(r); ok {
		if r.RepairBrand && how == "codigo" {
			return repairExistingBrand(ctx, cfg, token, r, p, marcaNome, res)
		}
		res.OK, res.Action, res.IDProduto = true, StockActionAlreadyExists, p.ID
		if how == "codigo" {
			res.Message = fmt.Sprintf("já existe um produto com o código %s (id %s) — nada foi criado nem alterado", r.Codigo, p.ID)
		} else {
			res.Message = fmt.Sprintf("já existe um produto com este nome (id %s) — nada foi criado nem alterado", p.ID)
		}
		return res
	}

	// 1) criar
	body, errMsg := stockCall(ctx, cfg, token, "POST", "/api/v1/integracao/estoque/produto", stockCreateBody(r, marcaNome))
	if errMsg != "" {
		res.Action, res.Message, res.Detail = StockActionFailed, "a HubSoft recusou a criação: "+errMsg, snippet(body)
		return res
	}
	var created struct {
		Produto struct {
			ID json.Number `json:"id_produto"`
		} `json:"produto"`
	}
	_ = json.Unmarshal(body, &created)
	res.IDProduto = created.Produto.ID.String()
	if res.IDProduto == "" || res.IDProduto == "0" {
		// Criou mas não devolveu o id: acha pelo código/nome para poder configurar e conferir.
		if list, err := ListStockProducts(ctx, cfg, token); err == nil {
			if p, _, ok := newStockProductIndex(list).find(r); ok {
				res.IDProduto = p.ID
			}
		}
	}
	res.Created = true
	if res.IDProduto != "" {
		ix.add(ExistingStockProduct{ID: res.IDProduto, Nome: strings.TrimSpace(r.Nome), Codigo: strings.TrimSpace(r.Codigo)})
	}
	if res.IDProduto == "" {
		res.Action = StockActionConfigFailed
		res.Message = "produto criado, mas a HubSoft não devolveu o id e ele não foi encontrado na lista — configure-o manualmente"
		res.Detail = snippet(body)
		return res
	}

	// 2) configurar
	cbody, cErr := stockCall(ctx, cfg, token, "PUT", "/api/v1/integracao/estoque/produto/"+res.IDProduto, stockConfigBody(r))
	cok := cErr == ""
	res.ConfigOK = &cok
	if !cok {
		res.Action = StockActionConfigFailed
		res.ConfigMessage = cErr
		res.Message = fmt.Sprintf("produto CRIADO (id %s), mas a configuração NÃO foi gravada: %s", res.IDProduto, cErr)
		res.Detail = snippet(cbody)
		return res
	}
	res.ConfigMessage = firstNonEmpty(hubsoftActionMessageWithErrors(cbody), "configuração gravada")

	// 3) conferir
	vbody, vErr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto/"+res.IDProduto, nil)
	vok, vmsg := verifyStockProduct(vbody, vErr, r)
	if !vok && strings.Contains(vmsg, "MARCA gravada") {
		fixed, how, nb := repairStockBrand(ctx, cfg, token, res.IDProduto, r, marcaNome)
		res.BrandRepair = how
		if fixed {
			vbody, vErr = nb, ""
			vok, vmsg = verifyStockProduct(vbody, vErr, r)
		}
	}
	res.Verified, res.VerifyMessage = &vok, vmsg
	res.OK = vok
	res.Action = StockActionCreated
	if vok {
		res.Message = fmt.Sprintf("produto criado e configurado (id %s)", res.IDProduto)
	} else {
		res.Message = fmt.Sprintf("produto criado e configuração enviada (id %s), mas a conferência encontrou diferença: %s", res.IDProduto, vmsg)
		res.Detail = snippet(vbody)
	}
	return res
}

// verifyStockProduct compara o que a HubSoft devolveu com o enviado. O nome é sempre conferido; a configuração só
// quando a HubSoft a devolve na consulta (a documentação não garante) — senão avisa que não pôde ser conferida.
func verifyStockProduct(body []byte, callErr string, r StockProductRow) (bool, string) {
	if callErr != "" {
		return false, "não foi possível reler o produto: " + callErr
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return false, "resposta inválida ao reler o produto"
	}
	p, _ := doc["produto"].(map[string]any)
	if p == nil {
		return false, "a HubSoft não devolveu o produto"
	}
	var diffs []string
	if alnumKey(pickStr(p, "nome")) != alnumKey(r.Nome) {
		diffs = append(diffs, fmt.Sprintf("nome gravado «%s»", pickStr(p, "nome")))
	}
	if c := strings.TrimSpace(r.Codigo); c != "" && strings.TrimSpace(pickStr(p, "codigo")) != c {
		diffs = append(diffs, fmt.Sprintf("código gravado «%s» (esperado «%s»)", pickStr(p, "codigo"), c))
	}
	// Marca e categoria: o valor gravado precisa ser o pedido (foi assim que a marca «API» passou despercebida).
	if want := strings.TrimSpace(r.IDMarca); want != "" {
		if m, ok := p["produto_marca"].(map[string]any); ok {
			if got := pickStr(m, "id_produto_marca"); got != "" && got != want {
				diffs = append(diffs, fmt.Sprintf("MARCA gravada «%s» (id %s) em vez do id %s", pickStr(m, "nome"), got, want))
			}
		}
	}
	if want := strings.TrimSpace(r.IDCategoria); want != "" {
		ids := []string{}
		switch c := p["produto_categoria"].(type) {
		case map[string]any:
			ids = append(ids, pickStr(c, "id_categoria"))
		case []any:
			for _, it := range c {
				if m, ok := it.(map[string]any); ok {
					ids = append(ids, pickStr(m, "id_categoria"))
				}
			}
		}
		if len(ids) > 0 {
			found := false
			for _, id := range ids {
				found = found || id == want
			}
			if !found {
				diffs = append(diffs, fmt.Sprintf("CATEGORIA gravada (id %s) em vez do id %s", strings.Join(ids, ","), want))
			}
		}
	}
	checkedCfg := false
	if pc, ok := p["produto_configuracao"].(map[string]any); ok {
		checkedCfg = true
		for _, f := range r.configFields() {
			want, _ := parseYesNo(f.Val)
			if _, present := pc[f.Key]; present {
				if got := pickBool(pc, f.Key); got != want {
					diffs = append(diffs, fmt.Sprintf("%s=%v (esperado %v)", f.Key, got, want))
				}
			}
		}
	}
	if len(diffs) > 0 {
		return false, strings.Join(diffs, "; ")
	}
	if checkedCfg {
		return true, "nome, código, marca, categoria e configuração conferidos"
	}
	return true, "nome, código, marca e categoria conferidos (a HubSoft não devolve a configuração na consulta — confira na tela do produto)"
}

// NewStockProductIndex carrega a lista atual e devolve o índice usado por ApplyStockProduct.
func NewStockProductIndex(ctx context.Context, cfg Config, token string) (*StockProductIndex, error) {
	list, err := ListStockProducts(ctx, cfg, token)
	if err != nil {
		return nil, err
	}
	return newStockProductIndex(list), nil
}

// LoadStockBrandNames devolve id → nome de cada marca da HubSoft. O NOME EXATO é enviado junto do id ao criar o produto
// (a HubSoft ignorou o id sozinho e criou a marca «API»). Falha ao ler = erro: sem isso nenhum produto deve ser criado.
func LoadStockBrandNames(ctx context.Context, cfg Config, token string) (map[string]string, error) {
	code, body, err := FetchCatalog(ctx, cfg, token, "produto_marca")
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler as marcas da HubSoft: %w", err)
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("não foi possível ler as marcas da HubSoft: HTTP %d", code)
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, fmt.Errorf("resposta inválida ao ler as marcas da HubSoft")
	}
	out := map[string]string{}
	for _, it := range extractArray(doc, "produto_marcas") {
		if m, ok := it.(map[string]any); ok {
			if id, nome := pickStr(m, "id_produto_marca"), strings.TrimSpace(pickStr(m, "nome")); id != "" && nome != "" {
				out[id] = nome
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("a lista de marcas da HubSoft veio vazia")
	}
	return out, nil
}

// brandOf lê a marca (id, nome) de um produto devolvido pela HubSoft.
func brandOf(p map[string]any) (id, nome string) {
	if m, ok := p["produto_marca"].(map[string]any); ok {
		return pickStr(m, "id_produto_marca"), pickStr(m, "nome")
	}
	return "", ""
}

// brandVariants — formas de pedir a troca da marca no PUT. A documentação da HubSoft não lista a marca entre os campos
// editáveis, e o POST já ignorou o id, então cada forma é TENTADA e conferida relendo o produto (nunca presumida).
func brandVariants(r StockProductRow, nome string) []struct {
	Name string
	Body map[string]any
} {
	id := atoiSafe(r.IDMarca)
	return []struct {
		Name string
		Body map[string]any
	}{
		{"produto_marca{id_produto_marca}", map[string]any{"produto_marca": map[string]any{"id_produto_marca": id}}},
		{"id_produto_marca na raiz", map[string]any{"id_produto_marca": id}},
		{"produto_marca{id_produto_marca, nome}", map[string]any{"produto_marca": map[string]any{"id_produto_marca": id, "nome": nome}}},
		{"produto_marca como número", map[string]any{"produto_marca": id}},
	}
}

// repairStockBrand tenta corrigir a marca de um produto já criado, uma forma por vez, relendo o produto a cada tentativa.
// Devolve (corrigiu, descrição do que foi tentado, corpo do último GET).
func repairStockBrand(ctx context.Context, cfg Config, token, id string, r StockProductRow, marcaNome string) (bool, string, []byte) {
	var tried []string
	var last []byte
	for _, v := range brandVariants(r, marcaNome) {
		_, perr := stockCall(ctx, cfg, token, "PUT", "/api/v1/integracao/estoque/produto/"+id, v.Body)
		if perr != "" {
			tried = append(tried, v.Name+" → recusada: "+perr)
			continue
		}
		gb, gerr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto/"+id, nil)
		last = gb
		if gerr != "" {
			tried = append(tried, v.Name+" → enviada, mas não consegui reler: "+gerr)
			continue
		}
		var doc map[string]any
		_ = json.Unmarshal(gb, &doc)
		p, _ := doc["produto"].(map[string]any)
		if got, _ := brandOf(p); got == strings.TrimSpace(r.IDMarca) {
			return true, "marca corrigida pelo PUT com " + v.Name, gb
		}
		tried = append(tried, v.Name+" → aceita pela HubSoft, mas a marca continuou a mesma")
	}
	return false, "não foi possível corrigir a marca pela API (tentativas: " + strings.Join(tried, "; ") + ")", last
}

// repairExistingBrand — produto que já existe com este código: se a marca é a pedida não faz nada; senão tenta corrigir.
func repairExistingBrand(ctx context.Context, cfg Config, token string, r StockProductRow, p ExistingStockProduct, marcaNome string, res StockApplyResult) StockApplyResult {
	res.IDProduto = p.ID
	gb, gerr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto/"+p.ID, nil)
	if gerr != "" {
		res.Action, res.Message, res.Detail = StockActionBrandRepairFailed, "não foi possível ler o produto id "+p.ID+": "+gerr, snippet(gb)
		return res
	}
	var doc map[string]any
	_ = json.Unmarshal(gb, &doc)
	prod, _ := doc["produto"].(map[string]any)
	got, gotNome := brandOf(prod)
	if got == strings.TrimSpace(r.IDMarca) {
		res.OK, res.Action = true, StockActionAlreadyExists
		res.Message = fmt.Sprintf("já existe (id %s) e a marca já está correta — nada foi alterado", p.ID)
		return res
	}
	fixed, how, nb := repairStockBrand(ctx, cfg, token, p.ID, r, marcaNome)
	res.BrandRepair = how
	if fixed {
		res.OK, res.Action = true, StockActionBrandRepaired
		res.Message = fmt.Sprintf("marca do produto id %s corrigida de «%s» para «%s»", p.ID, gotNome, marcaNome)
		return res
	}
	res.Action = StockActionBrandRepairFailed
	res.Message = fmt.Sprintf("o produto id %s está com a marca «%s» e %s", p.ID, gotNome, how)
	res.Detail = snippet(nb)
	return res
}
