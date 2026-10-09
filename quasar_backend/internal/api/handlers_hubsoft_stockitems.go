package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// Cadastro em massa de PATRIMÔNIOS de estoque na HubSoft (passo 2 da migração). Permissão integrations.hubsoft_bulk.
// Lógica em integrationhubsoft/stock_item.go: cada linha = um patrimônio (entrada de estoque de 1 unidade + gravação de
// identificador/série/MAC + conferência), com checagem de duplicidade em toda a HubSoft.
//   POST …/hubsoft/stock-items/validate   — formato + produtos reais da HubSoft; não altera nada
//   POST …/hubsoft/stock-items/preflight  — compara com os patrimônios existentes (série/MAC/identificador); não altera nada
//   POST …/hubsoft/stock-items/apply      — cria + identifica + confere, até 10 linhas por chamada; cada linha vai para ops_audit_log

type stockItemsBody struct {
	Rows          []json.RawMessage `json:"rows"`
	CheckProducts bool              `json:"check_products"`
	// IDLocalEstoque — local de estoque da entrada, escolhido na tela; vale para todas as linhas.
	IDLocalEstoque string `json:"id_local_estoque"`
}

func decodeStockItemRows(body stockItemsBody) []integrationhubsoft.StockItemRow {
	rows := make([]integrationhubsoft.StockItemRow, 0, len(body.Rows))
	for i, raw := range body.Rows {
		var row integrationhubsoft.StockItemRow
		_ = json.Unmarshal(raw, &row)
		row.Line = bulkRowLine(raw, i)
		row.IDLocalEstoque = strings.TrimSpace(body.IDLocalEstoque)
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) stockItemsPrepare(w http.ResponseWriter, r *http.Request, maxRows int, maxBytes int64) (cfg integrationhubsoft.Config, body stockItemsBody, ok bool) {
	if _, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido: "+err.Error(), nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > maxRows {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a "+strconv.Itoa(maxRows)+" linhas", nil)
		return
	}
	integID, _ := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	c, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	return c, body, true
}

func (s *Server) hubsoftStockItemsToken(ctx context.Context, w http.ResponseWriter, r *http.Request, cfg integrationhubsoft.Config) (string, bool) {
	integID, _ := s.resolveIntegrationID(ctx, chi.URLParam(r, "id"))
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", "não foi possível autenticar na HubSoft: "+err.Error(), nil)
		return "", false
	}
	return token, true
}

func (s *Server) hubsoftStockItemsValidate(w http.ResponseWriter, r *http.Request) {
	cfg, body, ok := s.stockItemsPrepare(w, r, 10000, 32<<20)
	if !ok {
		return
	}
	extendWriteDeadline(w, 3*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var meta map[string]integrationhubsoft.StockItemProduct
	if body.CheckProducts {
		token, ok := s.hubsoftStockItemsToken(ctx, w, r, cfg)
		if !ok {
			return
		}
		prods, err := integrationhubsoft.ListStockProducts(ctx, cfg, token)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
			return
		}
		meta = map[string]integrationhubsoft.StockItemProduct{}
		for _, p := range prods {
			meta[p.ID] = integrationhubsoft.StockItemProduct{Nome: p.Nome, Patrimonio: p.Patrimonial}
		}
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.ValidateStockItemRows(decodeStockItemRows(body), meta))
}

func (s *Server) hubsoftStockItemsPreflight(w http.ResponseWriter, r *http.Request) {
	cfg, body, ok := s.stockItemsPrepare(w, r, 10000, 32<<20)
	if !ok {
		return
	}
	// Lê TODOS os patrimônios da HubSoft (produto a produto) para achar duplicidade em qualquer produto — pode demorar.
	extendWriteDeadline(w, 12*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Minute)
	defer cancel()
	token, ok := s.hubsoftStockItemsToken(ctx, w, r, cfg)
	if !ok {
		return
	}
	results, summary, err := integrationhubsoft.PreflightStockItems(ctx, cfg, token, decodeStockItemRows(body))
	if err != nil {
		s.Log.Warn().Err(err).Msg("hubsoft stock-items preflight falhou")
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "summary": summary})
}

func (s *Server) hubsoftStockItemsApply(w http.ResponseWriter, r *http.Request) {
	cfg, body, ok := s.stockItemsPrepare(w, r, 10, 1<<20)
	if !ok {
		return
	}
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	token, ok := s.hubsoftStockItemsToken(ctx, w, r, cfg)
	if !ok {
		return
	}
	integID, _ := s.resolveIntegrationID(ctx, chi.URLParam(r, "id"))
	actor := s.actorFromRequest(r)
	rows := decodeStockItemRows(body)
	results := make([]integrationhubsoft.StockItemResult, 0, len(rows))
	halted, consecErr := false, 0
	for i, row := range rows {
		if i > 0 {
			time.Sleep(250 * time.Millisecond) // folga sob o limite de 20 req/s da HubSoft
		}
		res := integrationhubsoft.ApplyStockItem(ctx, cfg, token, row, nil)
		results = append(results, res)

		switch res.Action {
		case integrationhubsoft.StockItemActionFailed, integrationhubsoft.StockItemActionRejected, integrationhubsoft.StockItemActionIdentifyFailed:
			consecErr++
			s.Log.Warn().Str("integration_id", integID.String()).Int("line", res.Line).Str("patrimonio", res.Label).Str("acao", res.Action).
				Str("id_produto_item", res.IDProdutoItem).Str("mensagem", res.Message).Str("detalhe", res.Detail).Msg("hubsoft stock-items: linha com problema")
		default:
			consecErr = 0
		}
		after := map[string]any{
			"line": res.Line, "label": res.Label, "description": res.Label, "reference": strings.TrimSpace(row.IdentificadorProprio), "ok": res.OK, "message": res.Message,
			"id_produto_item": res.IDProdutoItem, "codigo_item": res.CodigoItem, "created": res.Created, "pending": res.Pending, "detail": res.Detail,
			"id_produto": row.IDProduto, "numero_serie": row.NumeroSerie, "mac_address": row.MacAddress, "id_local_estoque": row.IDLocalEstoque,
			"mac_omitted": res.MacOmitted, "mac_omitted_reason": res.MacOmittedReason,
		}
		if res.IdentifyOK != nil {
			after["identify_ok"], after["identify_message"] = *res.IdentifyOK, res.IdentifyMsg
		}
		if res.Verified != nil {
			after["verified"], after["verify_message"] = *res.Verified, res.VerifyMessage
		}
		s.appendAuditLog(ctx, "hubsoft_stock_item", res.IDProdutoItem, res.Action, actor, nil, after)
		if consecErr >= 3 {
			halted = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "halted": halted})
}

// POST …/hubsoft/stock-items/check — CONFERÊNCIA (somente leitura): compara o CSV de patrimônios com a HubSoft, campo a campo.
func (s *Server) hubsoftStockItemsCheck(w http.ResponseWriter, r *http.Request) {
	cfg, body, ok := s.stockItemsPrepare(w, r, 10000, 32<<20)
	if !ok {
		return
	}
	extendWriteDeadline(w, 12*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Minute)
	defer cancel()
	token, ok := s.hubsoftStockItemsToken(ctx, w, r, cfg)
	if !ok {
		return
	}
	res, summary, err := integrationhubsoft.CheckStockItems(ctx, cfg, token, decodeStockItemRows(body))
	if err != nil {
		s.Log.Warn().Err(err).Msg("hubsoft stock-items check falhou")
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": res, "summary": summary})
}
