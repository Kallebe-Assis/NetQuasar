package alertnotify

import (
	"fmt"
	"strings"
	"time"
)

// MonitoringTestMessage gera texto de exemplo para o botão de teste Telegram (monitorização).
func MonitoringTestMessage(template string) string {
	ts := time.Now().Format("02/01/2006 15:04")
	equip := "OLT-Central"
	ip := "10.0.0.1"
	switch strings.ToLower(strings.TrimSpace(template)) {
	case "ping_unreachable", "offline":
		return formatMonitoringTelegram("CRITICAL", "Equipamento offline",
			fmt.Sprintf("%s (%s): sem resposta ICMP/TCP dentro do tempo de espera configurado.", equip, ip))
	case "latency_high", "latency":
		return formatMonitoringTelegram("WARNING", "Latência elevada",
			fmt.Sprintf("%s (%s): latência ICMP/TCP em 342 ms (limiar warning; 3 leituras consecutivas).", equip, ip))
	case "uptime", "uptime_restart_low":
		return formatMonitoringTelegram("WARNING", "Possível reinício do equipamento",
			fmt.Sprintf("Concentrador-Norte (%s): uptime SNMP em 12 min — abaixo do limiar configurado.", ip))
	case "sfp_rx", "mikrotik_sfp_rx":
		return formatMonitoringTelegram("WARNING", "Potência óptica SFP",
			fmt.Sprintf("Router-Core (%s): interface sfp1 — potência SFP RX -18.570 dBm (severidade: warning).", ip))
	case "sfp_tx", "mikrotik_sfp_tx":
		return formatMonitoringTelegram("WARNING", "Potência óptica SFP",
			fmt.Sprintf("Router-Core (%s): interface sfp1 — potência SFP TX -12.300 dBm (severidade: warning).", ip))
	case "pon_down", "pon_off", "olt_onu_drop":
		return formatMonitoringTelegram("CRITICAL", "Queda de ONUs online — PON",
			fmt.Sprintf("Queda de 8 ONUs online na PON 0/1/1 da OLT %s (%s).", equip, ip))
	case "interface_down", "interface_down_transition":
		return formatMonitoringTelegram("WARNING", "Interface DOWN (mudança de estado)",
			fmt.Sprintf("%s (%s): interface ether1 mudou de UP para DOWN.", equip, ip))
	case "telemetry", "telemetry_threshold":
		return formatMonitoringTelegram("WARNING", "Telemetria — limiar global",
			fmt.Sprintf("%s (%s): Temperatura está em 68.50 — estado Atenção segundo os seus limiares de alerta.", equip, ip))
	case "snmp_failure":
		return formatMonitoringTelegram("WARNING", "Falha SNMP",
			fmt.Sprintf("%s (%s): telemetria SNMP sem resposta útil na última coleta.", equip, ip))
	default:
		return fmt.Sprintf("NetQuasar\nTeste de alerta de monitoramento\n%s", ts)
	}
}

// ReportsTestMessage gera texto de exemplo para o canal de relatórios.
func ReportsTestMessage(template string) string {
	ts := time.Now().Format("02/01/2006 15:04")
	switch strings.ToLower(strings.TrimSpace(template)) {
	case "alerts_digest":
		return fmt.Sprintf("NetQuasar — Resumo de alertas\n%s\n\nAbertos: 12 | Resolvidos (24 h): 5 | Incidentes correlacionados: 2", ts)
	case "onu_monthly":
		return fmt.Sprintf("NetQuasar — Relatório mensal ONU\n%s\n\nONUs online: 4.832 | Offline: 218 | OLTs consultadas: 6", ts)
	default:
		return fmt.Sprintf("NetQuasar\nTeste de envio de relatório\n%s", ts)
	}
}

func formatMonitoringTelegram(level, title, detail string) string {
	equip, ip, incident, value := shortEquipmentAndIncident(detail)
	metric := metricLabel(title, incident)
	return fmt.Sprintf("%s *%s*\n\n*Equipamento:* %s\n*IP:* %s\n*Incidente:* %s\n*Métrica:* %s\n*Valor:* %s",
		levelEmoji(level), title, equip, ip, incident, metric, value)
}
