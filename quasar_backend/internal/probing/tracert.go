package probing

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RunTracert executa traceroute/tracert no servidor (ICMP/UDP conforme SO).
func RunTracert(ctx context.Context, host string, maxHops int, timeoutPerHopMs int) (command string, output string, hops []map[string]any, err error) {
	host, err = ValidateToolHost(host)
	if err != nil {
		return "", "", nil, err
	}
	if maxHops < 1 || maxHops > 64 {
		maxHops = 30
	}
	if timeoutPerHopMs < 500 {
		timeoutPerHopMs = 500
	}
	if timeoutPerHopMs > 15000 {
		timeoutPerHopMs = 15000
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "tracert", "-h", strconv.Itoa(maxHops), "-w", strconv.Itoa(timeoutPerHopMs), host)
	} else {
		waitSec := (timeoutPerHopMs + 999) / 1000
		if waitSec < 1 {
			waitSec = 1
		}
		if path, lookErr := exec.LookPath("traceroute"); lookErr == nil {
			cmd = exec.CommandContext(ctx, path, "-m", strconv.Itoa(maxHops), "-w", strconv.Itoa(waitSec), "-q", "1", host)
		} else if path, lookErr := exec.LookPath("tracepath"); lookErr == nil {
			cmd = exec.CommandContext(ctx, path, "-m", strconv.Itoa(maxHops), host)
		} else {
			return "", "", nil, fmt.Errorf("traceroute/tracepath não encontrado no servidor (instale traceroute no Linux ou use Windows)")
		}
	}

	out, runErr := cmd.CombinedOutput()
	output = strings.TrimSpace(string(out))
	command = strings.Join(cmd.Args, " ")
	hops = parseTracertOutput(output, runtime.GOOS == "windows")
	if runErr != nil && output == "" {
		return command, output, hops, runErr
	}
	if ctx.Err() != nil {
		return command, output, hops, ctx.Err()
	}
	return command, output, hops, nil
}

var reWinHop = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)

func parseTracertOutput(output string, windows bool) []map[string]any {
	var hops []map[string]any
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if windows {
			if m := reWinHop.FindStringSubmatch(line); len(m) == 3 {
				hops = append(hops, map[string]any{
					"hop":     m[1],
					"detail":  strings.TrimSpace(m[2]),
					"raw":     line,
				})
			}
			continue
		}
		if strings.HasPrefix(line, " ") || strings.Contains(line, "(") {
			hops = append(hops, map[string]any{"raw": line})
		}
	}
	return hops
}

// ValidateToolHost rejeita entrada que possa ser interpretada como shell injection.
func ValidateToolHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host obrigatório")
	}
	if len(host) > 253 {
		return "", fmt.Errorf("host demasiado longo")
	}
	if strings.ContainsAny(host, ";&|`$<>\"'\\") {
		return "", fmt.Errorf("caracteres inválidos no host")
	}
	return host, nil
}

// DefaultToolTimeout limite global para ferramentas de rede.
func DefaultToolTimeout(ms int) time.Duration {
	if ms < 1000 {
		ms = 1000
	}
	if ms > 120000 {
		ms = 120000
	}
	return time.Duration(ms) * time.Millisecond
}
