package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationixc"
)

// Inativar/reativar logins (radusuarios) do IXC em massa — migração para a HubSoft. Permissão "integrations.ixc_logins".
//   POST /integrations/{id}/ixc/logins/preview  — SOMENTE LEITURA: situação atual de cada login da lista.
//   POST /integrations/{id}/ixc/logins/apply    — inativa ("inativar") ou reativa ("reativar") os ids informados, com conferência
//                                                  de leitura antes e depois e registro no histórico (com o estado anterior).

func (s *Server) ixcClient(ctx context.Context, idOrSlug string) (integrationixc.Client, bool) {
	integID, err := s.resolveIntegrationID(ctx, idOrSlug)
	if err != nil {
		return integrationixc.Client{}, false
	}
	cfg, err := s.loadIntegrationRunner(ctx, integID)
	if err != nil {
		return integrationixc.Client{}, false
	}
	return integrationixc.Client{Cfg: cfg}, true
}

type ixcLoginPreviewRow struct {
	Login       string `json:"login"`
	Status      string `json:"status"` // pronto | ja_inativo | nao_encontrado | ambiguo | erro
	Message     string `json:"message,omitempty"`
	IDLogin     string `json:"id_login,omitempty"`
	LoginIXC    string `json:"login_ixc,omitempty"`
	Active      bool   `json:"ativo"`
	Online      bool   `json:"online"`
	IDCliente   string `json:"id_cliente,omitempty"`
	IDContrato  string `json:"id_contrato,omitempty"`
	ClienteNome string `json:"cliente,omitempty"`
}

func (s *Server) ixcLoginsPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Logins []string `json:"logins"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Logins) == 0 || len(body.Logins) > 100 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 100 logins por chamada", nil)
		return
	}
	cl, ok := s.ixcClient(r.Context(), chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "NOT_IXC", "integração IXC não encontrada", nil)
		return
	}
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	names := map[string]string{}
	rows := make([]ixcLoginPreviewRow, 0, len(body.Logins))
	for i, raw := range body.Logins {
		if i > 0 {
			time.Sleep(120 * time.Millisecond)
		}
		login := strings.TrimSpace(raw)
		row := ixcLoginPreviewRow{Login: login}
		if login == "" {
			row.Status, row.Message = "erro", "login vazio"
			rows = append(rows, row)
			continue
		}
		recs, err := cl.FindByLogin(ctx, login)
		switch {
		case err != nil:
			row.Status, row.Message = "erro", "falha ao consultar o IXC: "+err.Error()
		case len(recs) == 0:
			row.Status, row.Message = "nao_encontrado", "nenhum login com este nome no IXC"
		case len(recs) > 1:
			ids := make([]string, 0, len(recs))
			for _, x := range recs {
				ids = append(ids, x.Str("id"))
			}
			row.Status, row.Message = "ambiguo", "mais de um login com este nome no IXC (ids "+strings.Join(ids, ", ")+") — não será alterado"
		default:
			x := recs[0]
			row.IDLogin, row.LoginIXC = x.Str("id"), x.Str("login")
			row.Active, row.Online = x.Active(), x.Online()
			row.IDCliente, row.IDContrato = x.Str("id_cliente"), x.Str("id_contrato")
			if row.IDCliente != "" {
				if n, seen := names[row.IDCliente]; seen {
					row.ClienteNome = n
				} else {
					row.ClienteNome = cl.ClientName(ctx, row.IDCliente)
					names[row.IDCliente] = row.ClienteNome
				}
			}
			if row.Active {
				row.Status = "pronto"
			} else {
				row.Status, row.Message = "ja_inativo", "já está inativo no IXC"
			}
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

type ixcLoginApplyItem struct {
	IDLogin string `json:"id_login"`
	Login   string `json:"login"`
}

type ixcLoginApplyResult struct {
	IDLogin      string `json:"id_login"`
	Login        string `json:"login"`
	OK           bool   `json:"ok"`
	Changed      bool   `json:"changed"`
	Message      string `json:"message"`
	OnlineBefore bool   `json:"online_before"`
	OnlineAfter  bool   `json:"online_after"`
}

func (s *Server) ixcLoginsApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string              `json:"action"` // inativar | reativar
		Items  []ixcLoginApplyItem `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action != "inativar" && action != "reativar" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "action deve ser \"inativar\" ou \"reativar\"", nil)
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 25 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 25 logins por chamada", nil)
		return
	}
	cl, ok := s.ixcClient(r.Context(), chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "NOT_IXC", "integração IXC não encontrada", nil)
		return
	}
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	wantActive := action == "reativar"
	actor := s.actorFromRequest(r)
	results := make([]ixcLoginApplyResult, 0, len(body.Items))
	for i, it := range body.Items {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		id, login := strings.TrimSpace(it.IDLogin), strings.TrimSpace(it.Login)
		res := ixcLoginApplyResult{IDLogin: id, Login: login}
		if id == "" || login == "" {
			res.Message = "id do login e login são obrigatórios"
			results = append(results, res)
			continue
		}
		before, after, changed, err := cl.SetActive(ctx, id, login, wantActive)
		if before != nil {
			res.OnlineBefore = before.Online()
		}
		if after != nil {
			res.OnlineAfter = after.Online()
		}
		res.Changed = changed
		switch {
		case err != nil:
			res.Message = err.Error()
		case !changed && wantActive:
			res.OK, res.Message = true, "já estava ativo — nada foi alterado"
		case !changed:
			res.OK, res.Message = true, "já estava inativo — nada foi alterado"
		case wantActive:
			res.OK, res.Message = true, "login reativado no IXC"
		default:
			res.OK, res.Message = true, "login inativado no IXC"
		}
		if changed {
			// before_data guarda o registro completo anterior — é o que permite reverter à mão se preciso.
			s.appendAuditLog(ctx, "ixc_login", id, action, actor, before,
				map[string]any{"login": login, "ok": res.OK, "ativo_depois": after.Str("ativo"), "message": res.Message})
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
