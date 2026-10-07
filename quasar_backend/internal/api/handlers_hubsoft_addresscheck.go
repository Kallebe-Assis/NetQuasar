package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// hubsoftAddressCheckScan — conferência dos 4 endereços (fiscal, cadastral, cobrança, instalação) de todos os serviços da
// HubSoft. SOMENTE LEITURA: devolve os serviços em que os quatro não são iguais, com o padrão da divergência.
func (s *Server) hubsoftAddressCheckScan(w http.ResponseWriter, r *http.Request) {
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
	extendWriteDeadline(w, 12*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	res, err := integrationhubsoft.ScanAddresses(ctx, cfg, token, r.URL.Query().Get("cancelados") == "1")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	if res.Rows == nil {
		res.Rows = []integrationhubsoft.AddressCheckRow{}
	}
	writeJSON(w, http.StatusOK, res)
}
