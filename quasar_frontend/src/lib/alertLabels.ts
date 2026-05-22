/** Rótulos em português para códigos internos de alertas — evitar mostrar chaves cruas na UI. */

const ALERT_TYPE_PT: Record<string, string> = {
  ping_unreachable: "Equipamento offline",
  latency_high: "Latência elevada",
  latency_degraded: "Latência degradada",
  cpu_high: "CPU elevada",
  memory_high: "Memória elevada",
  temperature_high: "Temperatura elevada",
  temperature_low: "Temperatura baixa",
  snmp_failure: "Falha SNMP",
  uptime_restart_low: "Possível reinício (uptime baixo)",
  interface_down: "Interface inativa",
  interface_down_transition: "Interface mudou para DOWN",
  pon_down: "PON inativa",
  mikrotik_sfp_tx: "SFP — potência TX",
  mikrotik_sfp_rx: "SFP — potência RX",
  telemetry_threshold: "Telemetria — limiar global",
  olt_onu_drop: "Queda de ONUs online (OLT)",
};

export function displayAlertType(code: string | null | undefined): string {
  const k = String(code ?? "").trim();
  if (!k) return "—";
  return ALERT_TYPE_PT[k] ?? k.replace(/_/g, " ");
}

/** Eventos genéricos — reutiliza o mapa de alertas quando o código coincidir. */
export function displayEventType(code: string | null | undefined): string {
  return displayAlertType(code);
}

export function displaySeverity(sev: string | null | undefined): string {
  const s = String(sev ?? "").trim().toLowerCase();
  if (!s) return "—";
  if (s === "critical") return "Crítico";
  if (s === "warning") return "Atenção";
  if (s === "info") return "Informação";
  return String(sev).trim() || "—";
}

/** Classe CSS para pastilha de severidade (página de alertas). */
export function severityPillClass(sev: string | null | undefined): string {
  const s = String(sev ?? "").trim().toLowerCase();
  if (s === "critical") return "alerts-sev-pill alerts-sev-pill--critical";
  if (s === "warning") return "alerts-sev-pill alerts-sev-pill--warning";
  if (s === "info") return "alerts-sev-pill alerts-sev-pill--info";
  return "alerts-sev-pill alerts-sev-pill--muted";
}

/** Lista «Ativos»: linha com fecho recente mostra severidade «Resolvido» (grace ~1 min no servidor). */
export function displayActiveRowSeverity(sev: string | null | undefined, closedAt: string | null | undefined): string {
  if (closedAt) return "Resolvido";
  return displaySeverity(sev);
}

export function activeRowSeverityPillClass(sev: string | null | undefined, closedAt: string | null | undefined): string {
  if (closedAt) return "alerts-sev-pill alerts-sev-pill--resolved";
  return severityPillClass(sev);
}

/** Mensagem curta na grelha de alertas (evita textos longos gerados pelo worker). */
export function displayAlertMessage(message: string | null | undefined, type: string | null | undefined): string {
  const t = String(type ?? "").trim();
  const m = String(message ?? "").trim();
  if (t === "ping_unreachable") return "Equipamento offline — sem resposta ICMP/TCP.";
  if (t === "uptime_restart_low") return "Possível reinício — tempo de atividade (uptime) abaixo do esperado.";
  if (/sem resposta ICMP/i.test(m) && /TCP|ICMP/i.test(m)) return "Equipamento offline — sem resposta ICMP/TCP.";
  if (/inalcançável/i.test(m) && /ICMP/i.test(m)) return "Equipamento offline.";
  if (t === "telemetry_threshold") {
    const cleaned = m.replace(/^[^:]+:\s*/, "").replace(/\s*—\s*estado\s+.*/i, "").trim();
    if (cleaned && !/metric_id|json|threshold_/i.test(cleaned)) return cleaned;
    if (m && !/metric_id|meta->|json|key:\s*telemetry:/i.test(m)) return m;
    return "Um valor medido ultrapassou o limiar definido em Configurações → Alertas.";
  }
  if (m && !/meta->|jsonb|uuid|ping_fail|device_id:/i.test(m)) return m;
  return m.replace(/\s+/g, " ").trim() || "—";
}

/** Data/hora legível para histórico (fuso local do navegador). */
export function formatAlertDateTimePt(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return String(iso);
    const pad = (n: number) => String(n).padStart(2, "0");
    return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} - ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  } catch {
    return String(iso);
  }
}

/** Tempo decorrido desde `iso` (actualiza quando o chamador re-renderiza com relógio periódico). */
export function formatRelativeTimeAgoPt(iso: string | null | undefined, _tick?: number): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return String(iso);
  const diffMs = Math.max(0, Date.now() - t);
  const sec = Math.floor(diffMs / 1000);
  if (sec < 10) return "agora";
  if (sec < 60) return sec === 1 ? "há 1 segundo" : `há ${sec} segundos`;
  const min = Math.floor(sec / 60);
  if (min < 60) return min === 1 ? "há 1 minuto" : `há ${min} minutos`;
  const h = Math.floor(min / 60);
  if (h < 24) return h === 1 ? "há 1 hora" : `há ${h} horas`;
  const d = Math.floor(h / 24);
  if (d < 30) return d === 1 ? "há 1 dia" : `há ${d} dias`;
  if (d < 365) {
    const months = Math.floor(d / 30);
    const daysRem = d % 30;
    if (daysRem === 0) return months === 1 ? "há 1 mês" : `há ${months} meses`;
    return months === 1 ? `há 1 mês e ${daysRem} ${daysRem === 1 ? "dia" : "dias"}` : `há ${months} meses e ${daysRem} dias`;
  }
  const years = Math.floor(d / 365);
  const remDays = d % 365;
  const months = Math.floor(remDays / 30);
  const daysRem = remDays % 30;
  const bits: string[] = [];
  if (years > 0) bits.push(years === 1 ? "1 ano" : `${years} anos`);
  if (months > 0) bits.push(months === 1 ? "1 mês" : `${months} meses`);
  if (daysRem > 0) bits.push(daysRem === 1 ? "1 dia" : `${daysRem} dias`);
  return bits.length ? `há ${bits.join(" e ")}` : "há mais de 1 ano";
}
