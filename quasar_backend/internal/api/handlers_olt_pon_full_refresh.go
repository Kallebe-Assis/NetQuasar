package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
)

// Atualização COMPLETA de uma PON: (1) serial/modelo/estado pelo comando "Atualizar ONUs de uma PON" do perfil (quando
// existe) e (2) RX/TX/temperatura de cada ONU online, UMA A UMA (algumas OLTs não têm comando que traga todas de uma
// vez). Roda em segundo plano no servidor — o utilizador pode sair da tela — e cada lote lido é gravado no snapshot.

type ponFullJob struct {
	mu         sync.Mutex
	deviceID   uuid.UUID
	pon        int
	cancel     context.CancelFunc
	running    bool
	phase      string
	total      int
	done       int
	okCount    int
	errCount   int
	skipped    int
	backOnline int
	current    string
	message    string
	errText    string
	warnings   []string
	startedAt  time.Time
	finishedAt time.Time
	cancelled  bool
	ponStats   *oltcollect.PonRefreshMergeStats
}

var ponFullJobs sync.Map // "deviceID:pon" -> *ponFullJob

func ponFullJobKey(id uuid.UUID, pon int) string { return id.String() + ":" + strconv.Itoa(pon) }

func (j *ponFullJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := map[string]any{
		"running": j.running, "phase": j.phase, "pon": j.pon, "total": j.total, "done": j.done,
		"ok": j.okCount, "errors": j.errCount, "skipped_offline": j.skipped, "back_online": j.backOnline, "current": j.current,
		"message": j.message, "cancelled": j.cancelled,
		"started_at": j.startedAt.UTC().Format(time.RFC3339),
	}
	if j.errText != "" {
		out["error"] = j.errText
	}
	if len(j.warnings) > 0 {
		out["warnings"] = j.warnings
	}
	if !j.finishedAt.IsZero() {
		out["finished_at"] = j.finishedAt.UTC().Format(time.RFC3339)
	}
	if j.ponStats != nil {
		out["pon_stats"] = j.ponStats
	}
	return out
}

func (j *ponFullJob) set(fn func()) {
	j.mu.Lock()
	fn()
	j.mu.Unlock()
}

// POST /olt/devices/{id}/pons/{pon}/refresh-full — inicia (ou devolve o estado de) a atualização completa da PON.
func (s *Server) startOLTPonFullRefresh(w http.ResponseWriter, r *http.Request) {
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
	if !sess.Cfg.HasCommands() {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure os comandos telnet de relatório ONU em Definições → Perfis OLT para "+sess.Brand+" / "+sess.Model, nil)
		return
	}
	if (sess.Cfg.NeedsEnablePassword() || (sess.Cfg.HasPonRefresh() && sess.Cfg.PonRefreshNeedsEnable())) && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP para os pré-comandos deste perfil", nil)
		return
	}

	key := ponFullJobKey(id, pon)
	if v, ok := ponFullJobs.Load(key); ok {
		if old := v.(*ponFullJob); func() bool { old.mu.Lock(); defer old.mu.Unlock(); return old.running }() {
			out := old.snapshot()
			out["already_running"] = true
			writeJSON(w, http.StatusOK, out)
			return
		}
	}

	jobCtx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	job := &ponFullJob{deviceID: id, pon: pon, cancel: cancel, running: true, phase: "iniciando", startedAt: time.Now()}
	ponFullJobs.Store(key, job)
	actor := s.actorFromRequest(r)
	go func() {
		defer cancel()
		s.runPonFullRefresh(jobCtx, job, sess, actor)
	}()
	writeJSON(w, http.StatusOK, job.snapshot())
}

// GET /olt/devices/{id}/pons/{pon}/refresh-full — estado do último/atual trabalho dessa PON.
func (s *Server) getOLTPonFullRefresh(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	pon, perr := strconv.Atoi(chi.URLParam(r, "pon"))
	if err != nil || perr != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	v, ok := ponFullJobs.Load(ponFullJobKey(id, pon))
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"running": false, "idle": true, "pon": pon})
		return
	}
	writeJSON(w, http.StatusOK, v.(*ponFullJob).snapshot())
}

// DELETE /olt/devices/{id}/pons/{pon}/refresh-full — interrompe (o que já foi lido fica gravado).
func (s *Server) cancelOLTPonFullRefresh(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	pon, perr := strconv.Atoi(chi.URLParam(r, "pon"))
	if err != nil || perr != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	v, ok := ponFullJobs.Load(ponFullJobKey(id, pon))
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"running": false, "idle": true, "pon": pon})
		return
	}
	job := v.(*ponFullJob)
	job.set(func() { job.cancelled = true })
	job.cancel()
	writeJSON(w, http.StatusOK, job.snapshot())
}

func (s *Server) runPonFullRefresh(ctx context.Context, job *ponFullJob, sess oltTelnetSession, actor string) {
	id, pon := job.deviceID, job.pon
	finish := func(msg, errText string) {
		job.set(func() {
			job.running = false
			job.phase = "concluído"
			job.current = ""
			job.finishedAt = time.Now()
			if errText != "" {
				job.errText = errText
				job.phase = "erro"
			}
			job.message = msg
		})
	}

	unlock, ok := snmpdevicelock.TryAcquire(ctx, id, 20*time.Second)
	if !ok {
		finish("", "a OLT está sendo coletada agora — tente de novo em instantes")
		return
	}
	defer unlock()

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}

	// 1) serial / modelo / estado (comando da PON) — opcional.
	if sess.Cfg.HasPonRefresh() {
		job.set(func() { job.phase = "serial e status das ONUs" })
		telCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		run := oltcollect.RunPonRefreshTelnet(telCtx, sess.Host, sess.User, sess.Password, sess.Enable, sess.Cfg, secrets, pon, 80*time.Second)
		cancel()
		if !run.OK {
			job.set(func() { job.warnings = append(job.warnings, "serial/status não atualizados: "+run.Error) })
		} else if stats, aerr := s.applyPonRefresh(ctx, id, pon, run.Entries); aerr != nil {
			job.set(func() { job.warnings = append(job.warnings, "serial/status não gravados: "+aerr.Error()) })
		} else {
			job.set(func() { job.ponStats = &stats })
			s.appendAuditLog(ctx, "olt_snapshot", id.String(), "pon_refresh", actor, nil,
				map[string]any{"pon": pon, "entries": stats.Entries, "added": stats.Added, "serials_filled": stats.SerialsFilled, "state_changed": stats.StateChanged, "origem": "atualizar PON inteira"})
		}
	}
	if ctx.Err() != nil {
		finish("interrompido", "")
		return
	}

	// 2) Relatório ONU a ONU: primeiro as online (serial, RX, TX, temperatura…); depois as offline, só para CONFIRMAR o
	// status — se continuarem offline não ganham leitura nenhuma; se a OLT disser que voltaram, gravam tudo como as online.
	rows, lerr := s.loadPonOnuRows(ctx, id, pon)
	if lerr != nil {
		finish("", "falha ao ler o snapshot da OLT: "+lerr.Error())
		return
	}
	var todo, offlineRows []map[string]any
	offlineKeys := map[int]bool{}
	for _, r := range rows {
		if b, ok := r["online"].(bool); ok && !b {
			offlineRows = append(offlineRows, r)
			if n, ok := r["onu"].(float64); ok {
				offlineKeys[int(n)] = true
			}
			continue
		}
		todo = append(todo, r)
	}
	todo = append(todo, offlineRows...)
	job.set(func() {
		job.phase = "RX de cada ONU"
		job.total = len(todo)
		job.skipped = len(offlineRows)
	})
	if len(todo) == 0 {
		finish("nenhuma ONU encontrada nessa PON", "")
		return
	}

	hooks := oltcollect.PonRxRefreshHooks{
		Progress: func(done, total int, current string) {
			job.set(func() { job.done = done; job.current = current })
		},
		Flush: func(batch []oltcollect.OnuReportResult) error {
			_, ferr := s.applyOnuReportResults(ctx, id, batch)
			return ferr
		},
	}
	creds := oltcollect.TelnetCredentials{User: sess.User, Password: sess.Password, Enable: sess.Enable}
	okN, errN, rerr := oltcollect.RefreshPonOnuReports(ctx, sess.Host, creds, sess.Cfg, todo, hooks)
	job.set(func() { job.okCount = okN; job.errCount = errN; job.done = okN + errN })
	s.appendAuditLog(context.Background(), "olt_snapshot", id.String(), "pon_full_refresh", actor, nil,
		map[string]any{"pon": pon, "onus_lidas": okN, "falhas": errN, "offline_verificadas": len(offlineRows)})

	// Quantas das que estavam offline voltaram a ficar online depois da confirmação.
	backOnline := 0
	if after, aerr := s.loadPonOnuRows(context.Background(), id, pon); aerr == nil {
		for _, r := range after {
			if n, ok := r["onu"].(float64); ok && offlineKeys[int(n)] {
				if b, ok := r["online"].(bool); ok && b {
					backOnline++
				}
			}
		}
	}
	job.set(func() { job.skipped = len(offlineRows) - backOnline; job.backOnline = backOnline })
	extra := fmt.Sprintf(" · %d offline confirmada(s), %d voltou(aram) online", len(offlineRows)-backOnline, backOnline)

	switch {
	case rerr != nil:
		finish(fmt.Sprintf("%d ONU(s) lida(s) antes da falha", okN), rerr.Error())
	case ctx.Err() != nil && job.cancelled:
		finish(fmt.Sprintf("interrompido — %d ONU(s) lida(s), já gravadas", okN), "")
	case ctx.Err() != nil:
		finish(fmt.Sprintf("tempo esgotado — %d ONU(s) lida(s), já gravadas", okN), "")
	default:
		finish(fmt.Sprintf("%d ONU(s) lida(s), %d sem resposta%s", okN, errN, extra), "")
	}
}

// loadPonOnuRows devolve as linhas de ONU de uma PON a partir do snapshot atual.
func (s *Server) loadPonOnuRows(ctx context.Context, id uuid.UUID, pon int) ([]map[string]any, error) {
	var sumRaw []byte
	err := s.DB().QueryRow(ctx, `SELECT COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1`, id).Scan(&sumRaw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	summary := map[string]any{}
	_ = json.Unmarshal(sumRaw, &summary)
	var out []map[string]any
	for _, r := range oltcollect.OnuRowsFromSummary(summary) {
		if p, ok := r["pon"].(float64); ok && int(p) == pon {
			out = append(out, r)
		}
	}
	return out, nil
}

// applyOnuReportResults grava leituras telnet por ONU no snapshot (transação com a linha bloqueada, como applyPonRefresh).
func (s *Server) applyOnuReportResults(ctx context.Context, id uuid.UUID, results []oltcollect.OnuReportResult) (int, error) {
	tx, err := s.DB().Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sumRaw []byte
	err = tx.QueryRow(ctx, `SELECT COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1 FOR UPDATE`, id).Scan(&sumRaw)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	summary := map[string]any{}
	if len(sumRaw) > 0 {
		_ = json.Unmarshal(sumRaw, &summary)
	}
	if summary == nil {
		return 0, nil
	}
	n := oltcollect.ApplyOnuReportResults(summary, results, time.Now())
	if n == 0 {
		return 0, nil
	}
	sb, _ := json.Marshal(summary)
	if _, err := tx.Exec(ctx, `UPDATE olt_snapshots SET summary=$2::jsonb, updated_at=now() WHERE device_id=$1`, id, sb); err != nil {
		return 0, err
	}
	return n, tx.Commit(ctx)
}
