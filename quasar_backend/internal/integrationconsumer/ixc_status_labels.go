package integrationconsumer

import "strings"

// FormatIXCAttendanceStatus traduz códigos de status do su_ticket.
func FormatIXCAttendanceStatus(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	labels := map[string]string{
		"N":  "Novo",
		"P":  "Pendente",
		"EP": "Em progresso",
		"S":  "Solucionado",
		"C":  "Cancelado",
	}
	if lbl, ok := labels[strings.ToUpper(code)]; ok {
		return lbl
	}
	low := strings.ToLower(code)
	if strings.Contains(low, "pendente") {
		return "Pendente"
	}
	if strings.Contains(low, "solucion") {
		return "Solucionado"
	}
	if strings.Contains(low, "cancel") {
		return "Cancelado"
	}
	if strings.Contains(low, "progresso") {
		return "Em progresso"
	}
	return code
}

// FormatIXCWorkOrderStatus traduz códigos de status do su_oss_chamado.
func FormatIXCWorkOrderStatus(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	labels := map[string]string{
		"A":   "Aberta",
		"AN":  "Análise",
		"EN":  "Encaminhada",
		"AS":  "Assumida",
		"AG":  "Agendada",
		"DS":  "Deslocamento",
		"EX":  "Execução",
		"F":   "Finalizada",
		"RAG": "Aguardando agendamento",
	}
	upper := strings.ToUpper(code)
	if lbl, ok := labels[upper]; ok {
		return lbl
	}
	return formatWorkOrderStatusLabel(code)
}
