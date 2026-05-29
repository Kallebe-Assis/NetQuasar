import { EM_DASH, formatDbm } from "./formatDisplay";
import { formatBytes, formatKilobytes, formatUptimeTicks } from "./formatBytes";

export type MikrotikCatalogEntry = {
  key: string;
  section: string;
  label: string;
  unit?: string;
  default_divisor?: number;
};

export type MikrotikMetricConfig = {
  enabled?: boolean;
  oid?: string;
  collect_mode?: string;
  value_divisor?: number;
};

type CollectionFieldRaw = {
  key?: string;
  label?: string;
  ok?: boolean;
  error?: string;
  value?: unknown;
  value_divisor?: number;
  collect_mode?: string;
  optical_ports?: Array<Record<string, unknown>>;
  interface_status?: Array<Record<string, unknown>>;
  pppoe_sessions?: Array<Record<string, unknown>>;
  walk?: { row_count?: number; truncated?: boolean };
};

export type MikrotikMetricCard =
  | { kind: "scalar"; key: string; label: string; section: string; value: string; error?: string; ok: boolean }
  | { kind: "optical"; key: string; label: string; section: string; ports: Array<Record<string, unknown>>; ok: boolean; error?: string }
  | { kind: "interface_status"; key: string; label: string; section: string; rows: Array<Record<string, unknown>>; ok: boolean; error?: string }
  | { kind: "pppoe"; key: string; label: string; section: string; sessions: Array<Record<string, unknown>>; ok: boolean; error?: string }
  | { kind: "count"; key: string; label: string; section: string; count: number; note?: string; ok: boolean; error?: string };

const SECTION_ORDER = ["system", "health", "interfaces", "optical", "ppp", "wireless", "users", "dhcp", "ip"];

function formatScalarValue(key: string, value: unknown, unit?: string, divisor?: number): string {
  if (value == null) return EM_DASH;
  if (key === "sys_uptime") return formatUptimeTicks(value);
  let n = Number(value);
  if (Number.isFinite(n) && divisor && divisor > 1) {
    n = n / divisor;
  }
  if (unit === "°C" || unit === "°c") {
    return Number.isFinite(n) ? `${n.toFixed(1)} °C` : String(value);
  }
  if (unit === "V") {
    return Number.isFinite(n) ? `${n.toFixed(2)} V` : String(value);
  }
  if (unit === "%") {
    return Number.isFinite(n) ? `${n.toFixed(1)}%` : String(value);
  }
  if (unit === "KB" && Number.isFinite(n)) {
    return formatKilobytes(n);
  }
  if (unit === "dBm" && Number.isFinite(n)) {
    return formatDbm(n);
  }
  if (typeof value === "string") return value.trim() || EM_DASH;
  if (Number.isFinite(n)) {
    const abs = Math.abs(n);
    if (abs >= 1_000_000) return formatBytes(n);
    if (abs >= 1000) return n.toLocaleString("pt-BR", { maximumFractionDigits: 2 });
    if (Number.isInteger(n)) return String(n);
    return n.toLocaleString("pt-BR", { maximumFractionDigits: 3 });
  }
  return String(value);
}

function fieldFromRaw(
  key: string,
  catalog: MikrotikCatalogEntry | undefined,
  raw: CollectionFieldRaw,
): MikrotikMetricCard | null {
  const label = raw.label || catalog?.label || key;
  const section = catalog?.section || "other";
  const ok = raw.ok === true;
  const error = raw.error;

  if (raw.pppoe_sessions != null) {
    return { kind: "pppoe", key, label, section, sessions: raw.pppoe_sessions, ok, error };
  }
  if (raw.optical_ports != null) {
    return { kind: "optical", key, label, section, ports: raw.optical_ports, ok, error };
  }
  if (raw.interface_status != null && raw.interface_status.length > 0) {
    return { kind: "interface_status", key, label, section, rows: raw.interface_status, ok, error };
  }

  const walkRows = raw.walk?.row_count;
  if (walkRows != null && walkRows > 0 && raw.value != null && typeof raw.value === "number") {
    return {
      kind: "count",
      key,
      label,
      section,
      count: Number(raw.value),
      note: raw.walk?.truncated ? "walk truncado" : `${walkRows} linhas SNMP`,
      ok,
      error,
    };
  }

  if (raw.value != null && typeof raw.value === "number" && !Number.isInteger(raw.value) === false) {
    const count = Number(raw.value);
    if (count > 0 && (key.includes("table") || key.includes("_count"))) {
      return { kind: "count", key, label, section, count, ok, error };
    }
  }

  const div = raw.value_divisor || catalog?.default_divisor;
  const scalar = formatScalarValue(key, raw.value, catalog?.unit, div);
  return { kind: "scalar", key, label, section, value: scalar, ok, error };
}

/** Cartões de métricas activas no perfil e presentes na última coleta. */
export function buildMikrotikMetricCards(args: {
  metrics?: Record<string, unknown>;
  catalog: MikrotikCatalogEntry[];
  config: Record<string, MikrotikMetricConfig>;
}): MikrotikMetricCard[] {
  const { metrics, catalog, config } = args;
  if (!metrics) return [];
  const coll = metrics.mikrotik_collection as { fields?: Record<string, CollectionFieldRaw> } | undefined;
  const fields = coll?.fields;
  if (!fields) return [];

  const catalogByKey = Object.fromEntries(catalog.map((c) => [c.key, c]));
  const cards: MikrotikMetricCard[] = [];

  for (const entry of catalog) {
    const cfg = config[entry.key];
    if (!cfg?.enabled) continue;
    const raw = fields[entry.key];
    if (!raw) continue;
    const card = fieldFromRaw(entry.key, catalogByKey[entry.key], raw);
    if (card) cards.push(card);
  }

  cards.sort((a, b) => {
    const ia = SECTION_ORDER.indexOf(a.section);
    const ib = SECTION_ORDER.indexOf(b.section);
    if (ia !== ib) return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
    return a.label.localeCompare(b.label, "pt");
  });
  return cards;
}

export function sectionLabel(section: string, labels: Record<string, string>): string {
  return labels[section] || section;
}

export function stripPPPoEName(name: string): string {
  return name.replace(/^<pppoe-?/i, "").replace(/>$/g, "").trim() || name;
}
