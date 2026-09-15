package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/buildinfo"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
)

// systemUpdateBranch é a branch acompanhada para atualizações — igual à branch principal do
// repositório (ver git remote/status: "Main branch (você vai usar para PRs): main").
const systemUpdateBranch = "main"

type systemUpdateStateOut struct {
	Status                 string          `json:"status"`
	BaseCommit             *string         `json:"base_commit,omitempty"`
	Branch                 string          `json:"branch"`
	RequestedAt            *time.Time      `json:"requested_at,omitempty"`
	RequestedBy            *string         `json:"requested_by,omitempty"`
	PreviousIsRunning      *bool           `json:"previous_is_running,omitempty"`
	PreviousMonitoringMode *string         `json:"previous_monitoring_mode,omitempty"`
	CheckResult            json.RawMessage `json:"check_result,omitempty"`
	StartedAt              *time.Time      `json:"started_at,omitempty"`
	CompletedAt            *time.Time      `json:"completed_at,omitempty"`
	ErrorMessage           *string         `json:"error_message,omitempty"`
}

func (s *Server) loadSystemUpdateState(ctx context.Context) (systemUpdateStateOut, error) {
	var out systemUpdateStateOut
	var checkResult []byte
	err := s.DB().QueryRow(ctx, `
		SELECT status, base_commit, branch, requested_at, requested_by,
			previous_is_running, previous_monitoring_mode, check_result::text,
			started_at, completed_at, error_message
		FROM system_update_state WHERE id = 1
	`).Scan(&out.Status, &out.BaseCommit, &out.Branch, &out.RequestedAt, &out.RequestedBy,
		&out.PreviousIsRunning, &out.PreviousMonitoringMode, &checkResult,
		&out.StartedAt, &out.CompletedAt, &out.ErrorMessage)
	if err != nil {
		return systemUpdateStateOut{}, err
	}
	if len(checkResult) > 0 {
		out.CheckResult = json.RawMessage(checkResult)
	}
	return out, nil
}

// systemVersion devolve o commit em que este binário foi compilado (ver internal/buildinfo,
// preenchido via -ldflags no Dockerfile) mais o estado actual de uma verificação/atualização em
// curso — é o endpoint que o frontend faz poll enquanto uma operação está a decorrer.
func (s *Server) systemVersion(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.view", "settings.system_update") {
		return
	}
	state, err := s.loadSystemUpdateState(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running_commit":       buildinfo.GitCommit,
		"running_commit_short": buildinfo.GitCommitShort,
		"build_time":           buildinfo.BuildTime,
		"branch":               systemUpdateBranch,
		"update_state":         state,
	})
}

// systemVersionCheck pede ao serviço `updater` (fora deste container — ver docker-compose.yml)
// para comparar o commit em execução com a ponta da branch no GitHub. Só grava o pedido; quem
// executa é o loop de polling do `updater`, que grava o resultado de volta em check_result.
func (s *Server) systemVersionCheck(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.system_update") {
		return
	}
	ctx := r.Context()
	actor := s.actorFromRequest(r)
	ct, err := s.DB().Exec(ctx, `
		UPDATE system_update_state SET
			status = 'check_requested',
			base_commit = $1,
			branch = $2,
			requested_at = now(),
			requested_by = $3,
			error_message = NULL,
			updated_at = now()
		WHERE id = 1
	`, buildinfo.GitCommit, systemUpdateBranch, actor)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusInternalServerError, "DB", "system_update_state row missing", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "check_requested"})
}

// systemUpdate pede a atualização completa (git pull + rebuild + restart, feita pelo serviço
// `updater`). Este handler NUNCA espera pelo resultado — o processo que o está a atender vai ser
// morto a meio do caminho pelo próprio `docker compose up -d --build netquasar` que o `updater`
// dispara. Por isso: pára o monitoramento (mesmo efeito de POST /monitoring/stop, mas guardando
// o estado anterior para o restaurar depois), grava o pedido, e responde logo. Quem confirma
// "terminei" é o binário NOVO, no arranque — ver internal/bootstrap.ResumeAfterUpdate.
func (s *Server) systemUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.system_update") {
		return
	}
	ctx := r.Context()

	var wasRunning bool
	var monitoringMode string
	if err := s.DB().QueryRow(ctx, `
		SELECT is_running, COALESCE(NULLIF(TRIM(monitoring_mode), ''), 'off') FROM monitoring_runtime WHERE id=1
	`).Scan(&wasRunning, &monitoringMode); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	if wasRunning {
		if _, err := s.DB().Exec(ctx, `
			UPDATE monitoring_runtime SET
				is_running = false,
				monitoring_mode = 'off',
				offer_snmp_inventory_refresh = false,
				last_stopped_at = now(),
				updated_at = now()
			WHERE id = 1
		`); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		s.setMonitoringActivity(ctx, "")
		monitorworker.EndActiveRun()

		// Folga curta e best-effort: dá chance a um ciclo em curso terminar sozinho antes do
		// container ser derrubado — não bloqueia além disto (EndActiveRun já cancelou o contexto
		// activo, então isto normalmente resolve em bem menos que o timeout).
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			var activity *string
			if err := s.DB().QueryRow(ctx, `SELECT current_activity FROM monitoring_runtime WHERE id=1`).Scan(&activity); err != nil {
				break
			}
			if activity == nil || *activity == "" {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	actor := s.actorFromRequest(r)
	ct, err := s.DB().Exec(ctx, `
		UPDATE system_update_state SET
			status = 'update_requested',
			base_commit = $1,
			branch = $2,
			requested_at = now(),
			requested_by = $3,
			previous_is_running = $4,
			previous_monitoring_mode = $5,
			error_message = NULL,
			completed_at = NULL,
			updated_at = now()
		WHERE id = 1
	`, buildinfo.GitCommit, systemUpdateBranch, actor, wasRunning, monitoringMode)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusInternalServerError, "DB", "system_update_state row missing", nil)
		return
	}

	s.appendAuditLog(ctx, "system_update_state", "1", "update_requested", actor, nil, map[string]any{
		"base_commit": buildinfo.GitCommit, "was_running": wasRunning, "monitoring_mode": monitoringMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "update_requested",
		"note":   "O servidor vai reiniciar em instantes; esta tela vai recarregar sozinha quando terminar.",
	})
}
