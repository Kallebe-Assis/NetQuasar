package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

// affectedClientRow uma ONU afetada com cliente vinculado (ver loadAffectedClients).
type affectedClientRow struct {
	Serial     string `json:"serial"`
	ClientName string `json:"client_name"`
	Pon        int    `json:"pon"`
	Onu        int    `json:"onu"`
}

type affectedClientsResult struct {
	OltLabel      string
	ScopeLabel    string
	OnuCount      int
	Clients       []affectedClientRow
	UnlinkedCount int
}

// affectedClientsAPIErr erro já classificado (status HTTP + código) — devolvido por
// loadAffectedClients para que cada handler (listar/enviar) decida como reportar.
type affectedClientsAPIErr struct {
	status int
	code   string
	msg    string
}

func (e *affectedClientsAPIErr) Error() string { return e.msg }

// loadAffectedClients reúne as ONUs afetadas por um alerta de OLT offline (ping_unreachable)
// ou PON DOWN (pon_down) — a OLT inteira ou só a PON caída — e cruza com onu_client_links (pelo
// serial) para saber quais têm cliente vinculado. Partilhado pelo endpoint de listagem (modal
// "Clientes afetados") e pelo de envio por Telegram, para as duas vias nunca divergirem.
func (s *Server) loadAffectedClients(ctx context.Context, alertID uuid.UUID) (*affectedClientsResult, *affectedClientsAPIErr) {
	var deviceID uuid.UUID
	var alertType, oltLabel, category string
	var metaRaw []byte
	err := s.DB().QueryRow(ctx, `
		SELECT a.device_id, a.alert_type, COALESCE(a.meta::text, '{}'),
			COALESCE(NULLIF(trim(a.device_name), ''), NULLIF(trim(d.description), ''), 'OLT'),
			COALESCE(NULLIF(trim(d.category), ''), '')
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		WHERE a.id = $1
	`, alertID).Scan(&deviceID, &alertType, &metaRaw, &oltLabel, &category)
	if err != nil {
		return nil, &affectedClientsAPIErr{http.StatusNotFound, "NOT_FOUND", "Alerta não encontrado."}
	}
	if alertType != "pon_down" && alertType != "ping_unreachable" {
		return nil, &affectedClientsAPIErr{http.StatusBadRequest, "VALIDATION", "«Clientes afetados» só está disponível para alertas de OLT offline ou PON DOWN."}
	}
	if !strings.EqualFold(strings.TrimSpace(category), "olt") {
		return nil, &affectedClientsAPIErr{http.StatusBadRequest, "VALIDATION", "Este alerta não é de uma OLT."}
	}

	var ponFilter string
	scopeLabel := "toda a OLT"
	if alertType == "pon_down" {
		var meta map[string]any
		_ = json.Unmarshal(metaRaw, &meta)
		ponFilter = strings.TrimSpace(fmt.Sprint(meta["pon"]))
		if name := strings.TrimSpace(fmt.Sprint(meta["pon_name"])); name != "" && name != "<nil>" {
			scopeLabel = fmt.Sprintf("PON %s (%s)", ponFilter, name)
		} else {
			scopeLabel = "PON " + ponFilter
		}
		if ponFilter == "" {
			return nil, &affectedClientsAPIErr{http.StatusBadRequest, "VALIDATION", "Alerta sem PON identificada no meta — não é possível filtrar."}
		}
	}

	var sumRaw []byte
	if err := s.DB().QueryRow(ctx, `SELECT summary::text FROM olt_snapshots WHERE device_id = $1`, deviceID).Scan(&sumRaw); err != nil {
		return nil, &affectedClientsAPIErr{http.StatusBadGateway, "NO_SNAPSHOT", "Sem snapshot recente desta OLT — atualize a OLT e tente de novo."}
	}
	onuRows := vsolparse.VsolOnuRowsFromSummaryBlob(sumRaw)
	if len(onuRows) == 0 {
		return nil, &affectedClientsAPIErr{http.StatusBadGateway, "NO_ONU_DATA", "Sem tabela de ONUs no snapshot desta OLT (só disponível para OLTs VSOL)."}
	}

	// serial tal como aparece no snapshot (SNMP/telnet) não tem case consistente — já visto em
	// produção "0000b11d1cc6" ao lado de "0000B11D1CFF" na mesma OLT — enquanto onu_client_links
	// grava sempre em maiúsculas (ver importOnuClientLinks/deleteOnuClientLink). Comparar sem
	// normalizar fazia o match falhar silenciosamente e reportar "nenhum cliente vinculado"
	// mesmo com o vínculo a existir (bug real reportado pelo utilizador).
	type onuRef struct {
		pon, onu int
	}
	ponByUpperSerial := map[string]onuRef{}
	serialsUpper := make([]string, 0, len(onuRows))
	onuCount := 0
	for _, it := range onuRows {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ponIdx := onuAffectedIntVal(m["pon"])
		if ponFilter != "" {
			if ponIdx < 1 || oltifderive.VsolMibPonCompactID(ponIdx) != ponFilter {
				continue
			}
		}
		serial := strings.ToUpper(strings.TrimSpace(fmt.Sprint(m["serial"])))
		if serial == "" || serial == "<NIL>" {
			continue
		}
		onuCount++
		ponByUpperSerial[serial] = onuRef{pon: ponIdx, onu: onuAffectedIntVal(m["onu"])}
		serialsUpper = append(serialsUpper, serial)
	}
	if onuCount == 0 {
		return nil, &affectedClientsAPIErr{http.StatusUnprocessableEntity, "NO_ONUS", fmt.Sprintf("Nenhuma ONU encontrada em %s.", scopeLabel)}
	}

	linkedNames, err := clientNamesForSerials(ctx, s.DB(), serialsUpper)
	if err != nil {
		return nil, &affectedClientsAPIErr{http.StatusInternalServerError, "DB", err.Error()}
	}

	clients := make([]affectedClientRow, 0, len(linkedNames))
	for serial, name := range linkedNames {
		ref := ponByUpperSerial[serial]
		clients = append(clients, affectedClientRow{Serial: serial, ClientName: name, Pon: ref.pon, Onu: ref.onu})
	}
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].ClientName != clients[j].ClientName {
			return clients[i].ClientName < clients[j].ClientName
		}
		return clients[i].Serial < clients[j].Serial
	})

	return &affectedClientsResult{
		OltLabel:      oltLabel,
		ScopeLabel:    scopeLabel,
		OnuCount:      onuCount,
		Clients:       clients,
		UnlinkedCount: onuCount - len(clients),
	}, nil
}

// alertAffectedClientsList — GET, usado pelo modal "Clientes afetados" (3 pontinhos, alertas de
// OLT offline ou PON DOWN). Ao contrário do envio por Telegram, uma lista vazia não é erro: o
// modal mostra o estado "nenhum cliente vinculado" normalmente.
func (s *Server) alertAffectedClientsList(w http.ResponseWriter, r *http.Request) {
	alertID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	res, aerr := s.loadAffectedClients(r.Context(), alertID)
	if aerr != nil {
		writeErr(w, aerr.status, aerr.code, aerr.msg, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"olt_label":      res.OltLabel,
		"scope_label":    res.ScopeLabel,
		"onu_count":      res.OnuCount,
		"clients":        res.Clients,
		"unlinked_count": res.UnlinkedCount,
	})
}

// alertAffectedClientsTelegram — POST, botão "Enviar por Telegram" dentro do modal "Clientes
// afetados": reenvia o mesmo conjunto calculado por loadAffectedClients (ver ali) pelo bot de
// monitorização — pedido explícito do utilizador. Só cobre OLTs VSOL (única fonte com
// serial+PON por ONU nesta versão, ver internal/vsolparse) — outros vendors não têm
// vsol_onu_rows no snapshot.
func (s *Server) alertAffectedClientsTelegram(w http.ResponseWriter, r *http.Request) {
	alertID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	ctx := r.Context()
	res, aerr := s.loadAffectedClients(ctx, alertID)
	if aerr != nil {
		writeErr(w, aerr.status, aerr.code, aerr.msg, nil)
		return
	}
	if len(res.Clients) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "NO_CLIENTS", fmt.Sprintf("Nenhuma das %d ONU(s) em %s tem cliente vinculado.", res.OnuCount, res.ScopeLabel), nil)
		return
	}

	cfg, err := telegramclient.LoadConfig(ctx, s.DB(), "monitoring")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !cfg.Ready() {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "Telegram de monitorização não configurado (bot_token/chat_id) — configure em Configurações → Telegram.", nil)
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 CLIENTES AFETADOS\n")
	sb.WriteString(fmt.Sprintf("%s — %s\n\n", res.OltLabel, res.ScopeLabel))
	for i, c := range res.Clients {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c.ClientName))
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d cliente(s) — %d ONU(s) sem vínculo não entraram na lista.", len(res.Clients), res.UnlinkedCount))

	if err := telegramclient.SendMessageChunks(ctx, cfg, sb.String()); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "client_count": len(res.Clients), "onu_count": res.OnuCount})
}

// clientNamesForSerials devolve serial (maiúsculas) -> nome do cliente, para os seriais dados
// (já normalizados para maiúsculas pelo chamador — onu_client_links só grava em maiúsculas).
func clientNamesForSerials(ctx context.Context, pool *pgxpool.Pool, serialsUpper []string) (map[string]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT serial, client_name FROM onu_client_links WHERE serial = ANY($1)
	`, serialsUpper)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var serial, name string
		if err := rows.Scan(&serial, &name); err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[strings.ToUpper(strings.TrimSpace(serial))] = name
	}
	return out, rows.Err()
}

func onuAffectedIntVal(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
