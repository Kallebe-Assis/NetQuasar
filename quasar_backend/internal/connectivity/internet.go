package connectivity

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Result representa o resultado de uma verificação de saída à Internet.
type Result struct {
	OK            bool              `json:"ok"`
	CheckedAt     time.Time         `json:"checked_at"`
	TargetsTried  []TargetResult    `json:"targets_tried"`
	LatencyMS     int64             `json:"latency_ms,omitempty"`
	ErrorCode     string            `json:"error_code,omitempty"`
	ErrorDetail   string            `json:"error_detail,omitempty"`
}

type TargetResult struct {
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// Check tenta alcançar pelo menos um dos targets (HTTPS GET, timeout curto).
func Check(ctx context.Context, targets []string, timeout time.Duration) Result {
	if len(targets) == 0 {
		targets = []string{"https://1.1.1.1", "https://www.google.com/generate_204"}
	}
	if timeout <= 0 {
		timeout = 3500 * time.Millisecond
	}
	res := Result{CheckedAt: time.Now().UTC(), TargetsTried: make([]TargetResult, 0, len(targets))}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
			ResponseHeaderTimeout: timeout,
		},
	}

	var firstOKMS int64
	for _, raw := range targets {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			u = "https://" + u
		}
		t0 := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			res.TargetsTried = append(res.TargetsTried, TargetResult{URL: u, Error: err.Error()})
			continue
		}
		resp, err := client.Do(req)
		ms := time.Since(t0).Milliseconds()
		tr := TargetResult{URL: u, LatencyMS: ms}
		if err != nil {
			tr.Error = err.Error()
			res.TargetsTried = append(res.TargetsTried, tr)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 || resp.StatusCode == 204 {
			tr.OK = true
			res.TargetsTried = append(res.TargetsTried, tr)
			if !res.OK {
				res.OK = true
				firstOKMS = ms
			}
			continue
		}
		tr.Error = fmt.Sprintf("http status %d", resp.StatusCode)
		res.TargetsTried = append(res.TargetsTried, tr)
	}

	if res.OK {
		res.LatencyMS = firstOKMS
		return res
	}
	res.OK = false
	res.ErrorCode = classifyError(res.TargetsTried)
	res.ErrorDetail = summarizeErrors(res.TargetsTried)
	return res
}

func classifyError(tt []TargetResult) string {
	for _, t := range tt {
		if t.Error == "" {
			continue
		}
		e := strings.ToLower(t.Error)
		if strings.Contains(e, "timeout") || strings.Contains(e, "deadline") {
			return "TIMEOUT"
		}
		if strings.Contains(e, "no such host") || strings.Contains(e, "lookup") {
			return "DNS"
		}
		if strings.Contains(e, "connection refused") {
			return "CONNECTION_REFUSED"
		}
	}
	return "NO_INTERNET"
}

func summarizeErrors(tt []TargetResult) string {
	var b strings.Builder
	for i, t := range tt {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s: %s", t.URL, firstNonEmpty(t.Error, "failed"))
	}
	return b.String()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// TargetsFromJSON decodifica lista JSON de strings.
func TargetsFromJSON(raw []byte, def []string) []string {
	var arr []string
	if len(raw) == 0 {
		return def
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return def
	}
	out := make([]string, 0, len(arr))
	for _, s := range arr {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
