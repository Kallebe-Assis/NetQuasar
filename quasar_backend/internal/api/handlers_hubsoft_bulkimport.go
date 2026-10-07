// Endpoints da importação em massa HubSoft (clientes/serviços): validar, pré-conferir, aplicar, conferir cadastro
// e registrar observações de login. Exigem a permissão integrations.hubsoft_bulk (ver server.go).

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// --- Importação em massa de clientes (permissão integrations.hubsoft_bulk) ---------------------------

// hubsoftBulkImportCatalogLabels — rótulo amigável de cada catálogo usado aqui, na ordem em que
// aparecem no aviso de "não foi possível conferir".
var hubsoftBulkImportCatalogLabels = map[string]string{
	"servico": "Serviços (planos)", "vencimento": "Vencimentos", "vendedor": "Vendedores",
	"forma_cobranca": "Formas de cobrança", "servico_status": "Status de serviço", "grupo_cliente": "Grupos de cliente",
}

// hubsoftBulkImportCatalogSets busca de uma vez os catálogos usados na validação referencial
// (id_servico, id_vencimento, id_usuario_vendedor, id_forma_cobranca, id_servico_status e, para
// linhas de cliente, ids_grupos_cliente). Falha ao buscar um catálogo não é fatal — só desliga a
// checagem referencial daquele campo (a validação de formato continua valendo) — MAS o chamador
// precisa avisar isso explicitamente ao operador (ver `unchecked` devolvido), nunca deixar parecer
// que um ID foi confirmado quando na verdade nem foi possível consultar o catálogo dele.
func (s *Server) hubsoftBulkImportCatalogSets(ctx context.Context, cfg integrationhubsoft.Config, token string, includeGrupoCliente bool) (sets integrationhubsoft.CatalogSets, unchecked []string) {
	fetch := func(which string) map[string]bool {
		_, body, err := integrationhubsoft.FetchCatalog(ctx, cfg, token, which)
		if err != nil {
			unchecked = append(unchecked, hubsoftBulkImportCatalogLabels[which])
			return nil
		}
		set := integrationhubsoft.CatalogSetsFromJSON(which, body)
		if set == nil {
			unchecked = append(unchecked, hubsoftBulkImportCatalogLabels[which])
		}
		return set
	}
	sets = integrationhubsoft.CatalogSets{
		Servico:       fetch("servico"),
		Vencimento:    fetch("vencimento"),
		Vendedor:      fetch("vendedor"),
		FormaCobranca: fetch("forma_cobranca"),
		ServicoStatus: fetch("servico_status"),
	}
	if includeGrupoCliente {
		sets.GrupoCliente = fetch("grupo_cliente")
	}
	return sets, unchecked
}

// hubsoftBulkImportInterfacePopLabels monta um mapa id_interface_conexao -> "POP X — equipamento Y
// (interface Z)" cruzando os catálogos "equipamento" (que traz as interfaces de cada equipamento) e
// "pop" (que traz os equipamentos de cada POP). Puramente informativo — nunca bloqueia a linha, só
// ajuda o operador a confirmar visualmente se o POP é o esperado antes de importar (ver pedido do
// usuário: "O POP de conexão é o 'SERVIDOR MIRACEMA', é obrigatório inserir também" — a API em si não
// tem um campo de id_pop separado, então isso vira uma nota de confirmação, não um campo extra).
// Falha ao consultar (rede/token) devolve mapa vazio — sem nota nenhuma, nunca um palpite.
func (s *Server) hubsoftBulkImportInterfacePopLabels(ctx context.Context, cfg integrationhubsoft.Config, token string) map[string]string {
	out := map[string]string{}
	_, eqBody, errEq := integrationhubsoft.FetchCatalog(ctx, cfg, token, "equipamento")
	_, popBody, errPop := integrationhubsoft.FetchCatalog(ctx, cfg, token, "pop")
	if errEq != nil || errPop != nil {
		return out
	}
	var eqDoc struct {
		Equipamentos []struct {
			IDEquipamento json.Number `json:"id_equipamento"`
			Nome          string      `json:"nome"`
			Interfaces    []struct {
				IDInterfaceConexao json.Number `json:"id_interface_conexao"`
				Nome               string      `json:"nome"`
			} `json:"interfaces"`
		} `json:"equipamentos"`
	}
	if json.Unmarshal(eqBody, &eqDoc) != nil {
		return out
	}
	equipNome := map[string]string{}
	ifaceToEquip := map[string]string{}
	ifaceNome := map[string]string{}
	for _, eq := range eqDoc.Equipamentos {
		eid := eq.IDEquipamento.String()
		equipNome[eid] = eq.Nome
		for _, iface := range eq.Interfaces {
			iid := iface.IDInterfaceConexao.String()
			ifaceToEquip[iid] = eid
			ifaceNome[iid] = iface.Nome
		}
	}
	var popDoc struct {
		Pops []struct {
			Nome         string `json:"nome"`
			Equipamentos []struct {
				IDEquipamento json.Number `json:"id_equipamento"`
			} `json:"equipamentos"`
		} `json:"pops"`
	}
	if json.Unmarshal(popBody, &popDoc) != nil {
		return out
	}
	popNomeByEquip := map[string]string{}
	for _, p := range popDoc.Pops {
		for _, eq := range p.Equipamentos {
			popNomeByEquip[eq.IDEquipamento.String()] = p.Nome
		}
	}
	for iid, eid := range ifaceToEquip {
		out[iid] = fmt.Sprintf("POP %s — equipamento %s, interface %s",
			firstNonEmptyStr(popNomeByEquip[eid], "desconhecido"), equipNome[eid], ifaceNome[iid])
	}
	return out
}

type hubsoftBulkImportValidateBody struct {
	Kind          string            `json:"kind"` // "client" | "service"
	Rows          []json.RawMessage `json:"rows"`
	CheckCatalogs bool              `json:"check_catalogs"`
}

// hubsoftBulkImportValidate — fase 1 da importação em massa: confere formato de cada linha e,
// opcionalmente, que os IDs de catálogo existem de verdade na conta. Não altera nada.
func (s *Server) hubsoftBulkImportValidate(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftBulkImportValidateBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 2000 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 2000 linhas", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 2*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	var cat integrationhubsoft.CatalogSets
	var unchecked []string
	ifacePopLabels := map[string]string{}
	if body.CheckCatalogs {
		token, err := s.hubsoftToken(ctx, integID, cfg)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
			return
		}
		cat, unchecked = s.hubsoftBulkImportCatalogSets(ctx, cfg, token, body.Kind != "service")
		ifacePopLabels = s.hubsoftBulkImportInterfacePopLabels(ctx, cfg, token)
	}

	var result integrationhubsoft.ImportValidationResult
	switch body.Kind {
	case "service":
		rows := make([]integrationhubsoft.ServiceImportRow, 0, len(body.Rows))
		for _, raw := range body.Rows {
			var row integrationhubsoft.ServiceImportRow
			_ = json.Unmarshal(raw, &row)
			rows = append(rows, row)
		}
		result = integrationhubsoft.ValidateServiceImportRows(rows, cat)
		for i, row := range rows {
			if label, ok := ifacePopLabels[row.IDInterfaceConexao]; ok {
				result.Rows[i].Info = append(result.Rows[i].Info, label)
			}
		}
	default:
		rows := make([]integrationhubsoft.ClientImportRow, 0, len(body.Rows))
		for _, raw := range body.Rows {
			var row integrationhubsoft.ClientImportRow
			_ = json.Unmarshal(raw, &row)
			rows = append(rows, row)
		}
		result = integrationhubsoft.ValidateClientImportRows(rows, cat)
		for i, row := range rows {
			if label, ok := ifacePopLabels[row.IDInterfaceConexao]; ok {
				result.Rows[i].Info = append(result.Rows[i].Info, label)
			}
		}
	}
	result.UncheckedCatalogs = unchecked
	writeJSON(w, http.StatusOK, result)
}

type hubsoftBulkImportApplyBody struct {
	Kind string            `json:"kind"`
	Rows []json.RawMessage `json:"rows"`
}

// hubsoftBulkImportApply — fase 2: cria de verdade (POST /cliente ou POST /cliente_servico), uma
// linha por vez, e grava cada resultado em ops_audit_log (entity_type "hubsoft_bulk_import_client"
// ou "hubsoft_bulk_import_service") — é esse log que alimenta o histórico da tela ("quantos e
// quais foram importados", GET /api/v1/ops/audit?entity_type=...). Lote pequeno (o front encadeia);
// para tudo depois de 3 falhas seguidas, para não insistir num problema sistémico (ex.: token caiu).
func (s *Server) hubsoftBulkImportApply(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftBulkImportApplyBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 20 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 20 linhas por chamada", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	extendWriteDeadline(w, 3*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}
	actor := s.actorFromRequest(r)
	entityType := "hubsoft_bulk_import_client"
	if body.Kind == "service" {
		entityType = "hubsoft_bulk_import_service"
	}

	results := make([]integrationhubsoft.ImportApplyResult, 0, len(body.Rows))
	halted := false
	consecErr := 0
	for i, raw := range body.Rows {
		if i > 0 {
			time.Sleep(350 * time.Millisecond) // folga sob o limite de 20 req/s da HubSoft
		}
		var res integrationhubsoft.ImportApplyResult
		var label, resLabel, afterExtra, login, senha, idInterfaceConexao string
		if body.Kind == "service" {
			var row integrationhubsoft.ServiceImportRow
			_ = json.Unmarshal(raw, &row)
			row.Line = bulkRowLine(raw, i)
			res = integrationhubsoft.ApplyServiceImportRowDedup(ctx, cfg, token, row)
			label = "id_cliente " + row.IDCliente
			resLabel = label
			if row.Login != "" {
				resLabel += " · " + row.Login
			}
			afterExtra = row.IDCliente
			login, senha, idInterfaceConexao = row.Login, row.Senha, row.IDInterfaceConexao
		} else {
			var row integrationhubsoft.ClientImportRow
			_ = json.Unmarshal(raw, &row)
			row.Line = bulkRowLine(raw, i)
			res = integrationhubsoft.ApplyClientImportRowDedup(ctx, cfg, token, row)
			label = row.NomeRazaoSocial
			resLabel = label
			afterExtra = row.CPFCNPJ
			login, senha, idInterfaceConexao = row.Login, row.Senha, row.IDInterfaceConexao
		}

		// Login PPPoE — segunda chamada (configurar_autenticacao), só depois do cliente/serviço criado
		// com sucesso. Resultado gravado em campos PRÓPRIOS (LoginOK/LoginMessage), nunca sobrescrevendo
		// OK/Message da criação — ver comentário em ApplyLoginConfig/ImportApplyResult. Pulado quando
		// DedupAction é "ja_existe": o serviço encontrado JÁ tem esse login configurado, reconfigurar
		// de novo não é "nada foi feito" (poderia até resetar a senha por engano).
		// "login_a_corrigir" é o reparo: o serviço já existia com o login padrão (troca anterior falhou).
		if res.OK && res.DedupAction != "ja_existe" && res.IDServico != "" && (login != "" || idInterfaceConexao != "") {
			time.Sleep(350 * time.Millisecond)
			loginRes := integrationhubsoft.ApplyLoginConfig(ctx, cfg, token, res.IDServico, idInterfaceConexao, login, senha)
			loginOK := loginRes.OK
			res.LoginOK = &loginOK
			res.LoginMessage = loginRes.Message
		}
		// Serviço que já existia com o login certo mas a senha padrão da máscara: corrige só a senha
		// (nunca toca em outra senha — pode ser a real do cliente). Ver RepairPlaceholderPassword.
		if res.OK && res.DedupAction == "ja_existe" && res.IDServico != "" {
			// Login e senha são sensíveis a maiúsculas/minúsculas. O reparo de caixa do login vem antes
			// do da senha; os resultados se somam (um falhar não esconde o outro).
			merge := func(rep *integrationhubsoft.LoginConfigResult) {
				if rep == nil {
					return
				}
				if res.LoginOK == nil {
					ok := rep.OK
					res.LoginOK = &ok
					res.LoginMessage = rep.Message
					return
				}
				ok := *res.LoginOK && rep.OK
				res.LoginOK = &ok
				res.LoginMessage += " | " + rep.Message
			}
			merge(integrationhubsoft.RecordLoginObservation(ctx, cfg, token, res.IDServico, login))
			if senha != "" {
				merge(integrationhubsoft.RepairPlaceholderPassword(ctx, cfg, token, res.IDServico, login, senha))
			}
		}
		res.Label = resLabel
		results = append(results, res)

		entityID := res.IDCliente
		if entityID == "" {
			entityID = res.IDServico
		}
		action := "created"
		switch {
		case res.Rejected:
			// Nunca chegou a sair um pedido para a HubSoft — a linha falhou na validação de formato
			// (ver ApplyClientImportRow/ApplyServiceImportRow). Ação própria no log para não passar a
			// impressão de que a HubSoft recusou dados que nem chegaram a ser enviados.
			action = "rejected_local"
			consecErr++
		case res.DedupAction == "login_em_uso":
			// Nada foi enviado: o login já existe na HubSoft. Não conta como falha de comunicação.
			action = "skipped_login_in_use"
		case !res.OK:
			action = "failed"
			consecErr++
		case res.DedupAction == "ja_existe":
			// Checagem de duplicidade (ver ApplyClientImportRowDedup/ApplyServiceImportRowDedup): já
			// existia cliente+serviço com esse login — nada foi criado nem alterado.
			action = "already_exists"
			consecErr = 0
		case res.DedupAction == "login_a_corrigir":
			// Serviço já existia com o login padrão — nada criado, só o login foi (ou tentou ser) corrigido.
			action = "login_repair"
			consecErr = 0
		case res.DedupAction == "servico_adicionado":
			// Cliente já existia (sem esse login) — criado só o serviço novo, não um cliente duplicado.
			action = "service_added"
			consecErr = 0
		default:
			consecErr = 0
		}
		after := map[string]any{
			"line": res.Line, "label": label, "description": label, "reference": afterExtra,
			"ok": res.OK, "rejected": res.Rejected, "message": res.Message,
			"id_cliente": res.IDCliente, "id_cliente_servico": res.IDServico,
			"dedup_action": res.DedupAction,
		}
		if res.LoginOK != nil {
			after["login_ok"] = *res.LoginOK
			after["login_message"] = res.LoginMessage
		}
		s.appendAuditLog(ctx, entityType, entityID, action, actor, nil, after)

		if consecErr >= 3 {
			halted = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "halted": halted})
}

// bulkRowLine lê o nº da linha do CSV enviado pelo front (aceita número ou texto numérico — o front já
// mandou "5" como texto, o que fazia o campo int virar 0 em silêncio). Sem número válido, cai para a
// posição na leva + 2 (cabeçalho = linha 1), igual à numeração do CSV.
func bulkRowLine(raw json.RawMessage, idx int) int {
	var l struct {
		Line json.Number `json:"line"`
	}
	if json.Unmarshal(raw, &l) == nil {
		if n, err := l.Line.Int64(); err == nil && n > 0 {
			return int(n)
		}
	}
	return idx + 2
}

type hubsoftRegistrationCheckBody struct {
	Kind string            `json:"kind"`
	Mode string            `json:"mode"` // "completa" (padrão) | "especifica"
	Rows []json.RawMessage `json:"rows"`
}

// hubsoftRegistrationCheck — "Conferência de cadastros" (só administradores, SOMENTE LEITURA): recebe
// linhas no mesmo formato do CSV de importação e compara cada campo com o que está cadastrado na
// HubSoft. O front envia em lotes pequenos (cada linha custa 1–2 consultas à HubSoft).
func (s *Server) hubsoftRegistrationCheck(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftRegistrationCheckBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 25 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 25 linhas por chamada", nil)
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
	labels := integrationhubsoft.LoadCheckLabels(ctx, cfg, token)
	mode := integrationhubsoft.ModeComplete
	if strings.EqualFold(strings.TrimSpace(body.Mode), integrationhubsoft.ModeSpecific) {
		mode = integrationhubsoft.ModeSpecific
	}
	results := make([]integrationhubsoft.RowCheck, 0, len(body.Rows))
	for i, raw := range body.Rows {
		if i > 0 {
			time.Sleep(300 * time.Millisecond) // folga sob o limite de requisições da HubSoft
		}
		if body.Kind == "service" {
			var row integrationhubsoft.ServiceImportRow
			_ = json.Unmarshal(raw, &row)
			row.Line = bulkRowLine(raw, i)
			results = append(results, integrationhubsoft.CheckServiceRowMode(ctx, cfg, token, row, labels, mode))
		} else {
			var row integrationhubsoft.ClientImportRow
			_ = json.Unmarshal(raw, &row)
			row.Line = bulkRowLine(raw, i)
			results = append(results, integrationhubsoft.CheckClientRowMode(ctx, cfg, token, row, labels, mode))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "catalogs_missing": labels.Missing})
}

// hubsoftBulkImportPreflight — pré-conferência (somente leitura) das linhas de "clientes novos": o que a
// importação vai fazer com cada uma (criar, adicionar serviço a cliente existente, login em uso...).
func (s *Server) hubsoftBulkImportPreflight(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftRegistrationCheckBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "corpo inválido", nil)
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 25 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie de 1 a 25 linhas por chamada", nil)
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
	results := make([]integrationhubsoft.PreflightRow, 0, len(body.Rows))
	for i, raw := range body.Rows {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		var row integrationhubsoft.ClientImportRow
		_ = json.Unmarshal(raw, &row)
		row.Line = bulkRowLine(raw, i)
		results = append(results, integrationhubsoft.PreflightClientRow(ctx, cfg, token, row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

type hubsoftLoginObservationBody struct {
	Rows []struct {
		IDClienteServico string `json:"id_cliente_servico"`
		Login            string `json:"login"`
	} `json:"rows"`
}

// hubsoftLoginObservations — registra, em lote, "Login PPPoE configurado no cliente: <login original>" nas
// Observações da autenticação dos serviços cuja HubSoft padronizou o login em minúsculas. Só age em serviço
// cujo login atual difere do informado SÓ na caixa e que ainda não tem o texto; preserva observações existentes.
func (s *Server) hubsoftLoginObservations(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftLoginObservationBody
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
	type outRow struct {
		IDClienteServico string `json:"id_cliente_servico"`
		Login            string `json:"login"`
		OK               bool   `json:"ok"`
		Skipped          bool   `json:"skipped"`
		Message          string `json:"message"`
	}
	results := make([]outRow, 0, len(body.Rows))
	actor := s.actorFromRequest(r)
	for i, row := range body.Rows {
		if i > 0 {
			time.Sleep(350 * time.Millisecond)
		}
		id, login := strings.TrimSpace(row.IDClienteServico), strings.TrimSpace(row.Login)
		out := outRow{IDClienteServico: id, Login: login}
		rec := integrationhubsoft.RecordLoginObservation(ctx, cfg, token, id, login)
		switch {
		case rec == nil:
			out.Skipped, out.OK = true, true
			out.Message = "nada a registrar (o login na HubSoft já é igual ao original, ou o serviço não foi encontrado)"
		default:
			out.OK, out.Message = rec.OK, rec.Message
			if rec.OK {
				s.appendAuditLog(ctx, "hubsoft_client_service", id, "login_observation", actor, nil, map[string]any{"integration_id": integID.String(), "login": login})
			}
		}
		results = append(results, out)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
