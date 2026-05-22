package integrationconsumer

import "strings"

// FormatIXCOnline traduz campo online (radusuarios / login).
func FormatIXCOnline(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "S":
		return "Online"
	case "N":
		return "Offline"
	case "SS":
		return "Sem status"
	default:
		return strings.TrimSpace(code)
	}
}

// FormatIXCStatusInternet traduz status_internet do contrato IXC.
func FormatIXCStatusInternet(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "A":
		return "Ativo"
	case "D":
		return "Desativado"
	case "CM":
		return "Bloqueio Manual"
	case "CA":
		return "Bloqueio Automático"
	case "FA":
		return "Financeiro em atraso"
	case "AA":
		return "Aguardando Assinatura"
	default:
		return strings.TrimSpace(code)
	}
}
