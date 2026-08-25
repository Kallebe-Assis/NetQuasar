package probing

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var nmapPortsRe = regexp.MustCompile(`^[0-9,\-]+$`)

// ValidateNmapPorts aceita listas nmap (ex.: 22,80,443 ou 1-1024).
func ValidateNmapPorts(ports string) (string, error) {
	ports = strings.TrimSpace(ports)
	if ports == "" {
		return "", nil
	}
	if len(ports) > 200 {
		return "", fmt.Errorf("lista de portas demasiado longa")
	}
	if !nmapPortsRe.MatchString(ports) {
		return "", fmt.Errorf("portas inválidas — use números, vírgulas e hífens (ex.: 22,80,443 ou 1-1024)")
	}
	return ports, nil
}

// RunNmap executa varredura rápida (requer nmap instalado no servidor).
func RunNmap(ctx context.Context, host, mode, ports string) (command string, output string, err error) {
	host, err = ValidateToolHost(host)
	if err != nil {
		return "", "", err
	}
	ports, err = ValidateNmapPorts(ports)
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
	case "ports", "custom":
		if ports == "" {
			return "", "", fmt.Errorf("indique as portas no modo personalizado (ex.: 22,80,443)")
		}
		args = append(args, "-p", ports)
	case "quick", "fast":
		if ports != "" {
			args = append(args, "-p", ports)
		} else {
			args = append(args, "-F")
		}
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
