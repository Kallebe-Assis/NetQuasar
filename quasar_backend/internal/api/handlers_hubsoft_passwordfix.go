package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// hubsoftPasswordFixScan — varre a base da HubSoft (somente leitura) atrás de senhas gravadas no formato
// ="12345" (texto de planilha) e devolve a lista, com a senha correta de cada uma.
func (s *Server) hubsoftPasswordFixScan(w http.ResponseWriter, r *http.Request) {
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
	scan, err := integrationhubsoft.ScanLiteralPasswords(ctx, cfg, token)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "HUBSOFT", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

type hubsoftPasswordFixBody struct {
	Rows []struct {
		IDClienteServico string `json:"id_cliente_servico"`
	} `json:"rows"`
}

// hubsoftPasswordFixApply — troca ="12345" por 12345 nos serviços informados. Cada serviço é relido antes e só é
// alterado se a senha atual ainda estiver nesse formato. A auditoria guarda só os ids (nunca a senha).
func (s *Server) hubsoftPasswordFixApply(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftPasswordFixBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 50 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 50 serviços por chamada", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	actor := s.actorFromRequest(r)
	results := make([]integrationhubsoft.LiteralPasswordFixResult, 0, len(body.Rows))
	for i, row := range body.Rows {
		if i > 0 {
			time.Sleep(350 * time.Millisecond)
		}
		res := integrationhubsoft.FixLiteralPassword(ctx, cfg, token, row.IDClienteServico)
		if res.OK && !res.Skipped {
			s.appendAuditLog(ctx, "hubsoft_client_service", res.IDClienteServico, "password_fix", actor, nil, map[string]any{"integration_id": integID.String(), "login": res.Login})
			res.Message = "senha corrigida (formato =\"…\" removido)" // não devolve/guarda a senha no histórico
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
