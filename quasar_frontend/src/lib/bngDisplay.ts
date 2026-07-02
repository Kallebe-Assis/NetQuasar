import { formatBitrate } from "./formatBitrate";
import { formatBytes } from "./formatBytes";

/** Data/hora legível para coletas BNG (pt-BR, 24h). */
export function formatBngDateTime(iso?: string | null): string {
  if (iso == null || iso === "") return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  return d.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/** Duração em segundos → min, horas ou dias. */
export function formatBngDuration(raw?: string | number | null): string {
  if (raw == null || raw === "") return "—";
  let sec = Number(String(raw).trim());
  if (!Number.isFinite(sec) || sec < 0) {
    const m = String(raw).match(/(\d+)/);
    if (!m) return String(raw);
    sec = Number(m[1]);
  }
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const mins = Math.floor((sec % 3600) / 60);
  if (days > 0) return `${days} d ${hours} h`;
  if (hours > 0) return `${hours} h ${mins} min`;
  if (mins > 0) return `${mins} min`;
  return `${sec} s`;
}

/** Taxa CIR Huawei → Mbps (planos típicos). */
export function formatBngKbitRate(raw?: string | number | null): string {
  const n = Number(String(raw ?? "").trim());
  if (!Number.isFinite(n) || n <= 0) return "Sem limite";
  let bps: number;
  if (n >= 10_000_000) {
    bps = n;
  } else if (n < 10_000) {
    bps = n * 1_000_000;
  } else {
    bps = n * 1000;
  }
  const mbps = bps / 1_000_000;
  if (mbps >= 100) return `${Math.round(mbps)} Mbps`;
  if (mbps >= 10) return `${mbps.toFixed(1)} Mbps`;
  if (mbps >= 1) return `${mbps.toFixed(2)} Mbps`;
  return formatBitrate(bps);
}

/** Contador Huawei Flow64 (×64 bytes) → volume legível. */
export function formatBngFlow64(raw?: string | number | null): string {
  const n = Number(String(raw ?? "").trim());
  if (!Number.isFinite(n) || n < 0) return "—";
  return formatBytes(n * 64);
}

/** Formatação de valores BNG para exibição. */

export function formatBngPercent(raw: unknown): string {
  if (raw == null || raw === "") return "—";
  const s = String(raw).trim().replace(",", ".");
  const n = Number(s);
  if (!Number.isFinite(n)) return s;
  if (n >= 0 && n <= 100) return `${Math.round(n * 10) / 10}%`;
  if (n > 100 && n <= 1000) return `${Math.round((n / 10) * 10) / 10}%`;
  return `${Math.round(n * 10) / 10}%`;
}

export function formatBngUptime(raw: unknown): string {
  if (raw == null || raw === "") return "—";
  const s = String(raw).trim();
  let sec = Number(s);
  if (!Number.isFinite(sec) || sec < 0) {
    const m = s.match(/(\d+)/);
    if (!m) return s;
    sec = Number(m[1]);
  }
  // sysUpTime em centésimos de segundo (SNMP)
  if (sec > 86400 * 365 * 50) {
    sec = Math.floor(sec / 100);
  }
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const mins = Math.floor((sec % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}min`;
  if (mins > 0) return `${mins} min`;
  return `${sec}s`;
}

export function formatBngTemperature(raw: unknown): string {
  if (raw == null || raw === "") return "—";
  const n = Number(String(raw).replace(",", "."));
  if (!Number.isFinite(n)) return String(raw);
  return `${Math.round(n * 10) / 10} °C`;
}

export type OverviewFieldKey = "sys_name" | "sys_uptime" | "cpu_usage" | "memory_usage" | "temperature";

export const OVERVIEW_FIELD_LABELS: Record<OverviewFieldKey, string> = {
  sys_name: "Nome do equipamento",
  sys_uptime: "Uptime",
  cpu_usage: "CPU",
  memory_usage: "Memória",
  temperature: "Temperatura",
};

export function formatOverviewField(key: OverviewFieldKey, raw: unknown): string {
  switch (key) {
    case "sys_uptime":
      return formatBngUptime(raw);
    case "cpu_usage":
    case "memory_usage":
      return formatBngPercent(raw);
    case "temperature":
      return formatBngTemperature(raw);
    default:
      return raw == null || raw === "" ? "—" : String(raw);
  }
}

export const BNG_SESSION_DISPLAY_LIMITS = [10, 25, 50, 100] as const;
export type BngSessionDisplayLimit = (typeof BNG_SESSION_DISPLAY_LIMITS)[number];

export type BngSessionRefreshMode = "manual" | "auto";

export const BNG_SESSION_REFRESH_MODE_KEY = "netquasar.bng.sessions.refreshMode";

const EMPTY_SNMP_DISPLAY = new Set(["", "<nil>", "null", "nil", "undefined"]);

/** Valor de célula PPPoE — oculta «<nil>» e vazios. */
export function bngCellDisplay(raw?: string | null): string {
  if (raw == null) return "—";
  const s = String(raw).trim();
  if (!s || EMPTY_SNMP_DISPLAY.has(s.toLowerCase())) return "—";
  if (s.toLowerCase().startsWith("estado <nil>")) return "—";
  return s;
}

export function formatBngSessionStatus(raw?: string): { label: string; online: boolean } {
  const s = String(raw ?? "Up").trim().toLowerCase();
  const online = s === "up" || s === "online" || s === "1" || s === "true";
  return { label: online ? "Online" : "Offline", online };
}

export function formatBngIpType(
  raw?: string,
  rawCode?: string,
  session?: { ipv4?: string; ipv6?: string; ipv6_pd?: string },
): string {
  const derived = deriveIpTypeFromSession(session);
  if (derived === "ipv4/v6") return derived;
  const code = String(rawCode ?? raw ?? "").trim();
  if (code.length === 3 && code[0] >= "0" && code[0] <= "1" && code[1] >= "0" && code[1] <= "1") {
    const v4 = code[0] === "1";
    const v6 = code[1] === "1";
    if (v4 && v6) return "ipv4/v6";
    if (v6) return "ipv6";
    if (v4 && derived === "ipv4/v6") return derived;
    if (v4) return "ipv4";
  }
  const v = String(raw ?? rawCode ?? "").trim().toLowerCase();
  if (!v) return derived || "—";
  if (v === "ipv4" || v.includes("ipv4 (10)") || v === "1" || v === "01" || v === "10") {
    return derived === "ipv4/v6" ? derived : "ipv4";
  }
  if (v === "ipv6" || v.includes("ipv6 (11)") || v === "2" || v === "02" || v === "11") return "ipv6";
  if (v.includes("dual") || v.includes("ipv4/v6") || v === "3" || v === "03") return "ipv4/v6";
  if (v === "100" && derived) return derived;
  if (v.startsWith("tipo ")) return v.slice(5);
  return derived || v;
}

function deriveIpTypeFromSession(session?: { ipv4?: string; ipv6?: string; ipv6_pd?: string }): string {
  if (!session) return "";
  const has4 = Boolean(String(session.ipv4 ?? "").trim() && session.ipv4 !== "0.0.0.0");
  const has6 = Boolean(String(session.ipv6 ?? "").trim() || String(session.ipv6_pd ?? "").trim());
  if (has4 && has6) return "ipv4/v6";
  if (has6) return "ipv6";
  if (has4) return "ipv4";
  return "";
}

/** Oculta lixo binário; aceita hex compacto de 32 nibbles e prefixos Huawei. */
export function formatBngIpv6Display(raw?: string): string {
  if (raw == null || raw === "") return "—";
  const s = String(raw).trim();
  if (!s) return "—";
  if (s.includes("/")) return s;
  const prefix = parseHuaweiIpv6Prefix(s);
  if (prefix) return prefix;
  if (/^[0-9a-f:]+$/i.test(s) && s.includes(":")) return s;
  if (/^[\da-f]{2}(?::[\da-f]{2})+$/i.test(s)) return s;
  if (/^[0-9a-f.:/]+$/i.test(s) && s.includes(":")) {
    const parts = s.split(":");
    if (parts.length === 16 && parts.every((p) => /^[0-9a-f]{2}$/i.test(p))) {
      const grouped: string[] = [];
      for (let i = 0; i < 16; i += 2) {
        grouped.push(parts.slice(i, i + 2).join(""));
      }
      return grouped.join(":");
    }
    return s;
  }
  const hex = s.replace(/^0x/i, "").replace(/[^0-9a-f]/gi, "");
  if (hex.length === 32) {
    const parts: string[] = [];
    for (let i = 0; i < 32; i += 4) {
      parts.push(hex.slice(i, i + 4));
    }
    return parts.join(":");
  }
  const printable = s.replace(/[\t\n\r .:/0-9a-fA-F]/g, "");
  if (printable.length > 0 && !s.includes(":")) return "—";
  return s;
}

function parseHuaweiIpv6Prefix(s: string): string | null {
  if (!s.includes(":")) return null;
  const parts = s.split(":");
  if (parts.length < 3 || parts.length > 9) return null;
  if (!parts.every((p) => /^[\da-f]{2}$/i.test(p.trim()))) return null;
  const raw = parts.map((p) => parseInt(p, 16));
  const plen = raw[0];
  if (plen < 8 || plen > 128) return null;
  const addr = new Uint8Array(16);
  for (let i = 1; i < raw.length && i - 1 < 16; i++) {
    addr[i - 1] = raw[i];
  }
  const groups: string[] = [];
  for (let i = 0; i < 16; i += 2) {
    groups.push(((addr[i] << 8) | addr[i + 1]).toString(16));
  }
  let compact = groups.join(":");
  compact = compact.replace(/(^|:)0(:0)+(:|$)/, "$1::$2").replace(/:{3,}/, "::");
  return `${compact}/${plen}`;
}

export type StatsSeriesKey = "total_online" | "pppoe_online" | "ipv4_online" | "ipv6_online" | "dual_stack_online";

export const STATS_SERIES: { key: StatsSeriesKey; color: string; label: string }[] = [
  { key: "total_online", color: "#64748b", label: "Total online" },
  { key: "pppoe_online", color: "#3b82f6", label: "PPPoE online" },
  { key: "ipv4_online", color: "#22c55e", label: "IPv4 online" },
  { key: "ipv6_online", color: "#a855f7", label: "IPv6 online" },
  { key: "dual_stack_online", color: "#f59e0b", label: "Dual-stack" },
];

export type PppoeSessionFields = {
  online_time?: string;
  online_time_sec?: string;
  car_up_cir_kbps?: string;
  car_dn_cir_kbps?: string;
  car_up_cir_display?: string;
  car_dn_cir_display?: string;
  up_flow_bytes?: string;
  dn_flow_bytes?: string;
  up_flow_display?: string;
  dn_flow_display?: string;
  vlan?: string;
};

export function sessionDisplayOnline(s?: PppoeSessionFields | null): string {
  if (!s) return "—";
  if (s.online_time?.trim()) return s.online_time;
  return formatBngDuration(s.online_time_sec);
}

export function sessionDisplayUpLimit(s?: PppoeSessionFields | null): string {
  if (!s) return "—";
  if (s.car_up_cir_display?.trim()) return s.car_up_cir_display;
  return formatBngKbitRate(s.car_up_cir_kbps);
}

export function sessionDisplayDnLimit(s?: PppoeSessionFields | null): string {
  if (!s) return "—";
  if (s.car_dn_cir_display?.trim()) return s.car_dn_cir_display;
  return formatBngKbitRate(s.car_dn_cir_kbps);
}

export function sessionDisplayUpFlow(s?: PppoeSessionFields | null): string {
  if (!s) return "—";
  if (s.up_flow_display?.trim()) return s.up_flow_display;
  return formatBngFlow64(s.up_flow_bytes);
}

export function sessionDisplayDnFlow(s?: PppoeSessionFields | null): string {
  if (!s) return "—";
  if (s.dn_flow_display?.trim()) return s.dn_flow_display;
  return formatBngFlow64(s.dn_flow_bytes);
}
