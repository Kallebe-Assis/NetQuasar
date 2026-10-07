package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
)

// POST /olt/devices/{id}/pons/{pon}/refresh — atualiza só as ONUs de UMA PON, com o comando telnet configurado no
// perfil da OLT (Configurações → OLT → "Atualizar ONUs de uma PON"). Não refaz a coleta da OLT inteira: lê a listagem
// da porta, funde serial/modelo/estado nas ONUs dessa PON do snapshot e acerta os totais.
func (s *Server) refreshOLTPon(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 3*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	pon, perr := strconv.Atoi(chi.URLParam(r, "pon"))
	if perr != nil || pon <= 0 || pon > 512 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "informe o número da PON", nil)
		return
	}
	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "credenciais telnet") || strings.Contains(err.Error(), "sem IP") {
			writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !sess.Cfg.HasPonRefresh() {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure o comando \"Atualizar ONUs de uma PON\" no perfil "+sess.Brand+" / "+sess.Model+" (Configurações → OLT).", nil)
		return
	}
	if sess.Cfg.PonRefreshNeedsEnable() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP para os pré-comandos deste perfil", nil)
		return
	}

	// Mesma exclusão mútua das coletas: não abre telnet enquanto a OLT está sendo coletada.
	unlock, ok := snmpdevicelock.TryAcquire(ctx, id, 8*time.Second)
	if !ok {
		writeErr(w, http.StatusConflict, "BUSY", "a OLT está sendo coletada agora — tente de novo em instantes", nil)
		return
	}
	defer unlock()

	telCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	run := oltcollect.RunPonRefreshTelnet(telCtx, sess.Host, sess.User, sess.Password, sess.Enable,
		sess.Cfg, oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}, pon, 80*time.Second)

	out := map[string]any{
		"ok": run.OK, "olt_id": id, "olt_description": sess.Desc, "pon": pon,
		"commands": run.Commands, "output": trimOutput(run.Output, 6000),
	}
	if !run.OK {
		out["error"] = run.Error
		writeJSON(w, http.StatusOK, out)
		return
	}

	stats, err := s.applyPonRefresh(ctx, id, pon, run.Entries)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out["stats"] = stats
	s.appendAuditLog(ctx, "olt_snapshot", id.String(), "pon_refresh", s.actorFromRequest(r), nil,
		map[string]any{"pon": pon, "entries": stats.Entries, "added": stats.Added, "serials_filled": stats.SerialsFilled, "state_changed": stats.StateChanged})
	writeJSON(w, http.StatusOK, out)
}

// applyPonRefresh funde as ONUs lidas no snapshot da OLT (summary + pons) numa transação, com a linha bloqueada para
// não perder uma gravação feita por uma coleta ao mesmo tempo.
func (s *Server) applyPonRefresh(ctx context.Context, id uuid.UUID, pon int, entries []oltcollect.PonRefreshEntry) (oltcollect.PonRefreshMergeStats, error) {
	var stats oltcollect.PonRefreshMergeStats
	tx, err := s.DB().Begin(ctx)
	if err != nil {
		return stats, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sumRaw, ponsRaw []byte
	err = tx.QueryRow(ctx, `SELECT COALESCE(summary::text,'{}'), COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1 FOR UPDATE`, id).Scan(&sumRaw, &ponsRaw)
	if err != nil && err != pgx.ErrNoRows {
		return stats, err
	}
	summary := map[string]any{}
	if len(sumRaw) > 0 {
		_ = json.Unmarshal(sumRaw, &summary)
	}
	if summary == nil {
		summary = map[string]any{}
	}
	var pons []map[string]any
	if len(ponsRaw) > 0 {
		_ = json.Unmarshal(ponsRaw, &pons)
	}

	stats = oltcollect.MergePonRefreshIntoSummary(summary, pon, entries, time.Now())
	oltcollect.ApplyPonRefreshToPons(pons, pon, stats)
	oltcollect.SanitizeOnuSerialsMap(summary)

	sb, _ := json.Marshal(summary)
	if pons == nil {
		pons = []map[string]any{}
	}
	pb, _ := json.Marshal(pons)
	if _, err := tx.Exec(ctx, `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, $3::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET summary = excluded.summary, pons = excluded.pons, updated_at = now()
	`, id, sb, pb); err != nil {
		return stats, err
	}
	return stats, tx.Commit(ctx)
}

func trimOutput(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n… (saída cortada)"
}

// oltPonRefreshConfigured informa se o perfil (marca/modelo) da OLT tem o comando de atualização por PON.
func (s *Server) oltPonRefreshConfigured(ctx context.Context, deviceID uuid.UUID) bool {
	var brand, model string
	if err := s.DB().QueryRow(ctx, `SELECT coalesce(trim(brand),''), coalesce(trim(model),'') FROM devices WHERE id=$1`, deviceID).Scan(&brand, &model); err != nil {
		return false
	}
	var raw []byte
	if err := s.DB().QueryRow(ctx, `
		SELECT coalesce(onu_report_commands::text,'{}') FROM olt_vendor_models
		WHERE upper(trim(brand)) = upper(trim($1)) AND upper(trim(model)) = upper(trim($2))
	`, brand, model).Scan(&raw); err != nil {
		return false
	}
	return oltcollect.ParseOnuReportConfig(raw).HasPonRefresh()
}
