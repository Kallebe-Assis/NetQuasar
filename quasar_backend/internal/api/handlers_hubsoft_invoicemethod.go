package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// GET /integrations/{id}/hubsoft/report/invoices-by-method?forma=...&data_inicio=...&data_fim=...
//
// Boletos EM ABERTO cuja fatura carrega a forma de cobrança `forma` (id numérico ou parte do nome). Somente leitura.
// Serve para achar boletos gerados numa forma de cobrança que o cliente/serviço já não usa mais (ver
// integrationhubsoft.BuildInvoicesByMethod). Sem `forma`, devolve só a contagem por forma de cobrança.
func (s *Server) hubsoftReportInvoicesByMethod(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	q := r.URL.Query()
	// Boleto em aberto pode ser antigo: por omissão olha 3 anos para trás e 120 dias para a frente.
	now := time.Now()
	from := strings.TrimSpace(q.Get("data_inicio"))
	to := strings.TrimSpace(q.Get("data_fim"))
	if from == "" {
		from = now.AddDate(-3, 0, 0).Format("2006-01-02")
	}
	if to == "" {
		to = now.AddDate(0, 0, 120).Format("2006-01-02")
	}
	if from > to {
		writeErr(w, http.StatusBadRequest, "BAD_PERIOD", "o período está invertido (a data inicial é maior que a final)", nil)
		return
	}
	extendWriteDeadline(w, 12*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.BuildInvoicesByMethod(ctx, cfg, token, from, to, q.Get("forma")))
}

// GET /integrations/{id}/hubsoft/report/invoices-by-method/formas — formas de cobrança cadastradas na HubSoft
// (catálogo de Configuração, somente leitura), para a tela oferecer uma lista em vez de um campo de texto livre.
// Fica no grupo dos relatórios (não exige a permissão de edição em massa).
func (s *Server) hubsoftInvoiceMethodFormas(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	formas, err := integrationhubsoft.ListFormasCobranca(ctx, cfg, token)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"formas": formas})
}

type hubsoftServiceFormaBody struct {
	IDs   []string `json:"ids"`
	Forma string   `json:"forma"`
}

// POST /integrations/{id}/hubsoft/report/invoices-by-method/services-check {"ids":[id_cliente…],"forma":"<id ou nome>"}
//
// Confere se os serviços desses clientes estão AGORA na forma de cobrança informada (somente leitura — lê GET /cliente).
func (s *Server) hubsoftServiceFormaCheck(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	var body hubsoftServiceFormaBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	extendWriteDeadline(w, 5*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, integrationhubsoft.CheckServiceFormas(ctx, cfg, token, body.IDs, body.Forma))
}
