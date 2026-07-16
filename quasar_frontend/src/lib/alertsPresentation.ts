/** Apresentação dos alertas na UI — categorias, filtros, textos curtos (sem JSON/códigos ao utilizador). */

export type AlertUiCategory = "equipment" | "interface" | "olt" | "optical" | "performance" | "system";

const TYPE_CATEGORY: Record<string, AlertUiCategory> = {
  ping_unreachable: "equipment",
  uptime_restart_low: "equipment",
  snmp_failure: "equipment",
  telemetry_threshold: "equipment",
  latency_high: "performance",
  latency_degraded: "performance",
  cpu_high: "performance",
  memory_high: "performance",
  temperature_high: "performance",
  temperature_low: "performance",
  interface_down: "interface",
  interface_down_transition: "interface",
  mikrotik_sfp_tx: "optical",
  mikrotik_sfp_rx: "optical",
  olt_onu_drop: "olt",
  olt_onu_rise: "olt",
  bng_subscriber_drop: "equipment",
  pon_down: "olt",
};

export function alertCategoryLabel(cat: AlertUiCategory): string {
  switch (cat) {
    case "equipment":
      return "Equipamento";
    case "interface":
      return "Interface";
    case "olt":
      return "OLT / PON";
    case "optical":
      return "Óptica / SFP";
    case "performance":
      return "Performance";
    default:
      return "Sistema";
  }
}

export function alertCategoryFromType(type: string | null | undefined): AlertUiCategory {
  const k = String(type ?? "").trim();
  return TYPE_CATEGORY[k] ?? "system";
}

/** Título curto para coluna «Problema» */
export function alertProblemTitle(type: string | null | undefined): string {
  const t = String(type ?? "").trim();
  switch (t) {
    case "ping_unreachable":
      return "Equipamento offline";
    case "latency_high":
      return "Latência alta";
    case "uptime_restart_low":
      return "Possível reinício";
    case "interface_down_transition":
      return "Interface DOWN";
    case "mikrotik_sfp_tx":
      return "Potência TX - SFP";
    case "mikrotik_sfp_rx":
      return "Potência RX - SFP";
    case "telemetry_threshold":
      return "Limiar de métrica";
    case "olt_onu_drop":
      return "Queda de ONUs";
    case "olt_onu_rise":
      return "Subida de ONUs";
    case "bng_subscriber_drop":
      return "Queda de logins BNG";
    default:
      return "Alerta";
  }
}

export function alertEquipmentPrimary(type: string | null | undefined, deviceName: string | null | undefined, message: string | null | undefined, meta: unknown): string {
  const t = String(type ?? "").trim();
  const m = metaObj(meta);
  if (t === "olt_onu_drop" || t === "olt_onu_rise") {
    const msgRaw = String(message ?? "");
    const oltFromMsg = msgRaw.match(/da\s+OLT\s+(.+)\s+\([^)]+\)\s*\.?\s*$/im)?.[1]?.trim() ?? "";
    const olt = String(deviceName ?? "").trim() || oltFromMsg;
    let ponKey = "";
    const ponMeta = m?.pon;
    if (typeof ponMeta === "string" && ponMeta.trim()) ponKey = ponMeta.trim();
    else {
      const hit = msgRaw.match(/na\s+PON\s+(\S+)/i);
      if (hit?.[1]) ponKey = hit[1];
      else {
        const hit2 = msgRaw.match(/PON\s+([a-z0-9_\/.\-]+)/i);
        if (hit2?.[1]) ponKey = hit2[1];
      }
    }
    if (ponKey) {
      return olt ? `${olt} — PON ${ponKey}` : `PON ${ponKey}`;
    }
    return olt || "-";
  }
  const msgStr = String(message ?? "");
  const equipName = String(deviceName ?? "").trim();

  function looksLikeIpOrOnlyDigits(raw: string): boolean {
    const s = raw.trim();
    if (!s) return false;
    if (/^\d+$/.test(s)) return true;
    if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(s)) return true;
    return false;
  }

  /** Nome SNMP / lógico da porta (sem o equipamento). */
  function resolveIfaceLabel(): string {
    const display = m?.display_name;
    if (typeof display === "string" && display.trim() && !looksLikeIpOrOnlyDigits(display)) return display.trim();
    const ifName = m?.if_name;
    if (typeof ifName === "string" && ifName.trim() && !looksLikeIpOrOnlyDigits(ifName)) return ifName.trim();
    const ifIdx = m?.if_index;
    if (typeof ifIdx === "number" && Number.isFinite(ifIdx)) return `ifIndex ${Math.round(ifIdx)}`;
    const sfpIf = msgStr.match(/\binterface\s+(.+?)\s+[—\-]\s*potência/i);
    if (sfpIf?.[1]?.trim() && !looksLikeIpOrOnlyDigits(sfpIf[1])) return sfpIf[1].trim();
    const mudouIf = msgStr.match(/\binterface\s+(\S+)\s+mudou/i);
    if (mudouIf?.[1] && !looksLikeIpOrOnlyDigits(mudouIf[1])) return mudouIf[1];
    const loose = msgStr.match(/\binterface\s+([a-z0-9_\/.@+-]+)/i);
    if (loose?.[1] && !looksLikeIpOrOnlyDigits(loose[1])) return loose[1];
    return "";
  }

  if (t === "mikrotik_sfp_tx" || t === "mikrotik_sfp_rx") {
    let equip = equipName;
    if (!equip) {
      const head = msgStr.match(/^\s*(.+?)\s*\([^)]+\)\s*:/);
      if (head?.[1]?.trim()) equip = head[1].trim();
    }
    const iface = resolveIfaceLabel();
    if (equip && iface) return `${equip} - ${iface}`;
    if (equip) return equip;
    if (iface) return iface;
    return "-";
  }

  if (t === "interface_down_transition" || t === "interface_down") {
    const iface = resolveIfaceLabel();
    if (equipName && iface) return `${equipName} - ${iface}`;
    if (equipName) return equipName;
    if (iface) return iface;
  }
  return equipName || "-";
}

/** Tipos conhecidos para filtro (valor API = value) */
export const ALERT_TYPE_FILTER_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "Todos os tipos" },
  { value: "ping_unreachable", label: "Equipamento offline" },
  { value: "latency_high", label: "Latência elevada" },
  { value: "uptime_restart_low", label: "Possível reinício (uptime)" },
  { value: "telemetry_threshold", label: "Limiar de telemetria (ex.: temperatura)" },
  { value: "interface_down_transition", label: "Interface mudou para DOWN" },
  { value: "mikrotik_sfp_tx", label: "SFP — TX" },
  { value: "mikrotik_sfp_rx", label: "SFP — RX" },
  { value: "olt_onu_drop", label: "OLT — queda de ONUs" },
  { value: "bng_subscriber_drop", label: "BNG — queda de logins" },
  { value: "olt_onu_rise", label: "OLT — subida de ONUs" },
];

export const ALERT_SEVERITY_FILTER_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "Todas as severidades" },
  { value: "critical", label: "Crítico" },
  { value: "warning", label: "Atenção" },
  { value: "info", label: "Informação" },
];

/** Tempo relativo compacto (estilo «há 2 min») */
export function formatRelativeCompactPt(iso: string | null | undefined, tick?: number): string {
  void tick;
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const sec = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (sec < 45) return "agora";
  const min = Math.floor(sec / 60);
  if (min < 60) return min <= 1 ? "há 1 min" : `há ${min} min`;
  const h = Math.floor(min / 60);
  if (h < 48) return h <= 1 ? "há 1 h" : `há ${h} h`;
  const d = Math.floor(h / 24);
  return d <= 1 ? "há 1 dia" : `há ${d} dias`;
}

function metaObj(meta: unknown): Record<string, unknown> | null {
  if (!meta || typeof meta !== "object" || Array.isArray(meta)) return null;
  return meta as Record<string, unknown>;
}

/** Números vindos da API / JSONB por vezes chegam como string. */
function coerceFiniteNumber(v: unknown): number | null {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string") {
    const n = Number(String(v).trim().replace(",", "."));
    return Number.isFinite(n) ? n : null;
  }
  return null;
}

function fmtByUnit(n: number, unit: "ms" | "°C" | "dBm" | "%" | "ONUs" | "min"): string {
  switch (unit) {
    case "ms":
      return `${Math.round(n)} ms`;
    case "°C":
      return `${n.toFixed(1)} °C`;
    case "dBm":
      return `${n.toFixed(2)} dBm`;
    case "%":
      return `${n.toFixed(1)}%`;
    case "ONUs":
      return `${Math.round(n)} ONUs`;
    case "min":
      return `${Math.round(n)} min`;
    default:
      return String(n);
  }
}

export function alertValueText(type: string | null | undefined, message: string | null | undefined, meta: unknown): string {
  const m = metaObj(meta);
  const t = String(type ?? "").trim();
  if (t === "ping_unreachable") return "-";

  if (t === "olt_onu_drop" || t === "olt_onu_rise") {
    const pctKey = t === "olt_onu_rise" ? "rise_online_pct" : "drop_online_pct";
    const cntKey = t === "olt_onu_rise" ? "rise_online_count" : "drop_online_count";
    const pct = m?.[pctKey];
    const cnt = m?.[cntKey];
    if (typeof pct === "number" && Number.isFinite(pct) && typeof cnt === "number" && Number.isFinite(cnt)) {
      return `${pct.toFixed(0)}% (${Math.round(cnt)} ONUs)`;
    }
    const cntOnly = coerceFiniteNumber(cnt);
    if (cntOnly !== null) return fmtByUnit(cntOnly, "ONUs");
  }

  const dbmN = coerceFiniteNumber(m?.dbm);
  if (dbmN !== null) return fmtByUnit(dbmN, "dBm");
  const latN = coerceFiniteNumber(m?.curr_latency_ms);
  if (latN !== null) return fmtByUnit(latN, "ms");
  const cntN = coerceFiniteNumber(m?.drop_online_count);
  if (cntN !== null) return fmtByUnit(cntN, "ONUs");
  const pctN = coerceFiniteNumber(m?.drop_online_pct);
  if (pctN !== null) return fmtByUnit(pctN, "%");
  const upN = coerceFiniteNumber(m?.observed_uptime_minutes);
  if (upN !== null) return fmtByUnit(upN, "min");

  const metricId = String(m?.metric_id ?? "").toLowerCase();
  const generic = coerceFiniteNumber(m?.value);
  if (generic !== null) {
    if (metricId.includes("uptime") || metricId === "uptime_minutes") return fmtByUnit(generic, "min");
    if (t.includes("latency")) return fmtByUnit(generic, "ms");
    if (t.includes("sfp")) return fmtByUnit(generic, "dBm");
    if (metricId.includes("cpu") || t.includes("cpu")) return fmtByUnit(generic, "%");
    if (metricId.includes("mem") || t.includes("memory")) return fmtByUnit(generic, "%");
    if (metricId.includes("temp") || t.includes("telemetry") || t.includes("temperature")) return fmtByUnit(generic, "°C");
    if (t.includes("uptime")) return fmtByUnit(generic, "min");
    if (t.includes("onu") || t.includes("pon")) return fmtByUnit(generic, "ONUs");
    if (t.includes("cpu") || t.includes("memory")) return fmtByUnit(generic, "%");
  }

  if (metricId === "latency_ms") {
    const v = coerceFiniteNumber(m?.value);
    if (v !== null) return fmtByUnit(v, "ms");
  }

  const msg = String(message ?? "");
  const num = msg.match(/(-?\d+(?:[.,]\d+)?)\s*(dBm|ms|°C|%|ONUs?|minutos|min)?/i);
  if (num?.[1]) {
    const n = Number(String(num[1]).replace(",", "."));
    if (Number.isFinite(n)) {
      const unitRaw = String(num[2] ?? "").toLowerCase();
      if (unitRaw === "dbm") return fmtByUnit(n, "dBm");
      if (unitRaw === "ms") return fmtByUnit(n, "ms");
      if (unitRaw === "°c") return fmtByUnit(n, "°C");
      if (unitRaw === "%") return fmtByUnit(n, "%");
      if (unitRaw.startsWith("onu")) return fmtByUnit(n, "ONUs");
      if (unitRaw.startsWith("min")) return fmtByUnit(n, "min");
      if (t.includes("latency")) return fmtByUnit(n, "ms");
      if (t.includes("sfp")) return fmtByUnit(n, "dBm");
      if (t.includes("telemetry") || t.includes("temperature")) return fmtByUnit(n, "°C");
    }
  }
  return "-";
}
