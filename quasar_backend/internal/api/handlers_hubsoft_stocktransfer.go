package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// Transferência de patrimônios entre locais de estoque (permissão integrations.hubsoft_bulk).
// Lógica em integrationhubsoft/stock_transfer.go.
//   POST …/hubsoft/stock-transfer/preview — confere cada patrimônio e o destino (somente leitura; até 60 linhas)
//   POST …/hubsoft/stock-transfer/apply   — refaz a conferência, transfere (agrupando por origem+produto), relê e grava ops_audit_log (até 30 linhas)

type stockTransferBody struct {
	integrationhubsoft.TransferOptions
	Rows []integrationhubsoft.TransferRow `json:"rows"`
}

func decodeStockTransfer(w http.ResponseWriter, r *http.Request, max int) (stockTransferBody, bool) {
	var b stockTransferBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido: "+err.Error(), nil)
		return b, false
	}
	if len(b.Rows) == 0 || len(b.Rows) > max {
		writeErr(w, http.StatusBadRequest, "BAD_ROWS", "envie de 1 a "+strconv.Itoa(max)+" linhas por chamada", nil)
		return b, false
	}
	return b, true
}

func (s *Server) hubsoftStockTransferPreview(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStockTransfer(w, r, 60)
	if !ok {
		return
	}
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 5*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	dest, results := integrationhubsoft.PreviewTransferRows(ctx, cfg, token, body.Rows, body.TransferOptions)
	writeJSON(w, http.StatusOK, map[string]any{"destino": dest, "results": results})
}

func (s *Server) hubsoftStockTransferApply(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStockTransfer(w, r, 30)
	if !ok {
		return
	}
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 8*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	results, halted := integrationhubsoft.ApplyTransferRows(ctx, cfg, token, body.Rows, body.TransferOptions)
	actor := s.actorFromRequest(r)
	for i, res := range results {
		if res.Action == "skipped" {
			continue // nada aconteceu: sem registro de auditoria
		}
		ref := strings.TrimSpace(body.Rows[i].IDProdutoItem)
		after := map[string]any{
			"ok": res.OK, "message": res.Message, "description": res.Message, "label": res.Message, "reference": ref, "id_produto_item": ref,
			"id_movimento_estoque": res.IDMovimento, "id_local_destino": strings.TrimSpace(body.IDLocalDestino), "local_origem": res.LocalOrigem,
			"local_depois": res.LocalDepois, "status_depois": res.StatusDepois, "detail": res.Detail, "problems": res.Problems, "linha": res.Linha, "observacao": body.Observacao,
		}
		if res.Item != nil {
			after["codigo_item"], after["identificador_proprio"], after["produto"] = res.Item.CodigoItem, res.Item.Identificador, res.Item.Produto
		}
		if res.Action == "failed" || res.Action == "verify_failed" {
			s.Log.Warn().Int("linha", res.Linha).Str("id_produto_item", ref).Str("acao", res.Action).Str("mensagem", res.Message).Str("detalhe", res.Detail).Msg("hubsoft stock-transfer: problema")
		}
		s.appendAuditLog(ctx, "hubsoft_stock_transfer", ref, res.Action, actor, nil, after)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "halted": halted})
}
