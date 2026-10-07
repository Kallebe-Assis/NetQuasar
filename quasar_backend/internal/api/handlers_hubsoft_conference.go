package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhubsoft"
)

// "Conferência" — botão na aba Ordens de serviço da HubSoft: junta três conferências que já
// existem em telas separadas (status de conexão — igual ao badge da aba Relatório → Clientes;
// acesso remoto HTTP/HTTPS — igual à aba Ferramentas → HTTP/HTTPS; IPv6 — prefixo da última
// conexão, direto da própria HubSoft, ver integrationhubsoft.pickIPv6Prefix) para cada O.S. de
// um período, com estatística e drill-down.

// hubsoftConferenceStandardPorts — mesmas portas por omissão da aba Ferramentas → HTTP/HTTPS
// (ver ToolsPage.tsx, mxPorts) — "acesso remoto" conta como OK se qualquer uma responder.
var hubsoftConferenceStandardPorts = []string{"80", "8080", "8888", "443", "8443", "2265"}

type hubsoftConferenceRequest struct {
	DataInicio        string `json:"data_inicio"`
	DataFim           string `json:"data_fim"`
	CheckConnection   bool   `json:"check_connection"`
	CheckRemoteAccess bool   `json:"check_remote_access"`
	CheckIPv6         bool   `json:"check_ipv6"`
	// OnlyFinished pede à HubSoft só as O.S. finalizadas (filtro no servidor da HubSoft). Ausente = true.
	OnlyFinished *bool `json:"only_finished,omitempty"`
}

type hubsoftConferenceItemOut struct {
	integrationhubsoft.ConferenceOSItem
	ConnectionChecked   bool `json:"connection_checked"`
	ConnectionOnline    bool `json:"connection_online"`
	RemoteAccessChecked bool `json:"remote_access_checked"`
	RemoteAccessOK      bool `json:"remote_access_ok"`
	IPv6Checked         bool `json:"ipv6_checked"`
	IPv6Present         bool `json:"ipv6_present"`
}

type hubsoftConferenceStatBucket struct {
	Checked int `json:"checked"`
	OK      int `json:"ok"`
	Fail    int `json:"fail"`
}

type hubsoftConferenceResponse struct {
	OK        bool                                   `json:"ok"`
	Message   string                                 `json:"message,omitempty"`
	From      string                                 `json:"from"`
	To        string                                 `json:"to"`
	Total     int                                    `json:"total"`
	Resolved  int                                    `json:"resolved"`
	Truncated bool                                   `json:"truncated,omitempty"`
	Checks    hubsoftConferenceRequest               `json:"checks"`
	Stats     map[string]hubsoftConferenceStatBucket `json:"stats"`
	Items     []hubsoftConferenceItemOut             `json:"items"`
}

// --- Execução em segundo plano com progresso ---------------------------------------------------------------------
//
// A conferência pode levar minutos (varre as O.S., consulta os clientes e testa o acesso remoto). Em vez de uma
// única requisição que fica pendurada, POST /hubsoft/conference inicia a tarefa e devolve o job_id; a tela consulta
// GET /hubsoft/conference/{jobId} e mostra uma barra de progresso. O percentual é arredondado para baixo de 5 em 5
// e nunca regride.

type hubsoftConferenceJob struct {
	mu       sync.Mutex
	ID       string
	Status   string // running | done | error
	Percent  int    // múltiplo de 5
	Label    string
	Message  string
	Result   *hubsoftConferenceResponse
	Started  time.Time
	Finished time.Time
}

func (j *hubsoftConferenceJob) setProgress(pct int, label string) {
	if pct > 99 {
		pct = 99
	}
	pct = pct / 5 * 5
	j.mu.Lock()
	defer j.mu.Unlock()
	if pct > j.Percent {
		j.Percent = pct
	}
	if label != "" {
		j.Label = label
	}
}

var (
	hubsoftConferenceJobsMu sync.Mutex
	hubsoftConferenceJobs   = map[string]*hubsoftConferenceJob{}
)

func newHubsoftConferenceJob() *hubsoftConferenceJob {
	hubsoftConferenceJobsMu.Lock()
	defer hubsoftConferenceJobsMu.Unlock()
	for id, j := range hubsoftConferenceJobs { // descarta tarefas antigas
		j.mu.Lock()
		old := !j.Finished.IsZero() && time.Since(j.Finished) > 30*time.Minute
		j.mu.Unlock()
		if old {
			delete(hubsoftConferenceJobs, id)
		}
	}
	j := &hubsoftConferenceJob{ID: uuid.NewString(), Status: "running", Started: time.Now(), Label: "Iniciando"}
	hubsoftConferenceJobs[j.ID] = j
	return j
}

func (s *Server) hubsoftConference(w http.ResponseWriter, r *http.Request) {
	integID, err := s.resolveIntegrationID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body hubsoftConferenceRequest
	if decErr := json.NewDecoder(r.Body).Decode(&body); decErr != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "corpo inválido", nil)
		return
	}
	cfg, err := s.loadHubsoftConfig(r.Context(), integID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_HUBSOFT", err.Error(), nil)
		return
	}
	job := newHubsoftConferenceJob()
	go func() {
		// Independe da requisição HTTP que iniciou a tarefa (que já respondeu).
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		token, terr := s.hubsoftToken(ctx, integID, cfg)
		var res hubsoftConferenceResponse
		if terr != nil {
			res = hubsoftConferenceResponse{Message: "Falha ao autenticar na HubSoft: " + terr.Error()}
		} else {
			res = runHubsoftConference(ctx, cfg, token, body, job.setProgress)
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		job.Finished = time.Now()
		job.Result = &res
		if res.OK {
			job.Status, job.Percent, job.Label = "done", 100, "Concluído"
		} else {
			job.Status, job.Message = "error", res.Message
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "status": "running", "percent": 0})
}

// hubsoftConferenceStatus — GET /hubsoft/conference/{jobId}: progresso e, quando termina, o resultado.
func (s *Server) hubsoftConferenceStatus(w http.ResponseWriter, r *http.Request) {
	hubsoftConferenceJobsMu.Lock()
	j := hubsoftConferenceJobs[chi.URLParam(r, "jobId")]
	hubsoftConferenceJobsMu.Unlock()
	if j == nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "conferência não encontrada (reinicie a consulta)", nil)
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := map[string]any{"job_id": j.ID, "status": j.Status, "percent": j.Percent, "label": j.Label, "elapsed_s": int(time.Since(j.Started).Seconds())}
	if j.Status == "error" {
		out["message"] = j.Message
	}
	if j.Status != "running" && j.Result != nil {
		out["result"] = j.Result
	}
	writeJSON(w, http.StatusOK, out)
}

func runHubsoftConference(ctx context.Context, cfg integrationhubsoft.Config, token string, body hubsoftConferenceRequest, progress func(pct int, label string)) hubsoftConferenceResponse {
	onlyFinished := body.OnlyFinished == nil || *body.OnlyFinished
	// A coleta (O.S. + clientes) ocupa de 0 a 85%; o teste de acesso remoto, de 85 a 99%.
	data := integrationhubsoft.BuildWorkOrderConference(ctx, cfg, token, integrationhubsoft.ConferenceOptions{
		From: body.DataInicio, To: body.DataFim, OnlyFinished: onlyFinished,
		Progress: func(pct int, label string) { progress(pct*85/100, label) },
	})
	if !data.OK {
		return hubsoftConferenceResponse{Message: data.Message, From: data.From, To: data.To}
	}

	// Acesso remoto — probe concorrente por IPv4 distinto (dedupe: vários clientes raramente
	// repetem IP, mas evita trabalho em duplicado quando acontece).
	var remoteSet map[string]bool
	if body.CheckRemoteAccess {
		ips := make(map[string]struct{})
		for _, it := range data.Items {
			ip := strings.TrimSpace(it.IPv4)
			if it.Resolved && ip != "" {
				ips[ip] = struct{}{}
			}
		}
		progress(85, "Testando o acesso remoto dos clientes")
		remoteSet = probeRemoteAccessBulk(ctx, ips, func(done, total int) {
			progress(85+14*done/maxIntConf(total, 1), "Testando o acesso remoto dos clientes")
		})
	}

	out := make([]hubsoftConferenceItemOut, 0, len(data.Items))
	connStat, remoteStat, ipv6Stat := hubsoftConferenceStatBucket{}, hubsoftConferenceStatBucket{}, hubsoftConferenceStatBucket{}
	for _, it := range data.Items {
		row := hubsoftConferenceItemOut{ConferenceOSItem: it}
		if body.CheckConnection && it.Resolved && it.Connected != "" {
			row.ConnectionChecked = true
			row.ConnectionOnline = it.Connected == "true"
			connStat.Checked++
			if row.ConnectionOnline {
				connStat.OK++
			} else {
				connStat.Fail++
			}
		}
		if body.CheckIPv6 && it.Resolved {
			row.IPv6Checked = true
			row.IPv6Present = strings.TrimSpace(it.IPv6) != ""
			ipv6Stat.Checked++
			if row.IPv6Present {
				ipv6Stat.OK++
			} else {
				ipv6Stat.Fail++
			}
		}
		if body.CheckRemoteAccess && it.Resolved && it.IPv4 != "" {
			row.RemoteAccessChecked = true
			row.RemoteAccessOK = remoteSet[strings.TrimSpace(it.IPv4)]
			remoteStat.Checked++
			if row.RemoteAccessOK {
				remoteStat.OK++
			} else {
				remoteStat.Fail++
			}
		}
		out = append(out, row)
	}

	return hubsoftConferenceResponse{
		OK: true, From: data.From, To: data.To, Total: data.Total, Resolved: data.Resolved,
		Truncated: data.Truncated, Checks: body,
		Stats: map[string]hubsoftConferenceStatBucket{
			"connection": connStat, "remote_access": remoteStat, "ipv6": ipv6Stat,
		},
		Items: out,
	}
}

func maxIntConf(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const hubsoftConferenceProbeConcurrency = 24

// probeRemoteAccessBulk testa, para cada IP, se alguma das portas padrão da aba Ferramentas →
// HTTP/HTTPS responde — "acesso remoto OK" = qualquer uma responder, mesmo critério de
// isHttpMatrixRowAnyProbeAccessible no frontend — só que aqui corre em paralelo no servidor
// (bounded por semáforo) em vez de N chamadas sequenciais a partir do browser.
func probeRemoteAccessBulk(ctx context.Context, ips map[string]struct{}, onProgress func(done, total int)) map[string]bool {
	out := make(map[string]bool, len(ips))
	if len(ips) == 0 {
		return out
	}
	total, finished := len(ips), 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, hubsoftConferenceProbeConcurrency)
	for ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				out[ip] = false
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			ok := probeAnyPortReachable(ctx, ip)
			mu.Lock()
			out[ip] = ok
			finished++
			d := finished
			mu.Unlock()
			if onProgress != nil {
				onProgress(d, total)
			}
		}(ip)
	}
	wg.Wait()
	return out
}

var hubsoftConferenceHTTPClient = &http.Client{
	// insecure: certificados de CPE/gestão remota são quase sempre autoassinados.
	Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
}

// probeAnyPortReachable tenta um GET curto (http:// e https://) em cada porta padrão, na ordem —
// a primeira resposta (qualquer status HTTP, mesmo 401/403 — só interessa "respondeu") já conta
// como acesso remoto OK.
func probeAnyPortReachable(ctx context.Context, ip string) bool {
	for _, port := range hubsoftConferenceStandardPorts {
		for _, scheme := range []string{"http", "https"} {
			url := scheme + "://" + ip + ":" + port + "/"
			reqCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
			if err != nil {
				cancel()
				continue
			}
			resp, err := hubsoftConferenceHTTPClient.Do(req)
			cancel()
			if err == nil {
				_ = resp.Body.Close()
				return true
			}
		}
	}
	return false
}
