package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// --- Transferência de patrimônios entre LOCAIS de estoque (permissão integrations.hubsoft_bulk) -----------------------
//
// API: POST /estoque/movimento_estoque/transferencia — «mover produtos de um local de estoque para outro». O corpo leva o local
// de origem, o de destino e, por produto, a lista de patrimônios (id_produto_item). A API NÃO cria local de estoque novo
// (só lista/consulta), então o destino precisa já existir.
//
// Regras de segurança:
//  - só transfere patrimônio com status ESTOQUE (um item em comodato/usuário/vendido continua onde está);
//  - o local de origem de cada item é o local ATUAL dele (relido antes de enviar);
//  - item que já está no destino é resultado normal («já estava»), não erro;
//  - depois de enviar, cada item é relido (local = destino e status ainda ESTOQUE).

// TransferRow — um patrimônio a transferir (só o id_produto_item é necessário; o resto é rótulo para o operador).
type TransferRow struct {
	Linha         int    `json:"linha"`
	IDProdutoItem string `json:"id_produto_item"`
	Rotulo        string `json:"rotulo,omitempty"`
}

// TransferOptions — escolhas do lote.
type TransferOptions struct {
	IDLocalDestino string `json:"id_local_destino"`
	Observacao     string `json:"observacao"`
}

// TransferItemInfo — como o patrimônio está hoje na HubSoft.
type TransferItemInfo struct {
	ID            string `json:"id_produto_item"`
	CodigoItem    string `json:"codigo_item,omitempty"`
	Identificador string `json:"identificador_proprio,omitempty"`
	Produto       string `json:"produto,omitempty"`
	ProdutoID     string `json:"id_produto,omitempty"`
	LocalID       string `json:"id_local_estoque,omitempty"`
	LocalNome     string `json:"local,omitempty"`
	Status        string `json:"status,omitempty"`
	StatusPrefixo string `json:"status_prefixo,omitempty"`
}

// TransferRowPreview — conferência de uma linha (somente leitura).
type TransferRowPreview struct {
	Linha       int               `json:"linha"`
	OK          bool              `json:"ok"`
	AlreadyDone bool              `json:"already_done,omitempty"` // já está no local de destino
	Problems    []string          `json:"problems,omitempty"`
	Item        *TransferItemInfo `json:"item,omitempty"`
}

// TransferRowResult — resultado de uma linha enviada.
type TransferRowResult struct {
	Linha        int               `json:"linha"`
	OK           bool              `json:"ok"`
	Action       string            `json:"action"` // transferred | already_done | blocked | failed | verify_failed | skipped
	Message      string            `json:"message"`
	IDMovimento  string            `json:"id_movimento_estoque,omitempty"`
	Item         *TransferItemInfo `json:"item,omitempty"`
	LocalDepois  string            `json:"local_depois,omitempty"`
	StatusDepois string            `json:"status_depois,omitempty"`
	Detail       string            `json:"detail,omitempty"`
	Problems     []string          `json:"problems,omitempty"`
	LocalOrigem  string            `json:"local_origem,omitempty"`
}

// transferChunk — máximo de patrimônios por chamada de transferência.
const transferChunk = 25

func transferItemInfo(it ExistingStockItem) *TransferItemInfo {
	return &TransferItemInfo{ID: it.ID, CodigoItem: it.CodigoItem, Identificador: it.Identificador, Produto: it.ProdutoNome, ProdutoID: it.ProdutoID, LocalID: it.LocalID, LocalNome: it.LocalNome, Status: it.Status, StatusPrefixo: it.StatusPrefix}
}

// getItemByID lê UM patrimônio (GET produto_item/{id}).
func getItemByID(ctx context.Context, cfg Config, token, id string) (ExistingStockItem, error) {
	body, errMsg := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/produto_item/"+id, nil)
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

// TransferDestination confere se o local de destino existe e devolve o nome.
func TransferDestination(ctx context.Context, cfg Config, token, id string) (string, error) {
	id = strings.TrimSpace(id)
	if !isPosInt(id) {
		return "", fmt.Errorf("escolha o local de destino")
	}
	body, errMsg := stockCall(ctx, cfg, token, "GET", "/api/v1/integracao/estoque/local_estoque/"+id, nil)
	if errMsg != "" {
		if notFoundRe.MatchString(errMsg) {
			return "", fmt.Errorf("o local de estoque %s não existe na HubSoft", id)
		}
		return "", fmt.Errorf("não foi possível conferir o local de destino: %s", errMsg)
	}
	var doc map[string]any
	_ = json.Unmarshal(body, &doc)
	for _, k := range []string{"local_estoque", "local", "locais_estoque"} {
		if m, ok := doc[k].(map[string]any); ok {
			if n := pickStr(m, "descricao", "nome"); n != "" {
				return n, nil
			}
		}
	}
	if n := pickStr(doc, "descricao", "nome"); n != "" {
		return n, nil
	}
	return "local " + id, nil
}

func previewOneTransfer(ctx context.Context, cfg Config, token string, r TransferRow, destino string) TransferRowPreview {
	pv := TransferRowPreview{Linha: r.Linha}
	id := strings.TrimSpace(r.IDProdutoItem)
	if !isPosInt(id) {
		pv.Problems = []string{"id_produto_item vazio ou não numérico"}
		return pv
	}
	it, err := getItemByID(ctx, cfg, token, id)
	if err != nil {
		pv.Problems = []string{err.Error()}
		return pv
	}
	pv.Item = transferItemInfo(it)
	switch {
	case it.LocalID == destino:
		pv.OK, pv.AlreadyDone = true, true
	case !strings.EqualFold(it.StatusPrefix, "estoque"):
		pv.Problems = []string{fmt.Sprintf("o patrimônio está com status «%s» — só patrimônio em ESTOQUE é transferido", it.Status)}
	case it.LocalID == "":
		pv.Problems = []string{"a HubSoft não informou o local atual do patrimônio"}
	default:
		pv.OK = true
	}
	return pv
}

// PreviewTransferRows confere as linhas (somente leitura), com até 4 consultas em paralelo.
func PreviewTransferRows(ctx context.Context, cfg Config, token string, rows []TransferRow, opt TransferOptions) (dest string, out []TransferRowPreview) {
	out = make([]TransferRowPreview, len(rows))
	dest, derr := TransferDestination(ctx, cfg, token, opt.IDLocalDestino)
	if derr != nil {
		for i, r := range rows {
			out[i] = TransferRowPreview{Linha: r.Linha, Problems: []string{derr.Error()}}
		}
		return "", out
	}
	destino := strings.TrimSpace(opt.IDLocalDestino)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = previewOneTransfer(ctx, cfg, token, rows[i], destino)
		}(i)
	}
	wg.Wait()
	// o mesmo patrimônio duas vezes na mesma chamada faz a HubSoft recusar o grupo inteiro («quantidade não corresponde»)
	seen := map[string]int{}
	for i := range out {
		if !out[i].OK || out[i].AlreadyDone || out[i].Item == nil {
			continue
		}
		if first, dup := seen[out[i].Item.ID]; dup {
			out[i].OK = false
			out[i].Problems = append(out[i].Problems, fmt.Sprintf("o patrimônio %s aparece repetido neste lote (linha %d) — só a primeira ocorrência é enviada", out[i].Item.CodigoItem, first))
			continue
		}
		seen[out[i].Item.ID] = out[i].Linha
	}
	return dest, out
}

type transferGroupKey struct{ origem, produto string }

// ApplyTransferRows confere de novo, transfere (agrupando por local de origem + produto) e relê cada patrimônio.
// Para depois de 3 grupos seguidos recusados pela HubSoft; os demais voltam como «skipped».
func ApplyTransferRows(ctx context.Context, cfg Config, token string, rows []TransferRow, opt TransferOptions) (results []TransferRowResult, halted bool) {
	dest, pvs := PreviewTransferRows(ctx, cfg, token, rows, opt)
	destino := strings.TrimSpace(opt.IDLocalDestino)
	results = make([]TransferRowResult, len(rows))
	groups := map[transferGroupKey][]int{}
	for i, pv := range pvs {
		res := TransferRowResult{Linha: pv.Linha, Item: pv.Item}
		switch {
		case !pv.OK:
			res.Action, res.Message, res.Problems = "blocked", "nada foi enviado: "+strings.Join(pv.Problems, " | "), pv.Problems
		case pv.AlreadyDone:
			res.OK, res.Action = true, "already_done"
			res.Message = fmt.Sprintf("o patrimônio %s já está no local «%s» — nada foi enviado", pv.Item.CodigoItem, dest)
			res.LocalDepois, res.StatusDepois = pv.Item.LocalNome, pv.Item.Status
		default:
			k := transferGroupKey{pv.Item.LocalID, pv.Item.ProdutoID}
			groups[k] = append(groups[k], i)
			res.LocalOrigem = pv.Item.LocalNome
		}
		results[i] = res
	}
	keys := make([]transferGroupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].origem != keys[b].origem {
			return keys[a].origem < keys[b].origem
		}
		return keys[a].produto < keys[b].produto
	})
	fails := 0
	for _, k := range keys {
		idxs := groups[k]
		for s := 0; s < len(idxs); s += transferChunk {
			e := s + transferChunk
			if e > len(idxs) {
				e = len(idxs)
			}
			part := idxs[s:e]
			if halted {
				for _, i := range part {
					results[i].Action, results[i].Message = "skipped", "não enviado: três grupos seguidos foram recusados pela HubSoft"
				}
				continue
			}
			pats := make([]any, 0, len(part))
			for _, i := range part {
				pats = append(pats, map[string]any{"id_produto_item": atoiSafe(pvs[i].Item.ID)})
			}
			body := map[string]any{
				"id_local_estoque_origem": atoiSafe(k.origem), "id_local_estoque_destino": atoiSafe(destino),
				"produtos": []any{map[string]any{"quantidade": len(part), "produto": map[string]any{"id_produto": atoiSafe(k.produto)}, "campo_identificacao_patrimonio": "id_produto_item", "patrimonios": pats}},
			}
			if o := strings.TrimSpace(opt.Observacao); o != "" {
				body["observacao"] = o
			}
			rb, errMsg := stockCall(ctx, cfg, token, "POST", "/api/v1/integracao/estoque/movimento_estoque/transferencia", body)
			if errMsg != "" {
				fails++
				for _, i := range part {
					results[i].Action, results[i].Message, results[i].Detail = "failed", "a HubSoft recusou a transferência: "+errMsg, snippet(rb)
				}
				if fails >= 3 {
					halted = true
				}
				continue
			}
			fails = 0
			mov := transferMovementID(rb)
			for _, i := range part {
				results[i].IDMovimento = mov
				after, rerr := getItemByID(ctx, cfg, token, pvs[i].Item.ID)
				switch {
				case rerr != nil:
					results[i].Action, results[i].Message = "verify_failed", "a transferência foi aceita (movimento "+mov+"), mas não foi possível reler o patrimônio: "+rerr.Error()
				case after.LocalID != destino:
					results[i].LocalDepois, results[i].StatusDepois = after.LocalNome, after.Status
					results[i].Action, results[i].Message = "verify_failed", fmt.Sprintf("a transferência foi aceita (movimento %s), mas o patrimônio continua no local «%s» (id %s) em vez de «%s»", mov, after.LocalNome, after.LocalID, dest)
				case !strings.EqualFold(after.StatusPrefix, "estoque"):
					results[i].LocalDepois, results[i].StatusDepois = after.LocalNome, after.Status
					results[i].Action, results[i].Message = "verify_failed", fmt.Sprintf("o patrimônio mudou de local, mas o status ficou «%s» em vez de ESTOQUE", after.Status)
				default:
					results[i].OK, results[i].Action = true, "transferred"
					results[i].LocalDepois, results[i].StatusDepois = after.LocalNome, after.Status
					results[i].Message = fmt.Sprintf("patrimônio %s transferido de «%s» para «%s» (movimento %s)", after.CodigoItem, results[i].LocalOrigem, after.LocalNome, mov)
				}
			}
		}
	}
	return results, halted
}

// transferMovementID lê o id do movimento de SAÍDA (e o de entrada, se vier) da resposta da transferência.
func transferMovementID(rb []byte) string {
	var doc struct {
		Mov struct {
			Saida struct {
				ID json.Number `json:"id_movimento_estoque"`
			} `json:"saida"`
			Entrada struct {
				ID json.Number `json:"id_movimento_estoque"`
			} `json:"entrada"`
		} `json:"movimentos_estoque"`
	}
	_ = json.Unmarshal(rb, &doc)
	switch {
	case doc.Mov.Saida.ID.String() != "" && doc.Mov.Entrada.ID.String() != "":
		return doc.Mov.Saida.ID.String() + "/" + doc.Mov.Entrada.ID.String()
	case doc.Mov.Saida.ID.String() != "":
		return doc.Mov.Saida.ID.String()
	}
	return doc.Mov.Entrada.ID.String()
}
