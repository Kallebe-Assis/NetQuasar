package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// Atendimento + Ordem de Serviço (permissão integrations.hubsoft_bulk). Lógica em integrationhubsoft/service_order.go.
//   GET  …/hubsoft/service-order/catalogs — tipos, status, técnicos, disponibilidades, origens (somente leitura)
//   POST …/hubsoft/service-order/preview  — confere as linhas (somente leitura; até 40)
//   POST …/hubsoft/service-order/apply    — confere de novo, abre atendimento (+O.S.) e confere a resposta; grava auditoria (até 12)

type serviceOrderBody struct {
	Rows    []integrationhubsoft.ServiceOrderRow   `json:"rows"`
	Options integrationhubsoft.ServiceOrderOptions `json:"options"`
}

func decodeServiceOrder(w http.ResponseWriter, r *http.Request, max int) (serviceOrderBody, bool) {
	var b serviceOrderBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido: "+err.Error(), nil)
		return b, false
	}
	if len(b.Rows) == 0 || len(b.Rows) > max {
		writeErr(w, http.StatusBadRequest, "BAD_ROWS", "envie de 1 a "+strconv.Itoa(max)+" linhas por chamada", nil)
		return b, false
	}
	return b, true
}

// serviceOrderDoneRefs — referências (ex.: id da O.S. do IXC) que já viraram atendimento, lidas do log de auditoria.
func (s *Server) serviceOrderDoneRefs(ctx context.Context) map[string]string {
	out := map[string]string{}
	rows, err := s.DB().Query(ctx, `
		SELECT entity_id, COALESCE(after_data->>'id_atendimento',''), created_at
		FROM ops_audit_log WHERE entity_type = 'hubsoft_service_order' AND action = 'created'`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var ref, at string
		var when time.Time
		if rows.Scan(&ref, &at, &when) != nil || strings.TrimSpace(ref) == "" {
			continue
		}
		out[ref] = "atendimento " + at + " em " + when.Local().Format("02/01/2006 15:04")
	}
	return out
}

func (s *Server) hubsoftServiceOrderCatalogs(w http.ResponseWriter, r *http.Request) {
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 2*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, integrationhubsoft.LoadServiceOrderCatalogs(ctx, cfg, token))
}

func (s *Server) hubsoftServiceOrderPreview(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeServiceOrder(w, r, 40)
	if !ok {
		return
	}
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 6*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	body.Options.Done = s.serviceOrderDoneRefs(ctx)
	cat := integrationhubsoft.LoadServiceOrderCatalogs(ctx, cfg, token)
	writeJSON(w, http.StatusOK, map[string]any{"results": integrationhubsoft.PreviewServiceOrderRows(ctx, cfg, token, body.Rows, body.Options, cat), "catalog_errors": cat.Erros})
}

func (s *Server) hubsoftServiceOrderApply(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeServiceOrder(w, r, 12)
	if !ok {
		return
	}
	cfg, token, _, ctx, cancel, ok := s.stockComodatoPrepare(w, r, 8*time.Minute)
	if !ok {
		return
	}
	defer cancel()
	body.Options.Done = s.serviceOrderDoneRefs(ctx)
	cat := integrationhubsoft.LoadServiceOrderCatalogs(ctx, cfg, token)
	results, halted := integrationhubsoft.ApplyServiceOrderRows(ctx, cfg, token, body.Rows, body.Options, cat)
	actor := s.actorFromRequest(r)
	for i, res := range results {
		if res.Action == "skipped" {
			continue // nada aconteceu: sem registro de auditoria
		}
		ref := strings.TrimSpace(body.Rows[i].Referencia)
		if ref == "" {
			ref = "linha-" + strconv.Itoa(res.Linha)
		}
		after := map[string]any{
			"ok": res.OK, "message": res.Message, "description": res.Message, "label": res.Message, "reference": ref, "linha": res.Linha,
			"id_atendimento": res.IDAtendimento, "protocolo": res.Protocolo, "ordens_servico": res.OrdensServico, "detail": res.Detail, "problems": res.Problems,
			"payload": res.Preview.Body,
		}
		if sv := res.Preview.Service; sv != nil {
			after["cliente"], after["id_cliente_servico"], after["login"] = sv.Cliente, sv.IDClienteServico, sv.Login
		}
		if res.Action == "failed" || res.Action == "verify_failed" {
			s.Log.Warn().Int("linha", res.Linha).Str("referencia", ref).Str("acao", res.Action).Str("mensagem", res.Message).Str("detalhe", res.Detail).Msg("hubsoft service-order: problema")
		}
		s.appendAuditLog(ctx, "hubsoft_service_order", ref, res.Action, actor, nil, after)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "halted": halted})
}
