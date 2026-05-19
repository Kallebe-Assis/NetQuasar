package probing

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RunNmap executa varredura rápida (requer nmap instalado no servidor).
func RunNmap(ctx context.Context, host, mode string) (command string, output string, err error) {
	host, err = ValidateToolHost(host)
	if err != nil {
		return "", "", err
	}
	nmapPath, err := exec.LookPath("nmap")
	if err != nil {
		return "", "", fmt.Errorf("nmap não encontrado no PATH do servidor — instale nmap e reinicie o backend")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	args := []string{"-Pn", "-T4", "--host-timeout", "25s"}
	switch mode {
	case "ping", "sn", "":
		args = append(args, "-sn")
	case "quick", "fast":
		args = append(args, "-F")
	default:
		args = append(args, "-sn")
	}
	args = append(args, host)
	cmd := exec.CommandContext(ctx, nmapPath, args...)
	out, runErr := cmd.CombinedOutput()
	output = strings.TrimSpace(string(out))
	command = "nmap " + strings.Join(args, " ")
	if runErr != nil && output == "" {
		return command, output, runErr
	}
	if ctx.Err() != nil {
		return command, output, ctx.Err()
	}
	return command, output, nil
}
