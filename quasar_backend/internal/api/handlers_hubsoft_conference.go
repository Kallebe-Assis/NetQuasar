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
	extendWriteDeadline(w, 4*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	token, err := s.hubsoftToken(ctx, integID, cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AUTH", err.Error(), nil)
		return
	}

	data := integrationhubsoft.BuildWorkOrderConferenceData(ctx, cfg, token, body.DataInicio, body.DataFim)
	if !data.OK {
		writeJSON(w, http.StatusOK, hubsoftConferenceResponse{Message: data.Message, From: data.From, To: data.To})
		return
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
		remoteSet = probeRemoteAccessBulk(ctx, ips)
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

	writeJSON(w, http.StatusOK, hubsoftConferenceResponse{
		OK: true, From: data.From, To: data.To, Total: data.Total, Resolved: data.Resolved,
		Truncated: data.Truncated, Checks: body,
		Stats: map[string]hubsoftConferenceStatBucket{
			"connection": connStat, "remote_access": remoteStat, "ipv6": ipv6Stat,
		},
		Items: out,
	})
}

const hubsoftConferenceProbeConcurrency = 24

// probeRemoteAccessBulk testa, para cada IP, se alguma das portas padrão da aba Ferramentas →
// HTTP/HTTPS responde — "acesso remoto OK" = qualquer uma responder, mesmo critério de
// isHttpMatrixRowAnyProbeAccessible no frontend — só que aqui corre em paralelo no servidor
// (bounded por semáforo) em vez de N chamadas sequenciais a partir do browser.
func probeRemoteAccessBulk(ctx context.Context, ips map[string]struct{}) map[string]bool {
	out := make(map[string]bool, len(ips))
	if len(ips) == 0 {
		return out
	}
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
			mu.Unlock()
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
