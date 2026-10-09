package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// --- Comodato de UM patrimônio para o serviço de um cliente (permissão integrations.hubsoft_bulk) ---------------------
//
// Passo 3 da migração do IXC. Na HubSoft o comodato é uma SAÍDA DE ESTOQUE para o serviço do cliente
// (POST /estoque/movimento_estoque/saida/cliente_servico): o patrimônio sai do local de estoque e passa a ficar vinculado ao
// serviço, com o status que o «tipo de movimento» escolhido define (Comodato, Vendido…). Por isso:
//  - o tipo de movimento é decisivo: usar um tipo com status «Vendido» marcaria o equipamento como VENDIDO. O NetQuasar só
//    aceita tipos cujo status é «Comodato» (descobertos pelos movimentos de saída para clientes que já existem na HubSoft);
//  - antes de enviar tudo é conferido (patrimônio existe e está em ESTOQUE, serviço existe, produto com controle patrimonial)
//    e, depois, relido (status Comodato + serviço certo + presente nos vínculos do serviço).

// MovementType — tipo de movimento de estoque visto em saídas para serviços de clientes.
type MovementType struct {
	ID            string `json:"id"`
	Nome          string `json:"nome"`
	Prefixo       string `json:"prefixo"`
	StatusPrefixo string `json:"status_prefixo"` // status que o patrimônio recebe (ex.: comodato, vendido)
	StatusNome    string `json:"status_nome"`
	Usos          int    `json:"usos"`
}

// IsComodato — o tipo deixa o patrimônio com status «Comodato».
func (m MovementType) IsComodato() bool { return strings.EqualFold(m.StatusPrefixo, "comodato") }

// DiscoverMovementTypes lê os movimentos de estoque dos últimos 2 anos (saídas para serviço de cliente) e reúne os tipos usados.
// Não existe catálogo de «tipo de movimento» na API; este é o único jeito de descobrir os ids reais da conta.
func DiscoverMovementTypes(ctx context.Context, cfg Config, token string) ([]MovementType, error) {
	now := time.Now()
	params := map[string]string{
		"data_inicio": now.AddDate(-2, 0, 0).Format("2006-01-02"), "data_fim": now.Format("2006-01-02"),
		"tipo_data": "cadastro", "tipo_vinculo_destino": "servico_cliente",
	}
	items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/estoque/movimento_estoque", params, 60, "movimentos_estoque")
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler os movimentos de estoque: %w", err)
	}
	by := map[string]*MovementType{}
	for _, m := range items {
		// só saídas para serviço de cliente (o filtro da API pode ser ignorado — confere de novo)
		if vd, ok := m["vinculo_destino"].(map[string]any); ok && pickStr(vd, "tipo_vinculo") != "" && pickStr(vd, "tipo_vinculo") != "servico_cliente" {
			continue
		}
		tm, ok := m["tipo_movimento_estoque"].(map[string]any)
		if !ok {
			continue
		}
		id := pickStr(tm, "id_tipo_movimento_estoque")
		if id == "" {
			continue
		}
		mt := by[id]
		if mt == nil {
			mt = &MovementType{ID: id, Nome: pickStr(tm, "nome"), Prefixo: pickStr(tm, "prefixo")}
			if st, ok := tm["produto_item_status"].(map[string]any); ok {
				mt.StatusPrefixo, mt.StatusNome = pickStr(st, "prefixo"), pickStr(st, "descricao")
			}
			by[id] = mt
		}
		mt.Usos++
	}
	out := make([]MovementType, 0, len(by))
	for _, v := range by {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsComodato() != out[j].IsComodato() {
			return out[i].IsComodato()
		}
		return out[i].Usos > out[j].Usos
	})
	return out, nil
}

// ServiceInfo — serviço de cliente na HubSoft.
type ServiceInfo struct {
	IDCliente        string `json:"id_cliente"`
	Cliente          string `json:"cliente"`
	CodigoCliente    string `json:"codigo_cliente,omitempty"`
	IDClienteServico string `json:"id_cliente_servico"`
	Login            string `json:"login,omitempty"`
	Plano            string `json:"plano,omitempty"`
	Status           string `json:"status,omitempty"`
}

// FindServices procura serviços por id_cliente_servico (número) ou por login PPPoE (texto), incluindo cancelados.
func FindServices(ctx context.Context, cfg Config, token, termo string) ([]ServiceInfo, error) {
	termo = strings.TrimSpace(termo)
	if termo == "" {
		return nil, fmt.Errorf("informe o id do serviço ou o login")
	}
	busca := "login_radius"
	if isPosInt(termo) {
		busca = "id_cliente_servico"
	}
	body, errMsg := stockGetQuery(ctx, cfg, token, "/api/v1/integracao/cliente", map[string]string{
		"busca": busca, "termo_busca": termo, "limit": "10", "cancelado": "todos", "inativo": "todos",
	})
	if errMsg != "" {
		if notFoundRe.MatchString(errMsg) {
			return nil, nil
		}
		return nil, fmt.Errorf("consulta de cliente falhou: %s", errMsg)
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, fmt.Errorf("resposta inválida da HubSoft ao procurar o serviço")
	}
	var out []ServiceInfo
	for _, it := range extractArray(doc, "clientes") {
		c, ok := it.(map[string]any)
		if !ok {
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
			// com busca por id, só o serviço pedido; com login, só o(s) serviço(s) com esse login
			if busca == "id_cliente_servico" && si.IDClienteServico != termo {
				continue
			}
			if busca == "login_radius" && !strings.EqualFold(si.Login, termo) {
				continue
			}
			out = append(out, si)
		}
	}
	return out, nil
}

// ServiceLink — um patrimônio já vinculado a um serviço.
type ServiceLink struct {
	IDProduto     string `json:"id_produto"`
	IDProdutoItem string `json:"id_produto_item"`
	Status        string `json:"status"`
	Identificador string `json:"identificador_proprio,omitempty"`
	Serie         string `json:"numero_serie,omitempty"`
	MAC           string `json:"mac_address,omitempty"`
	CodigoItem    string `json:"codigo_item,omitempty"`
	Quantidade    string `json:"quantidade,omitempty"`
}

// ListServiceLinks lista os produtos/patrimônios já vinculados a um serviço (GET produto_vinculo/cliente_servico/{id}).
func ListServiceLinks(ctx context.Context, cfg Config, token, idServico string) ([]ServiceLink, error) {
	items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/estoque/produto_vinculo/cliente_servico/"+strings.TrimSpace(idServico), nil, 20, "produtos_cliente_servico")
	if err != nil {
		if notFoundRe.MatchString(err.Error()) {
			return nil, nil
		}
		return nil, fmt.Errorf("não foi possível ler os produtos vinculados ao serviço %s: %w", idServico, err)
	}
	var out []ServiceLink
	for _, m := range items {
		pats := extractArray(m, "patrimonios")
		if len(pats) == 0 {
			out = append(out, ServiceLink{IDProduto: pickStr(m, "id_produto"), Quantidade: pickStr(m, "quantidade"), Status: "(sem patrimônio — quantidade)"})
			continue
		}
		for _, p := range pats {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			l := ServiceLink{IDProduto: pickStr(m, "id_produto"), IDProdutoItem: pickStr(pm, "id_produto_item"), Identificador: pickStr(pm, "identificador_proprio"), Serie: pickStr(pm, "numero_serie"), MAC: pickStr(pm, "mac_address"), CodigoItem: pickStr(pm, "codigo_item")}
			if st, ok := pm["produto_item_status"].(map[string]any); ok {
				l.Status = pickStr(st, "descricao")
			}
			out = append(out, l)
		}
	}
	return out, nil
}

// patrimônio: campos aceitos para localizar o patrimônio (os da API de consulta + o id).
var comodatoItemFields = map[string]bool{"id_produto_item": true, "identificador_proprio": true, "numero_serie": true, "mac_address": true, "codigo_item": true}

// FindItem localiza UM patrimônio por id_produto_item, identificador_proprio, numero_serie, mac_address ou codigo_item. Mais de um
// resultado (comparação exata) é erro: nunca escolhe «o primeiro».
func FindItem(ctx context.Context, cfg Config, token, campo, valor string) (ExistingStockItem, error) {
	campo, valor = strings.TrimSpace(campo), strings.TrimSpace(valor)
	if !comodatoItemFields[campo] {
		return ExistingStockItem{}, fmt.Errorf("campo de busca inválido: %q", campo)
	}
	if valor == "" {
		return ExistingStockItem{}, fmt.Errorf("informe o valor a procurar")
	}
	if campo == "id_produto_item" {
		if !isPosInt(valor) {
			return ExistingStockItem{}, fmt.Errorf("id_produto_item precisa ser um número")
		}
		body, errMsg := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto_item/"+valor, nil)
		if errMsg != "" {
			if notFoundRe.MatchString(errMsg) {
				return ExistingStockItem{}, fmt.Errorf("nenhum patrimônio com este id_produto_item")
			}
			return ExistingStockItem{}, fmt.Errorf("consulta falhou: %s", errMsg)
		}
		var doc map[string]any
		_ = json.Unmarshal(body, &doc)
		if m, ok := doc["produto"].(map[string]any); ok {
			return itemFromMap(m), nil
		}
		return ExistingStockItem{}, fmt.Errorf("nenhum patrimônio com este id_produto_item")
	}
	list, err := lookupStockItems(ctx, cfg, token, campo, valor)
	if err != nil {
		return ExistingStockItem{}, err
	}
	var exact []ExistingStockItem
	for _, it := range list {
		var got string
		switch campo {
		case "identificador_proprio":
			got = it.Identificador
		case "numero_serie":
			got = it.Serie
		case "mac_address":
			if macKey(it.MAC) == macKey(valor) {
				exact = append(exact, it)
			}
			continue
		case "codigo_item":
			got = it.CodigoItem
		}
		if strings.EqualFold(strings.TrimSpace(got), valor) {
			exact = append(exact, it)
		}
	}
	switch len(exact) {
	case 0:
		return ExistingStockItem{}, fmt.Errorf("nenhum patrimônio com %s «%s»", campo, valor)
	case 1:
		return exact[0], nil
	}
	var ds []string
	for _, e := range exact {
		ds = append(ds, e.Describe())
	}
	return ExistingStockItem{}, fmt.Errorf("mais de um patrimônio com %s «%s» — use o id_produto_item: %s", campo, valor, strings.Join(ds, " | "))
}

// ComodatoRequest — um patrimônio → um serviço.
type ComodatoRequest struct {
	IDClienteServico string `json:"id_cliente_servico"`
	Campo            string `json:"campo"` // como o patrimônio é localizado
	Valor            string `json:"valor"`
	// IDLocalEstoque — local de onde o patrimônio sai; vazio = o local onde ele está agora.
	IDLocalEstoque  string `json:"id_local_estoque"`
	IDTipoMovimento string `json:"id_tipo_movimento_estoque"`
	Observacao      string `json:"observacao"`
	// TipoConfirmadoComodato — o operador confirmou que o tipo informado gera status Comodato (usado quando o tipo não foi
	// descoberto pelos movimentos existentes).
	TipoConfirmadoComodato bool `json:"tipo_confirmado_comodato"`
}

// ComodatoPreview — o que o comodato VAI fazer (somente leitura).
type ComodatoPreview struct {
	OK bool `json:"ok"`
	// AlreadyDone — o patrimônio JÁ está como comodato NESTE serviço: nada a enviar (resultado normal, não erro).
	AlreadyDone bool               `json:"already_done,omitempty"`
	Problems    []string           `json:"problems,omitempty"` // bloqueiam
	Warnings    []string           `json:"warnings,omitempty"`
	Service     *ServiceInfo       `json:"service,omitempty"`
	Item        *ExistingStockItem `json:"item,omitempty"`
	Produto     string             `json:"produto,omitempty"`
	Links       []ServiceLink      `json:"links,omitempty"` // o que o serviço já tem vinculado
	Tipo        *MovementType      `json:"tipo,omitempty"`
	LocalID     string             `json:"id_local_estoque,omitempty"`
}

// PreviewComodato confere tudo e diz se pode seguir. `types` são os tipos descobertos (pode ser nil).
func PreviewComodato(ctx context.Context, cfg Config, token string, req ComodatoRequest, types []MovementType) ComodatoPreview {
	pv := ComodatoPreview{}
	bad := func(f string, a ...any) { pv.Problems = append(pv.Problems, fmt.Sprintf(f, a...)) }

	if !isPosInt(strings.TrimSpace(req.IDClienteServico)) {
		bad("informe o id do serviço do cliente (id_cliente_servico)")
	}
	if !isPosInt(strings.TrimSpace(req.IDTipoMovimento)) {
		bad("escolha o tipo de movimento")
	}
	if len(pv.Problems) > 0 {
		return pv
	}

	// tipo de movimento
	for i := range types {
		if types[i].ID == strings.TrimSpace(req.IDTipoMovimento) {
			t := types[i]
			pv.Tipo = &t
		}
	}
	switch {
	case pv.Tipo != nil && !pv.Tipo.IsComodato():
		bad("o tipo de movimento «%s» (id %s) deixa o patrimônio com status «%s», não «Comodato» — recusado para não marcar o equipamento errado", pv.Tipo.Nome, pv.Tipo.ID, pv.Tipo.StatusNome)
	case pv.Tipo == nil && !req.TipoConfirmadoComodato:
		bad("o tipo de movimento %s não foi visto em nenhum movimento existente, então não dá para saber o status que ele gera — confirme que ele gera «Comodato» para liberar", req.IDTipoMovimento)
	case pv.Tipo == nil:
		pv.Warnings = append(pv.Warnings, "tipo de movimento informado manualmente: o status do patrimônio será conferido DEPOIS e, se não for Comodato, o resultado avisa (reversível com «retorno ao estoque»)")
	}

	// serviço
	svcs, err := FindServices(ctx, cfg, token, req.IDClienteServico)
	switch {
	case err != nil:
		bad("não foi possível conferir o serviço: %v", err)
	case len(svcs) == 0:
		bad("o serviço %s não existe na HubSoft", strings.TrimSpace(req.IDClienteServico))
	default:
		pv.Service = &svcs[0]
		if st := normText(svcs[0].Status); strings.Contains(st, "cancel") {
			bad("o serviço %s está «%s» — não é recomendável ligar um equipamento a um serviço cancelado", svcs[0].IDClienteServico, svcs[0].Status)
		}
	}

	// patrimônio
	item, ierr := FindItem(ctx, cfg, token, req.Campo, req.Valor)
	if ierr != nil {
		bad("patrimônio: %v", ierr)
		return finishPreview(pv)
	}
	pv.Item = &item

	// o que o serviço já tem vinculado (a lista de vínculos do serviço é a fonte da verdade)
	linkedHere := false
	if pv.Service != nil {
		if links, lerr := ListServiceLinks(ctx, cfg, token, pv.Service.IDClienteServico); lerr == nil {
			pv.Links = links
			for _, l := range links {
				linkedHere = linkedHere || l.IDProdutoItem == item.ID
			}
		} else {
			pv.Warnings = append(pv.Warnings, lerr.Error())
		}
	}
	if linkedHere {
		// resultado normal (não é erro): nada a enviar
		pv.AlreadyDone = true
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("este patrimônio já está vinculado a este serviço (status «%s») — nada será enviado", item.Status))
		return finishPreview(pv)
	}
	if item.StatusPrefix != "estoque" {
		where := ""
		if item.Cliente != "" {
			where = " (cliente " + item.Cliente + ")"
		}
		bad("o patrimônio está com status «%s»%s — só um patrimônio em ESTOQUE pode ser ligado a um serviço", item.Status, where)
	}
	if l := strings.TrimSpace(req.IDLocalEstoque); l != "" && l != item.LocalID {
		bad("o patrimônio está no local «%s» (id %s), não no id %s informado", item.LocalNome, item.LocalID, l)
	}
	pv.LocalID = item.LocalID

	// produto com controle patrimonial
	if item.ProdutoID != "" {
		if pb, perr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto/"+item.ProdutoID, nil); perr == "" {
			var pd map[string]any
			_ = json.Unmarshal(pb, &pd)
			if pm, ok := pd["produto"].(map[string]any); ok {
				pv.Produto = pickStr(pm, "nome")
				if !pickBool(pm, "controle_patrimonial") {
					bad("o produto «%s» não tem controle patrimonial", pv.Produto)
				}
			}
		} else {
			pv.Warnings = append(pv.Warnings, "não foi possível ler o produto do patrimônio: "+perr)
		}
	}

	// aviso: não duplicar o equipamento do cliente
	if len(pv.Links) > 0 {
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("o serviço já tem %d produto(s)/patrimônio(s) vinculado(s) — confira a lista abaixo para não duplicar o equipamento do cliente", len(pv.Links)))
	}
	return finishPreview(pv)
}

func finishPreview(pv ComodatoPreview) ComodatoPreview {
	pv.OK = len(pv.Problems) == 0
	return pv
}

// ComodatoResult — resultado da saída para o serviço.
type ComodatoResult struct {
	OK            bool            `json:"ok"`
	Action        string          `json:"action"` // created | already_done | blocked | failed | verify_failed
	Message       string          `json:"message"`
	IDMovimento   string          `json:"id_movimento_estoque,omitempty"`
	IDProdutoItem string          `json:"id_produto_item,omitempty"`
	StatusDepois  string          `json:"status_depois,omitempty"`
	Verified      *bool           `json:"verified,omitempty"`
	VerifyMessage string          `json:"verify_message,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Preview       ComodatoPreview `json:"preview"`
}

// ApplyComodato refaz TODAS as conferências, envia a saída para o serviço e relê o patrimônio.
func ApplyComodato(ctx context.Context, cfg Config, token string, req ComodatoRequest, types []MovementType) ComodatoResult {
	return applyPrepared(ctx, cfg, token, req, PreviewComodato(ctx, cfg, token, req, types))
}

// applyPrepared envia a saída a partir de uma pré-visualização JÁ feita (sem refazer as consultas) e confere o resultado.
func applyPrepared(ctx context.Context, cfg Config, token string, req ComodatoRequest, pv ComodatoPreview) ComodatoResult {
	res := ComodatoResult{Preview: pv}
	if pv.AlreadyDone && pv.Item != nil && pv.Service != nil {
		res.OK, res.Action, res.IDProdutoItem = true, "already_done", pv.Item.ID
		res.Message = fmt.Sprintf("o patrimônio %s já está como comodato do serviço %s (%s — %s) — nada foi enviado", pv.Item.CodigoItem, pv.Service.IDClienteServico, pv.Service.Cliente, pv.Service.Login)
		return res
	}
	if !pv.OK || pv.Item == nil || pv.Service == nil {
		res.Action, res.Message = "blocked", "nada foi enviado — a conferência encontrou problema(s): "+strings.Join(pv.Problems, " | ")
		return res
	}
	res.IDProdutoItem = pv.Item.ID
	obs := strings.TrimSpace(req.Observacao)
	body := map[string]any{
		"id_cliente_servico": atoiSafe(pv.Service.IDClienteServico), "id_tipo_movimento_estoque": atoiSafe(req.IDTipoMovimento), "id_local_estoque": atoiSafe(pv.LocalID),
		"produtos": []any{map[string]any{
			"id_produto": atoiSafe(pv.Item.ProdutoID), "quantidade": 1, "campo_identificacao_patrimonio": "id_produto_item",
			"patrimonios": []any{map[string]any{"id_produto_item": atoiSafe(pv.Item.ID)}},
		}},
	}
	if obs != "" {
		body["observacao"] = obs
	}
	rb, errMsg := stockCall(ctx, cfg, token, "POST", "/api/v1/integracao/estoque/movimento_estoque/saida/cliente_servico", body)
	if errMsg != "" {
		res.Action, res.Message, res.Detail = "failed", "a HubSoft recusou o comodato: "+errMsg, snippet(rb)
		return res
	}
	var mv struct {
		Mov struct {
			ID json.Number `json:"id_movimento_estoque"`
		} `json:"movimento_estoque"`
	}
	_ = json.Unmarshal(rb, &mv)
	res.IDMovimento = mv.Mov.ID.String()

	// conferir: status Comodato + serviço certo + presente nos vínculos do serviço
	vb, verr := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto_item/"+pv.Item.ID, nil)
	var diffs []string
	if verr != "" {
		diffs = append(diffs, "não foi possível reler o patrimônio: "+verr)
	} else {
		var doc map[string]any
		_ = json.Unmarshal(vb, &doc)
		if m, ok := doc["produto"].(map[string]any); ok {
			it := itemFromMap(m)
			res.StatusDepois = it.Status
			if !strings.EqualFold(it.StatusPrefix, "comodato") {
				diffs = append(diffs, fmt.Sprintf("status «%s» em vez de «Comodato» — o tipo de movimento %s NÃO gera comodato; reverta com «retorno ao estoque»", it.Status, req.IDTipoMovimento))
			}
			if cs, ok := m["cliente_servico"].(map[string]any); ok && pickStr(cs, "id_cliente_servico") != "" && pickStr(cs, "id_cliente_servico") != pv.Service.IDClienteServico {
				diffs = append(diffs, fmt.Sprintf("vinculado ao serviço %s em vez de %s", pickStr(cs, "id_cliente_servico"), pv.Service.IDClienteServico))
			}
		}
	}
	if links, lerr := ListServiceLinks(ctx, cfg, token, pv.Service.IDClienteServico); lerr == nil {
		found := false
		for _, l := range links {
			found = found || l.IDProdutoItem == pv.Item.ID
		}
		if !found {
			diffs = append(diffs, "o patrimônio não aparece nos produtos vinculados ao serviço")
		}
	}
	ok := len(diffs) == 0
	res.Verified, res.VerifyMessage, res.OK = &ok, "status Comodato, serviço e vínculo conferidos", ok
	if ok {
		res.Action = "created"
		res.Message = fmt.Sprintf("patrimônio %s ligado como COMODATO ao serviço %s (%s — %s), movimento %s", pv.Item.CodigoItem, pv.Service.IDClienteServico, pv.Service.Cliente, pv.Service.Login, res.IDMovimento)
	} else {
		res.Action, res.VerifyMessage = "verify_failed", strings.Join(diffs, "; ")
		res.Message = "a saída foi aceita (movimento " + res.IDMovimento + "), mas a conferência encontrou diferença: " + res.VerifyMessage
		res.Detail = snippet(vb)
	}
	return res
}

// --- Lote: várias linhas «patrimônio → serviço» (CSV) -------------------------------------------------------------------

// ComodatoRow — uma linha do arquivo de importação. O patrimônio pode ser localizado por vários campos; a ordem de
// prioridade é id_produto_item > identificador_proprio > numero_serie > mac_address > codigo_item. Se mais de um campo vier
// preenchido, TODOS precisam apontar para o MESMO patrimônio (senão a linha é recusada — nunca «escolhe um»).
type ComodatoRow struct {
	Linha                int    `json:"linha"`
	IDClienteServico     string `json:"id_cliente_servico"`
	IDProdutoItem        string `json:"id_produto_item,omitempty"`
	IdentificadorProprio string `json:"identificador_proprio,omitempty"`
	NumeroSerie          string `json:"numero_serie,omitempty"`
	MacAddress           string `json:"mac_address,omitempty"`
	CodigoItem           string `json:"codigo_item,omitempty"`
	IDProduto            string `json:"id_produto,omitempty"` // opcional: confere se o patrimônio é deste produto
	Cliente              string `json:"cliente,omitempty"`    // opcional: confere o nome do cliente do serviço
	Login                string `json:"login,omitempty"`      // opcional: confere o login do serviço
	Observacao           string `json:"observacao,omitempty"`
}

// ComodatoBatchOptions — escolhas válidas para o lote inteiro.
type ComodatoBatchOptions struct {
	IDTipoMovimento        string `json:"id_tipo_movimento_estoque"`
	TipoConfirmadoComodato bool   `json:"tipo_confirmado_comodato"`
}

// ComodatoRowPreview — conferência de uma linha.
type ComodatoRowPreview struct {
	Linha   int             `json:"linha"`
	Locator string          `json:"locator,omitempty"` // campo usado para achar o patrimônio
	Preview ComodatoPreview `json:"preview"`
}

// ComodatoRowResult — resultado de uma linha enviada.
type ComodatoRowResult struct {
	Linha   int            `json:"linha"`
	Locator string         `json:"locator,omitempty"`
	Result  ComodatoResult `json:"result"`
}

func (r ComodatoRow) locators() [][2]string {
	return [][2]string{
		{"id_produto_item", r.IDProdutoItem}, {"identificador_proprio", r.IdentificadorProprio}, {"numero_serie", r.NumeroSerie},
		{"mac_address", r.MacAddress}, {"codigo_item", r.CodigoItem},
	}
}

func hasLocatorBesidesMAC(r ComodatoRow) bool {
	for _, l := range r.locators() {
		if l[0] != "mac_address" && strings.TrimSpace(l[1]) != "" {
			return true
		}
	}
	return false
}

// resolveRowItem acha o patrimônio da linha e confere a consistência entre os campos informados.
func resolveRowItem(ctx context.Context, cfg Config, token string, row ComodatoRow) (it ExistingStockItem, locator string, err error) {
	var found []ExistingStockItem
	var errs []string
	for _, l := range row.locators() {
		v := strings.TrimSpace(l[1])
		if v == "" {
			continue
		}
		got, ferr := FindItem(ctx, cfg, token, l[0], v)
		if ferr != nil {
			// o MAC pode ter sido omitido no cadastro (a HubSoft recusa alguns formatos): não achar por MAC não é divergência
			if l[0] != "mac_address" || len(found) == 0 && !hasLocatorBesidesMAC(row) {
				errs = append(errs, fmt.Sprintf("%s «%s»: %v", l[0], v, ferr))
			}
			continue
		}
		if locator == "" {
			locator = l[0]
		}
		dup := false
		for _, f := range found {
			dup = dup || f.ID == got.ID
		}
		if !dup {
			found = append(found, got)
		}
	}
	switch {
	case len(found) == 0 && len(errs) == 0:
		return it, "", fmt.Errorf("a linha não tem nenhum campo para localizar o patrimônio (id_produto_item, identificador_proprio, numero_serie, mac_address ou codigo_item)")
	case len(found) == 0:
		return it, "", fmt.Errorf("patrimônio não encontrado — %s", strings.Join(errs, " | "))
	case len(errs) > 0:
		return it, "", fmt.Errorf("os campos da linha não concordam: o patrimônio %s foi achado por %s, mas %s", found[0].ID, locator, strings.Join(errs, " | "))
	case len(found) > 1:
		var ds []string
		for _, f := range found {
			ds = append(ds, f.Describe())
		}
		return it, "", fmt.Errorf("os campos da linha apontam para patrimônios DIFERENTES — %s", strings.Join(ds, " | "))
	}
	it = found[0]
	// campos informados que não bateram com o patrimônio escolhido viram aviso no chamador; aqui só o produto é bloqueante
	if p := strings.TrimSpace(row.IDProduto); p != "" && p != it.ProdutoID {
		return it, locator, fmt.Errorf("o patrimônio %s é do produto %s («%s»), mas a linha diz produto %s", it.ID, it.ProdutoID, it.ProdutoNome, p)
	}
	return it, locator, nil
}

// rowPreview resolve o patrimônio e roda a conferência completa.
func rowPreview(ctx context.Context, cfg Config, token string, row ComodatoRow, opt ComodatoBatchOptions, types []MovementType) (string, ComodatoPreview) {
	it, loc, err := resolveRowItem(ctx, cfg, token, row)
	if err != nil {
		pv := ComodatoPreview{Problems: []string{err.Error()}}
		if it.ID != "" {
			pv.Item = &it
		}
		return loc, pv
	}
	pv := PreviewComodato(ctx, cfg, token, ComodatoRequest{
		IDClienteServico: row.IDClienteServico, Campo: "id_produto_item", Valor: it.ID,
		IDTipoMovimento: opt.IDTipoMovimento, Observacao: row.Observacao, TipoConfirmadoComodato: opt.TipoConfirmadoComodato,
	}, types)
	// conferências opcionais de cliente/login do serviço (evitam ligar ao serviço errado por um id digitado errado)
	if pv.Service != nil {
		if c := normText(row.Cliente); c != "" && normText(pv.Service.Cliente) != c {
			pv.Problems = append(pv.Problems, fmt.Sprintf("o serviço %s é do cliente «%s», mas a linha diz «%s»", pv.Service.IDClienteServico, pv.Service.Cliente, row.Cliente))
		}
		if lg := strings.TrimSpace(row.Login); lg != "" && !strings.EqualFold(pv.Service.Login, lg) {
			pv.Problems = append(pv.Problems, fmt.Sprintf("o serviço %s tem o login «%s», mas a linha diz «%s»", pv.Service.IDClienteServico, pv.Service.Login, lg))
		}
	}
	return loc, finishPreview(pv)
}

// PreviewComodatoRows confere as linhas (somente leitura), uma de cada vez.
func PreviewComodatoRows(ctx context.Context, cfg Config, token string, rows []ComodatoRow, opt ComodatoBatchOptions, types []MovementType) []ComodatoRowPreview {
	out := make([]ComodatoRowPreview, 0, len(rows))
	for _, r := range rows {
		loc, pv := rowPreview(ctx, cfg, token, r, opt, types)
		out = append(out, ComodatoRowPreview{Linha: r.Linha, Locator: loc, Preview: pv})
	}
	return out
}

// ApplyComodatoRows confere e envia as linhas, uma de cada vez. Para na primeira linha com falha do HubSoft
// (failed/verify_failed) para não repetir o mesmo erro — as demais voltam como «skipped».
func ApplyComodatoRows(ctx context.Context, cfg Config, token string, rows []ComodatoRow, opt ComodatoBatchOptions, types []MovementType) []ComodatoRowResult {
	out := make([]ComodatoRowResult, 0, len(rows))
	stop := false
	for _, r := range rows {
		if stop {
			out = append(out, ComodatoRowResult{Linha: r.Linha, Result: ComodatoResult{Action: "skipped", Message: "não enviada: uma linha anterior do mesmo grupo falhou na HubSoft"}})
			continue
		}
		loc, pv := rowPreview(ctx, cfg, token, r, opt, types)
		res := applyPrepared(ctx, cfg, token, ComodatoRequest{
			IDClienteServico: r.IDClienteServico, IDTipoMovimento: opt.IDTipoMovimento, Observacao: r.Observacao, TipoConfirmadoComodato: opt.TipoConfirmadoComodato,
		}, pv)
		out = append(out, ComodatoRowResult{Linha: r.Linha, Locator: loc, Result: res})
		if res.Action == "failed" || res.Action == "verify_failed" {
			stop = true
		}
	}
	return out
}
