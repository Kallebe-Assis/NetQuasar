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

// Cadastro em massa de PRODUTOS de estoque na HubSoft (passo 1 da migração de patrimônios). Exige a permissão
// integrations.hubsoft_bulk (ver server.go). Lógica em integrationhubsoft/stock_product.go.
//   POST …/hubsoft/stock-products/validate   — só formato (+ IDs de categoria/marca/tipo contra a HubSoft); não altera nada
//   POST …/hubsoft/stock-products/preflight  — compara com os produtos já cadastrados; não altera nada
//   POST …/hubsoft/stock-products/apply      — cria + configura + confere, até 20 linhas por chamada; cada linha vai para ops_audit_log

type stockProductsBody struct {
	Rows          []json.RawMessage `json:"rows"`
	CheckCatalogs bool              `json:"check_catalogs"`
	// RepairBrand — no apply: tenta corrigir a marca dos produtos que já existem com o mesmo código.
	RepairBrand bool `json:"repair_brand"`
}

func decodeStockRows(raw []json.RawMessage) []integrationhubsoft.StockProductRow {
	rows := make([]integrationhubsoft.StockProductRow, 0, len(raw))
	for i, r := range raw {
		var row integrationhubsoft.StockProductRow
		_ = json.Unmarshal(r, &row)
		row.Line = bulkRowLine(r, i)
		rows = append(rows, row)
	}
	return rows
}

// stockProductsPrepare resolve integração/config e lê o corpo; devolve ok=false depois de já ter respondido o erro.
func (s *Server) stockProductsPrepare(w http.ResponseWriter, r *http.Request, maxRows int, maxBytes int64) (cfg integrationhubsoft.Config, integID string, body stockProductsBody, ok bool) {
	id, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
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
	c, err := s.loadHubsoftConfig(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	return c, id.String(), body, true
}

func (s *Server) hubsoftStockProductsValidate(w http.ResponseWriter, r *http.Request) {
	cfg, _, body, ok := s.stockProductsPrepare(w, r, 2000, 8<<20)
	if !ok {
		return
	}
	extendWriteDeadline(w, 2*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var cat integrationhubsoft.StockCatalogSets
	var unchecked []string
	if body.CheckCatalogs {
		integID, _ := s.resolveIntegrationID(ctx, chi.URLParam(r, "id"))
		token, err := s.hubsoftToken(ctx, integID, cfg)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "AUTH", "não foi possível autenticar na HubSoft: "+err.Error(), nil)
			return
		}
		cat, unchecked = integrationhubsoft.LoadStockCatalogSets(ctx, cfg, token)
	}
	res := integrationhubsoft.ValidateStockProductRows(decodeStockRows(body.Rows), cat)
	res.UncheckedCatalogs = unchecked
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) hubsoftStockProductsPreflight(w http.ResponseWriter, r *http.Request) {
	cfg, _, body, ok := s.stockProductsPrepare(w, r, 2000, 8<<20)
	if !ok {
		return
	}
	extendWriteDeadline(w, 2*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	integID, _ := s.resolveIntegrationID(ctx, chi.URLParam(r, "id"))
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", "não foi possível autenticar na HubSoft: "+err.Error(), nil)
		return
	}
	results, err := integrationhubsoft.PreflightStockProducts(ctx, cfg, token, decodeStockRows(body.Rows))
	if err != nil {
		s.Log.Warn().Err(err).Msg("hubsoft stock-products preflight falhou")
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) hubsoftStockProductsApply(w http.ResponseWriter, r *http.Request) {
	cfg, integIDStr, body, ok := s.stockProductsPrepare(w, r, 20, 1<<20)
	if !ok {
		return
	}
	extendWriteDeadline(w, 3*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	integID, _ := s.resolveIntegrationID(ctx, chi.URLParam(r, "id"))
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", "não foi possível autenticar na HubSoft: "+err.Error(), nil)
		return
	}
	// Lista atual de produtos — carregada UMA vez por lote, para a checagem de duplicidade de cada linha.
	ix, err := integrationhubsoft.NewStockProductIndex(ctx, cfg, token)
	if err != nil {
		s.Log.Warn().Err(err).Msg("hubsoft stock-products apply: lista de produtos indisponível")
		writeErr(w, http.StatusBadGateway, "HUBSOFT", "nada foi criado: "+err.Error(), nil)
		return
	}
	brands, err := integrationhubsoft.LoadStockBrandNames(ctx, cfg, token)
	if err != nil {
		s.Log.Warn().Err(err).Msg("hubsoft stock-products apply: marcas indisponíveis")
		writeErr(w, http.StatusBadGateway, "HUBSOFT", "nada foi criado: "+err.Error(), nil)
		return
	}
	actor := s.actorFromRequest(r)
	rows := decodeStockRows(body.Rows)
	for i := range rows {
		rows[i].RepairBrand = body.RepairBrand
	}
	results := make([]integrationhubsoft.StockApplyResult, 0, len(rows))
	halted, consecErr := false, 0
	for i, row := range rows {
		if i > 0 {
			time.Sleep(350 * time.Millisecond) // folga sob o limite de 20 req/s da HubSoft
		}
		res := integrationhubsoft.ApplyStockProduct(ctx, cfg, token, row, ix, brands)
		results = append(results, res)

		switch res.Action {
		case integrationhubsoft.StockActionFailed, integrationhubsoft.StockActionRejected, integrationhubsoft.StockActionConfigFailed, integrationhubsoft.StockActionBrandRepairFailed:
			consecErr++
			s.Log.Warn().Str("integration_id", integIDStr).Int("line", res.Line).Str("produto", res.Label).Str("acao", res.Action).Str("mensagem", res.Message).Str("detalhe", res.Detail).Msg("hubsoft stock-products: linha com problema")
		default:
			consecErr = 0
		}
		after := map[string]any{
			"line": res.Line, "label": res.Label, "description": res.Label, "reference": strings.TrimSpace(row.Codigo), "ok": res.OK, "message": res.Message,
			"id_produto": res.IDProduto, "created": res.Created, "rejected": res.Rejected, "detail": res.Detail, "brand_repair": res.BrandRepair,
		}
		if res.ConfigOK != nil {
			after["config_ok"], after["config_message"] = *res.ConfigOK, res.ConfigMessage
		}
		if res.Verified != nil {
			after["verified"], after["verify_message"] = *res.Verified, res.VerifyMessage
		}
		s.appendAuditLog(ctx, "hubsoft_stock_product", res.IDProduto, res.Action, actor, nil, after)
		if consecErr >= 3 {
			halted = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "halted": halted})
}
