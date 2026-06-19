export type AuditRowView = {
  id?: number;
  entity_type: string;
  entity_id: string;
  entity_label?: string | null;
  action: string;
  actor?: string | null;
  before_data?: Record<string, unknown> | null;
  after_data?: Record<string, unknown> | null;
  created_at: string;
};

const ENTITY_LABELS: Record<string, string> = {
  device: "Equipamento",
  pop: "POP",
  pop_contact: "Contacto POP",
  monitoring_runtime: "Monitoramento",
  monitoring_intervals: "Intervalos de monitoramento",
  monitoring_settings: "Definições de monitoramento",
  commercial_locality: "Localidade comercial",
  commercial_monthly_record: "Base de clientes",
  client_connection: "Conexão de cliente",
  network_tool: "Ferramenta de rede",
  integration: "Integração",
  user: "Usuário",
  alert_rule: "Regra de alerta",
  maintenance_window: "Janela de manutenção",
  automation_onu_report: "Automação — relatório ONU",
  automation_alerts_digest: "Automação — resumo de alertas",
  automation_commercial_report: "Automação — base comercial",
  nightly_collection: "Automação — coleta noturna",
  olt_vendor_model: "Modelo OLT",
  olt_vendor_profile: "Perfil OLT",
  settings_connection_defaults: "Definições SNMP/ligação",
  settings_telegram: "Telegram",
  settings_ui: "Aparência",
  settings_smtp: "E-mail (SMTP)",
  settings_mikrotik_collection: "Coleta Mikrotik",
  device_config_backup: "Backup de configuração",
  commercial_report: "Relatório comercial",
};

const ACTION_LABELS: Record<string, string> = {
  create: "Adicionado",
  patch: "Editado",
  delete: "Removido",
  put: "Atualizado",
  start: "Iniciado",
  stop: "Parado",
  run: "Executado",
  run_manual: "Execução manual",
  bulk_upsert: "Importação/atualização em massa",
  import_csv: "Importação CSV",
  import: "Importação/atualização",
  refresh_olt: "Atualização ONU/PON (OLT)",
  refresh_interfaces: "Atualização de interfaces",
  collect_telemetry: "Coleta de telemetria",
  ping_run: "Teste de ping",
  snmp_discover: "Descoberta SNMP",
  full_report: "Relatório completo",
  reload_devices: "Recarregar equipamentos",
  executed: "Executado",
  test_send: "Envio de teste",
  telegram_send: "Envio Telegram",
};

export function formatAuditEntityType(type: string): string {
  const t = String(type ?? "").trim();
  return ENTITY_LABELS[t] ?? t.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function formatAuditAction(action: string, entityType?: string): string {
  const a = String(action ?? "").trim();
  if (a === "start" && entityType === "monitoring_runtime") return "Monitoramento iniciado";
  if (a === "stop" && entityType === "monitoring_runtime") return "Monitoramento parado";
  return ACTION_LABELS[a] ?? a.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function formatAuditActor(actor?: string | null): string {
  const a = String(actor ?? "").trim();
  if (!a || a === "anonymous") return "SISTEMA";
  const lower = a.toLowerCase();
  if (
    lower === "sistema" ||
    lower === "system" ||
    lower === "worker" ||
    lower === "automation" ||
    lower === "scheduler" ||
    lower === "system:monitor_worker" ||
    lower.startsWith("system:")
  ) {
    return "SISTEMA";
  }
  if (lower === "chave api" || a.startsWith("key:")) return "Chave API";
  return a;
}

const NETWORK_TOOL_LABELS: Record<string, string> = {
  dns: "Consulta DNS",
  http_probe: "Teste HTTP",
  icmp_ping: "Ping ICMP",
  snmp_get: "SNMP GET",
  telnet: "Teste Telnet",
  ssh: "Teste SSH",
  mikrotik_walk: "Walk SNMP (interfaces)",
};

export function resolveAuditEntityName(row: AuditRowView): string {
  const label = String(row.entity_label ?? "").trim();
  if (label) return label;
  const after = row.after_data ?? {};
  const before = row.before_data ?? {};
  const fromJson =
    String(after.description ?? after.name ?? before.description ?? before.name ?? "").trim() ||
    String(after.host ?? after.ip ?? "").trim();
  if (fromJson) return fromJson;
  const et = row.entity_type;
  if (et === "monitoring_runtime") return "Sistema";
  if (et === "network_tool") {
    const tool = String(after.tool ?? row.entity_id ?? "").trim();
    return NETWORK_TOOL_LABELS[tool] ?? (tool || "Ferramenta");
  }
  if (et === "commercial_monthly_record" && row.entity_id === "bulk") return "Registos mensais";
  return formatAuditEntityType(et);
}

export function formatAuditSummary(row: AuditRowView): string {
  const name = resolveAuditEntityName(row);
  const action = formatAuditAction(row.action, row.entity_type);
  const kind = formatAuditEntityType(row.entity_type);
  if (row.entity_type === "device" || row.entity_type === "pop" || row.entity_type === "client_connection") {
    return `${kind}: ${name} — ${action}`;
  }
  if (row.entity_type === "monitoring_runtime") {
    return action;
  }
  if (row.entity_type === "network_tool") {
    return `Ferramenta: ${name} — ${action}`;
  }
  return `${kind} — ${action}${name && name !== kind ? ` (${name})` : ""}`;
}

export function formatAuditDetailPreview(row: AuditRowView): string {
  const after = row.after_data ?? {};
  const parts: string[] = [];
  const pick = (k: string, label: string) => {
    const v = after[k];
    if (v == null || v === "") return;
    parts.push(`${label}: ${String(v)}`);
  };
  pick("description", "Nome");
  pick("host", "Host");
  pick("ip", "IP");
  pick("ok", "OK");
  pick("latency_ms", "Latência (ms)");
  pick("count", "Quantidade");
  pick("imported", "Importados");
  pick("scope", "Âmbito");
  pick("mode", "Modo");
  pick("tool", "Ferramenta");
  if (parts.length) return parts.join(" · ");
  return "";
}
