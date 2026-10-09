package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// Comodato de UM patrimônio para o serviço de um cliente (passo 3 da migração). Permissão integrations.hubsoft_bulk.
// Lógica em integrationhubsoft/stock_comodato.go.
//   GET  …/hubsoft/stock-comodato/movement-types   — tipos de movimento vistos em saídas para clientes (status que geram)
//   GET  …/hubsoft/stock-comodato/service?termo=   — serviço por id ou login + o que ele já tem vinculado
//   GET  …/hubsoft/stock-comodato/item?campo=&valor= — patrimônio (id, identificador, série, MAC ou código do item)
//   POST …/hubsoft/stock-comodato/preview          — confere tudo (somente leitura)
//   POST …/hubsoft/stock-comodato/apply            — refaz a conferência, envia a saída e relê; grava ops_audit_log
//   POST …/hubsoft/stock-comodato/preview-batch    — confere várias linhas «patrimônio → serviço» (somente leitura)
//   POST …/hubsoft/stock-comodato/apply-batch      — envia um grupo pequeno de linhas (o front manda 5 por vez); 1 registro de auditoria por linha

// Os tipos de movimento só mudam quando alguém cria um tipo novo — a descoberta lê milhares de movimentos, então fica em cache.
type movementTypesCacheEntry struct {
	at    time.Time
	types []integrationhubsoft.MovementType
}

var (
	movementTypesMu    sync.Mutex
	movementTypesCache = map[string]movementTypesCacheEntry{}
)

const movementTypesTTL = 15 * time.Minute

func (s *Server) stockComodatoTypes(ctx context.Context, integKey string, cfg integrationhubsoft.Config, token string, force bool) ([]integrationhubsoft.MovementType, error) {
	movementTypesMu.Lock()
	e, ok := movementTypesCache[integKey]
	movementTypesMu.Unlock()
	if ok && !force && time.Since(e.at) < movementTypesTTL {
		return e.types, nil
	}
	types, err := integrationhubsoft.DiscoverMovementTypes(ctx, cfg, token)
	if err != nil {
		return nil, err
	}
	movementTypesMu.Lock()
	movementTypesCache[integKey] = movementTypesCacheEntry{at: time.Now(), types: types}
	movementTypesMu.Unlock()
	return types, nil
}

// stockComodatoPrepare resolve integração, config e token (responde o erro e devolve ok=false se falhar).
func (s *Server) stockComodatoPrepare(w http.ResponseWriter, r *http.Request, timeout time.Duration) (cfg integrationhubsoft.Config, token, integKey string, ctx context.Context, cancel context.CancelFunc, ok bool) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	c, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, timeout+30*time.Second)
	cx, cn := context.WithTimeout(r.Context(), timeout)
	tok, err := s.hubsoftToken(cx, integID, c)
	if err != nil {
		cn()
		writeErr(w, http.StatusBadGateway, "AUTH", "não foi possível autenticar na HubSoft: "+err.Error(), nil)
		return
	}
	return c, tok, integID.String(), cx, cn, true
}

func (s *Server) hubsoftStockComodatoTypes(w http.ResponseWriter, r *http.Request) {
	cfg, token, key, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 4*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	types, err := s.stockComodatoTypes(ctx, key, cfg, token, r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error(), "types": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "types": types})
}

func (s *Server) hubsoftStockComodatoService(w http.ResponseWriter, r *http.Request) {
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, time.Minute)
	if !ok {
		return
	}
	defer cancel()
	svcs, err := integrationhubsoft.FindServices(ctx, cfg, token, r.URL.Query().Get("termo"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error(), "services": []any{}})
		return
	}
	out := make([]map[string]any, 0, len(svcs))
	for _, sv := range svcs {
		links, lerr := integrationhubsoft.ListServiceLinks(ctx, cfg, token, sv.IDClienteServico)
		item := map[string]any{"service": sv, "links": links}
		if lerr != nil {
			item["links_error"] = lerr.Error()
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "services": out})
}

func (s *Server) hubsoftStockComodatoItem(w http.ResponseWriter, r *http.Request) {
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, time.Minute)
	if !ok {
		return
	}
	defer cancel()
	q := r.URL.Query()
	it, err := integrationhubsoft.FindItem(ctx, cfg, token, q.Get("campo"), q.Get("valor"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "item": map[string]any{
		"id_produto_item": it.ID, "id_produto": it.ProdutoID, "produto": it.ProdutoNome, "id_local_estoque": it.LocalID, "local": it.LocalNome,
		"status": it.Status, "status_prefixo": it.StatusPrefix, "identificador_proprio": it.Identificador, "numero_serie": it.Serie, "mac_address": it.MAC,
		"codigo_item": it.CodigoItem, "cliente": it.Cliente, "observacoes": it.Observacoes,
	}})
}

func decodeComodatoReq(w http.ResponseWriter, r *http.Request) (integrationhubsoft.ComodatoRequest, bool) {
	var req integrationhubsoft.ComodatoRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido: "+err.Error(), nil)
		return req, false
	}
	return req, true
}

func (s *Server) hubsoftStockComodatoPreview(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeComodatoReq(w, r)
	if !ok {
		return
	}
	cfg, token, key, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 5*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	types, terr := s.stockComodatoTypes(ctx, key, cfg, token, false)
	pv := integrationhubsoft.PreviewComodato(ctx, cfg, token, req, types)
	if terr != nil {
		pv.Warnings = append(pv.Warnings, "não foi possível descobrir os tipos de movimento: "+terr.Error())
	}
	writeJSON(w, http.StatusOK, pv)
}

func (s *Server) hubsoftStockComodatoApply(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeComodatoReq(w, r)
	if !ok {
		return
	}
	cfg, token, key, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 5*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	types, _ := s.stockComodatoTypes(ctx, key, cfg, token, false)
	res := integrationhubsoft.ApplyComodato(ctx, cfg, token, req, types)

	after := map[string]any{
		"ok": res.OK, "message": res.Message, "description": res.Message, "label": res.Message, "reference": strings.TrimSpace(req.Valor),
		"id_movimento_estoque": res.IDMovimento, "id_produto_item": res.IDProdutoItem, "id_cliente_servico": strings.TrimSpace(req.IDClienteServico),
		"id_tipo_movimento_estoque": strings.TrimSpace(req.IDTipoMovimento), "status_depois": res.StatusDepois, "detail": res.Detail,
		"problems": res.Preview.Problems, "campo": req.Campo, "valor": req.Valor, "observacao": req.Observacao,
	}
	if res.Preview.Service != nil {
		after["cliente"], after["id_cliente"], after["login"] = res.Preview.Service.Cliente, res.Preview.Service.IDCliente, res.Preview.Service.Login
	}
	if res.Verified != nil {
		after["verified"], after["verify_message"] = *res.Verified, res.VerifyMessage
	}
	if res.Action == "failed" || res.Action == "verify_failed" {
		s.Log.Warn().Str("id_cliente_servico", req.IDClienteServico).Str("id_produto_item", res.IDProdutoItem).Str("acao", res.Action).Str("mensagem", res.Message).Str("detalhe", res.Detail).Msg("hubsoft stock-comodato: problema")
	}
	s.appendAuditLog(ctx, "hubsoft_stock_comodato", res.IDProdutoItem, res.Action, s.actorFromRequest(r), nil, after)
	writeJSON(w, http.StatusOK, res)
}

type comodatoBatchBody struct {
	integrationhubsoft.ComodatoBatchOptions
	Rows []integrationhubsoft.ComodatoRow `json:"rows"`
}

const comodatoBatchMax = 25

func decodeComodatoBatch(w http.ResponseWriter, r *http.Request) (comodatoBatchBody, bool) {
	var b comodatoBatchBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido: "+err.Error(), nil)
		return b, false
	}
	if len(b.Rows) == 0 || len(b.Rows) > comodatoBatchMax {
		writeErr(w, http.StatusBadRequest, "BAD_ROWS", "envie de 1 a 25 linhas por chamada", nil)
		return b, false
	}
	return b, true
}

func (s *Server) hubsoftStockComodatoPreviewBatch(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeComodatoBatch(w, r)
	if !ok {
		return
	}
	cfg, token, key, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 5*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	types, terr := s.stockComodatoTypes(ctx, key, cfg, token, false)
	out := map[string]any{"results": integrationhubsoft.PreviewComodatoRows(ctx, cfg, token, body.Rows, body.ComodatoBatchOptions, types)}
	if terr != nil {
		out["types_warning"] = "não foi possível descobrir os tipos de movimento: " + terr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hubsoftStockComodatoApplyBatch(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeComodatoBatch(w, r)
	if !ok {
		return
	}
	cfg, token, key, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 5*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	types, _ := s.stockComodatoTypes(ctx, key, cfg, token, false)
	results := integrationhubsoft.ApplyComodatoRows(ctx, cfg, token, body.Rows, body.ComodatoBatchOptions, types)
	actor := s.actorFromRequest(r)
	for i, rr := range results {
		row := body.Rows[i]
		res := rr.Result
		after := map[string]any{
			"ok": res.OK, "message": res.Message, "description": res.Message, "label": res.Message, "reference": strings.TrimSpace(row.IDClienteServico),
			"id_movimento_estoque": res.IDMovimento, "id_produto_item": res.IDProdutoItem, "id_cliente_servico": strings.TrimSpace(row.IDClienteServico),
			"id_tipo_movimento_estoque": strings.TrimSpace(body.IDTipoMovimento), "status_depois": res.StatusDepois, "detail": res.Detail,
			"problems": res.Preview.Problems, "linha": row.Linha, "campo": rr.Locator, "observacao": row.Observacao, "lote": true,
		}
		if res.Preview.Service != nil {
			after["cliente"], after["id_cliente"], after["login"] = res.Preview.Service.Cliente, res.Preview.Service.IDCliente, res.Preview.Service.Login
		}
		if res.Verified != nil {
			after["verified"], after["verify_message"] = *res.Verified, res.VerifyMessage
		}
		if res.Action == "failed" || res.Action == "verify_failed" {
			s.Log.Warn().Int("linha", row.Linha).Str("id_cliente_servico", row.IDClienteServico).Str("id_produto_item", res.IDProdutoItem).Str("acao", res.Action).Str("mensagem", res.Message).Str("detalhe", res.Detail).Msg("hubsoft stock-comodato lote: problema")
		}
		if res.Action == "skipped" {
			continue // nada aconteceu: sem registro de auditoria
		}
		s.appendAuditLog(ctx, "hubsoft_stock_comodato", res.IDProdutoItem, res.Action, actor, nil, after)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
