package api

import (
	"context"
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
