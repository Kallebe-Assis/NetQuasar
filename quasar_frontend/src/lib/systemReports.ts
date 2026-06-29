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

export type SystemReportPayload = {
  report_id: SystemReportId;
  title: string;
  description?: string;
  generated_at: string;
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

export function fetchSystemReport(id: SystemReportId) {
  return apiFetch<SystemReportPayload>(`/api/v1/reports/system/${id}`);
}

export function downloadSystemReportCsv(id: SystemReportId) {
  const token = getAuthToken();
  const url = `/api/v1/reports/system/${id}/csv`;
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

export function sendSystemReportTelegram(id: SystemReportId) {
  return apiFetch<{ ok: boolean }>(`/api/v1/reports/system/${id}/telegram`, { method: "POST" });
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
