import { buildExcelCsvBlob } from "./excelCsv";
import { apiFetch } from "./api";
import { getAuthToken } from "./auth";

export type SystemReportId =
  | "active-alerts"
  | "connections"
  | "equipment-by-pop"
  | "olt-overview"
  | "system-general"
  | "integrations"
  | "attention-devices"
  | "alerts-by-category"
  | "onu-per-pon"
  | "bng-subscribers";

export type SystemReportCatalogItem = {
  id: SystemReportId;
  title: string;
  description: string;
};

export type EquipmentByPopReportOptions = {
  include_without_pop?: boolean;
  include_pop_coordinates?: boolean;
};

export type ConnectionsReportOptions = {
  mode?: "summary" | "detailed";
  source?: "connections" | "bng_cache";
  bng_device_id?: string;
};

export type OltOverviewReportOptions = {
  period?: "today" | "3d" | "7d" | "30d";
};

export type SystemReportOptions =
  | EquipmentByPopReportOptions
  | ConnectionsReportOptions
  | OltOverviewReportOptions;

export type SystemReportPayload = {
  report_id: SystemReportId;
  title: string;
  description?: string;
  generated_at: string;
  options?: SystemReportOptions;
  summary?: Record<string, unknown>;
  columns?: string[];
  rows?: string[][];
  chart?: {
    label?: string;
    kind?: string;
    points?: Array<{
      t: string;
      collected_at?: string;
      device?: string;
      total?: number;
      online?: number;
      offline?: number;
      pppoe?: number;
      ipv4?: number;
      ipv6?: number;
      dual_stack?: number;
    }>;
  };
  averages?: {
    windows?: Array<{
      days: number;
      label: string;
      samples: number;
      total?: number;
      pppoe?: number;
      ipv4?: number;
      ipv6?: number;
      dual_stack?: number;
    }>;
  };
  groups?: Array<{
    pop?: string;
    latitude?: number;
    longitude?: number;
    coordinates?: string;
    devices?: Array<{
      name?: string;
      category?: string;
      label?: string;
    }>;
  }>;
};

export const SYSTEM_REPORT_IDS: SystemReportId[] = [
  "active-alerts",
  "connections",
  "equipment-by-pop",
  "olt-overview",
  "system-general",
  "integrations",
  "attention-devices",
  "alerts-by-category",
  "onu-per-pon",
  "bng-subscribers",
];

export function fetchSystemReportCatalog() {
  return apiFetch<{ reports: SystemReportCatalogItem[] }>("/api/v1/reports/system");
}

function equipmentByPopQuery(opts?: EquipmentByPopReportOptions): string {
  const p = new URLSearchParams();
  if (opts?.include_without_pop) p.set("include_without_pop", "1");
  if (opts?.include_pop_coordinates) p.set("include_pop_coordinates", "1");
  const qs = p.toString();
  return qs ? `?${qs}` : "";
}

function connectionsQuery(opts?: ConnectionsReportOptions): string {
  const p = new URLSearchParams();
  if (opts?.mode) p.set("mode", opts.mode);
  if (opts?.source) p.set("source", opts.source);
  if (opts?.bng_device_id) p.set("bng_device_id", opts.bng_device_id);
  const qs = p.toString();
  return qs ? `?${qs}` : "";
}

function oltOverviewQuery(opts?: OltOverviewReportOptions): string {
  const p = new URLSearchParams();
  if (opts?.period) p.set("period", opts.period);
  const qs = p.toString();
  return qs ? `?${qs}` : "";
}

function reportQuery(
  id: SystemReportId,
  opts?: EquipmentByPopReportOptions | ConnectionsReportOptions | OltOverviewReportOptions,
): string {
  if (id === "equipment-by-pop") return equipmentByPopQuery(opts as EquipmentByPopReportOptions);
  if (id === "connections") return connectionsQuery(opts as ConnectionsReportOptions);
  if (id === "olt-overview") return oltOverviewQuery(opts as OltOverviewReportOptions);
  return "";
}

export function fetchSystemReport(
  id: SystemReportId,
  opts?: EquipmentByPopReportOptions | ConnectionsReportOptions | OltOverviewReportOptions,
) {
  return apiFetch<SystemReportPayload>(`/api/v1/reports/system/${id}${reportQuery(id, opts)}`);
}

export function downloadSystemReportCsv(
  id: SystemReportId,
  opts?: EquipmentByPopReportOptions | ConnectionsReportOptions | OltOverviewReportOptions,
) {
  const token = getAuthToken();
  const url = `/api/v1/reports/system/${id}/csv${reportQuery(id, opts)}`;
  return fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  }).then(async (res) => {
    if (!res.ok) {
      const txt = await res.text();
      throw new Error(txt || `HTTP ${res.status}`);
    }
    const blob = await res.blob();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `relatorio_${id}.csv`;
    a.click();
    URL.revokeObjectURL(a.href);
  });
}

export function downloadSystemReportCsvClient(payload: SystemReportPayload) {
  const cols = payload.columns ?? [];
  const rows = payload.rows ?? [];
  const blob = buildExcelCsvBlob([cols, ...rows]);
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `relatorio_${payload.report_id}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
}

export function sendSystemReportTelegram(
  id: SystemReportId,
  opts?: EquipmentByPopReportOptions | ConnectionsReportOptions | OltOverviewReportOptions,
) {
  const body =
    id === "equipment-by-pop" || id === "connections" || id === "olt-overview" ? opts ?? {} : undefined;
  return apiFetch<{ ok: boolean }>(`/api/v1/reports/system/${id}/telegram`, {
    method: "POST",
    json: body,
  });
}

export function summaryEntries(summary: Record<string, unknown> | undefined): [string, string][] {
  if (!summary) return [];
  return Object.entries(summary).map(([k, v]) => {
    if (v != null && typeof v === "object" && !Array.isArray(v)) {
      const inner = Object.entries(v as Record<string, unknown>)
        .map(([ik, iv]) => `${ik}: ${iv}`)
        .join(", ");
      return [k, inner || "—"];
    }
    if (Array.isArray(v)) return [k, v.join(", ")];
    return [k, String(v ?? "—")];
  });
}
