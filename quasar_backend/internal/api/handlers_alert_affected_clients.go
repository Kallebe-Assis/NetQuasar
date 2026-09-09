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

// alertAffectedClientsTelegram — botão "Clientes afetados" (3 pontinhos, alertas de OLT offline
// ou PON DOWN): reúne as ONUs da OLT inteira (ping_unreachable) ou só da PON que caiu (pon_down),
// filtra pelas que têm um cliente vinculado (onu_client_links, pelo serial) e manda os nomes por
// Telegram — pedido explícito do utilizador. Só cobre OLTs VSOL (única fonte com serial+PON por
// ONU nesta versão, ver internal/vsolparse) — outros vendors não têm vsol_onu_rows no snapshot.
func (s *Server) alertAffectedClientsTelegram(w http.ResponseWriter, r *http.Request) {
	alertID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	ctx := r.Context()

	var deviceID uuid.UUID
	var alertType, oltLabel, category string
	var metaRaw []byte
	err = s.DB().QueryRow(ctx, `
		SELECT a.device_id, a.alert_type, COALESCE(a.meta::text, '{}'),
			COALESCE(NULLIF(trim(a.device_name), ''), NULLIF(trim(d.description), ''), 'OLT'),
			COALESCE(NULLIF(trim(d.category), ''), '')
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		WHERE a.id = $1
	`, alertID).Scan(&deviceID, &alertType, &metaRaw, &oltLabel, &category)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "Alerta não encontrado.", nil)
		return
	}
	if alertType != "pon_down" && alertType != "ping_unreachable" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "«Clientes afetados» só está disponível para alertas de OLT offline ou PON DOWN.", nil)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(category), "olt") {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "Este alerta não é de uma OLT.", nil)
		return
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
			writeErr(w, http.StatusBadRequest, "VALIDATION", "Alerta sem PON identificada no meta — não é possível filtrar.", nil)
			return
		}
	}

	var sumRaw []byte
	if err := s.DB().QueryRow(ctx, `SELECT summary::text FROM olt_snapshots WHERE device_id = $1`, deviceID).Scan(&sumRaw); err != nil {
		writeErr(w, http.StatusBadGateway, "NO_SNAPSHOT", "Sem snapshot recente desta OLT — atualize a OLT e tente de novo.", nil)
		return
	}
	onuRows := vsolparse.VsolOnuRowsFromSummaryBlob(sumRaw)
	if len(onuRows) == 0 {
		writeErr(w, http.StatusBadGateway, "NO_ONU_DATA", "Sem tabela de ONUs no snapshot desta OLT (só disponível para OLTs VSOL).", nil)
		return
	}

	serials := make([]string, 0, len(onuRows))
	for _, it := range onuRows {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if ponFilter != "" {
			ponIdx := onuAffectedIntVal(m["pon"])
			if ponIdx < 1 || oltifderive.VsolMibPonCompactID(ponIdx) != ponFilter {
				continue
			}
		}
		serial := strings.TrimSpace(fmt.Sprint(m["serial"]))
		if serial == "" || serial == "<nil>" {
			continue
		}
		serials = append(serials, serial)
	}
	if len(serials) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "NO_ONUS", fmt.Sprintf("Nenhuma ONU encontrada em %s.", scopeLabel), nil)
		return
	}

	names, err := clientNamesForSerials(ctx, s.DB(), serials)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(names) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "NO_CLIENTS", fmt.Sprintf("Nenhuma das %d ONU(s) em %s tem cliente vinculado.", len(serials), scopeLabel), nil)
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
	sb.WriteString(fmt.Sprintf("%s — %s\n\n", oltLabel, scopeLabel))
	for i, n := range names {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, n))
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d cliente(s) — %d ONU(s) sem vínculo não entraram na lista.", len(names), len(serials)-len(names)))

	if err := telegramclient.SendMessageChunks(ctx, cfg, sb.String()); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "client_count": len(names), "onu_count": len(serials)})
}

func clientNamesForSerials(ctx context.Context, pool *pgxpool.Pool, serials []string) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT client_name FROM onu_client_links WHERE serial = ANY($1) ORDER BY client_name
	`, serials)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, rows.Err()
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
