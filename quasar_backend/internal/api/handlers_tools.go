package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func (s *Server) toolsDNSRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host         string   `json:"host"`
		Nameservers  []string `json:"nameservers"`
		TimeoutMs    int      `json:"timeout_ms"`
		RecordTypes  []string `json:"record_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	host := strings.TrimSpace(body.Host)
	if host == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "host obrigatório", nil)
		return
	}
	timeout := time.Duration(body.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 3500 * time.Millisecond
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	types := body.RecordTypes
	if len(types) == 0 {
		types = []string{"A", "AAAA"}
	}
	servers := normalizeDNSServers(body.Nameservers)
	if len(servers) == 0 {
		servers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	client := &dns.Client{Net: "udp", Timeout: timeout}
	fqdn := dns.Fqdn(host)
	results := map[string][]string{}
	var lastErr error

	for _, qtype := range types {
		var qt uint16
		switch strings.ToUpper(qtype) {
		case "A":
			qt = dns.TypeA
		case "AAAA":
			qt = dns.TypeAAAA
		case "CNAME":
			qt = dns.TypeCNAME
		case "MX":
			qt = dns.TypeMX
		case "TXT":
			qt = dns.TypeTXT
		case "NS":
			qt = dns.TypeNS
		default:
			continue
		}
		msg := new(dns.Msg)
		msg.SetQuestion(fqdn, qt)
		msg.RecursionDesired = true
		var answers []string
		for _, srv := range servers {
			if err := ctx.Err(); err != nil {
				lastErr = err
				break
			}
			in, _, err := client.ExchangeContext(ctx, msg, srv)
			if err != nil {
				lastErr = err
				continue
			}
			for _, a := range in.Answer {
				switch rr := a.(type) {
				case *dns.A:
					answers = append(answers, rr.A.String())
				case *dns.AAAA:
					answers = append(answers, rr.AAAA.String())
				case *dns.CNAME:
					answers = append(answers, strings.TrimSuffix(rr.Target, "."))
				case *dns.MX:
					answers = append(answers, strings.TrimSuffix(rr.Mx, "."))
				case *dns.TXT:
					answers = append(answers, strings.Join(rr.Txt, " "))
				case *dns.NS:
					answers = append(answers, strings.TrimSuffix(rr.Ns, "."))
				}
			}
			if len(answers) > 0 {
				lastErr = nil
				break
			}
		}
		if len(answers) > 0 {
			results[strings.ToUpper(qtype)] = answers
		}
	}

	out := map[string]any{
		"host":       host,
		"nameservers": servers,
		"records":    results,
	}
	if len(results) == 0 && lastErr != nil {
		out["error"] = lastErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

func normalizeDNSServers(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, ":") {
			s = net.JoinHostPort(s, "53")
		}
		out = append(out, s)
	}
	return out
}

func (s *Server) toolsHTTPProbeStub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL                string `json:"url"`
		Method             string `json:"method"`
		TimeoutMs          int    `json:"timeout_ms"`
		FollowRedirects    bool   `json:"follow_redirects"`
		InsecureTLS        bool   `json:"insecure_tls"`
		MaxBodyBytes       int64  `json:"max_body_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	u := strings.TrimSpace(body.URL)
	if u == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "url obrigatória", nil)
		return
	}
	method := strings.ToUpper(strings.TrimSpace(body.Method))
	if method == "" {
		method = http.MethodGet
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	if to <= 0 {
		to = 8 * time.Second
	}
	if to > 60*time.Second {
		to = 60 * time.Second
	}
	maxBody := body.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 512 * 1024
	}
	if maxBody > 4*1024*1024 {
		maxBody = 4 * 1024 * 1024
	}

	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: body.InsecureTLS},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   to,
	}
	if !body.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), to)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_URL", err.Error(), nil)
		return
	}
	t0 := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(t0).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":           false,
			"url":          u,
			"latency_ms":   latency,
			"error":        err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	n, _ := io.CopyN(io.Discard, resp.Body, maxBody)

	tlsVer := ""
	if resp.TLS != nil {
		tlsVer = tls.VersionName(resp.TLS.Version)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"url":            u,
		"status":         resp.StatusCode,
		"latency_ms":     latency,
		"bytes_discarded": n,
		"tls_version":    tlsVer,
		"content_type":   resp.Header.Get("Content-Type"),
	})
}

func (s *Server) toolsICMPPing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string `json:"host"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	if to <= 0 {
		to = 4 * time.Second
	}
	if to > 15*time.Second {
		to = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to+200*time.Millisecond)
	defer cancel()
	out := probing.ICMPPing(ctx, body.Host, to, 32)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) toolsSNMPGet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string   `json:"host"`
		Port      int      `json:"port"`
		Community string   `json:"community"`
		OIDs      []string `json:"oids"`
		Version   string   `json:"version"`
		TimeoutMs int      `json:"timeout_ms"`
		Retries   int      `json:"retries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if len(body.OIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "oids não vazio", nil)
		return
	}
	if strings.TrimSpace(body.Community) == "" {
		var def *string
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&def)
		if def != nil {
			body.Community = strings.TrimSpace(*def)
		}
	}
	if strings.TrimSpace(body.Community) == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community não informada e sem padrão configurado", nil)
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	if to <= 0 {
		to = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
	defer cancel()
	p := probing.SNMPGetParams{
		Host:      body.Host,
		Port:      uint16(body.Port),
		Community: body.Community,
		OIDs:      body.OIDs,
		Version:   body.Version,
		Timeout:   to,
		Retries:   body.Retries,
	}
	res := probing.SNMPGet(ctx, p)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) toolsTelnetTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host         string `json:"host"`
		Port         string `json:"port"`
		TimeoutMs    int    `json:"timeout_ms"`
		User         string `json:"user"`
		Password     string `json:"password"`
		MaxReadBytes int    `json:"max_read_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	if to <= 0 {
		to = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
	defer cancel()
	res := probing.TelnetProbe(ctx, probing.TelnetTestParams{
		Host: body.Host, Port: body.Port, Timeout: to,
		User: body.User, Password: body.Password, MaxReadBytes: body.MaxReadBytes,
	})
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) toolsSSHTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string `json:"host"`
		Port      string `json:"port"`
		User      string `json:"user"`
		Password  string `json:"password"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	if to <= 0 {
		to = 12 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
	defer cancel()
	p := probing.SSHDialParams{Host: body.Host, Port: body.Port, User: body.User, Password: body.Password, Timeout: to}
	res := probing.SSHDialWithPassword(ctx, p)
	writeJSON(w, http.StatusOK, res)
}
