import type { ActiveEquipmentRow, MonitorDeviceKPIs, MonitorReachability } from "./types";

/** Extrai reachability de uma linha de equipamento activo. */
export function reachabilityFromActiveRow(row: ActiveEquipmentRow): MonitorReachability {
  return {
    online: row.ping_reachable === true,
    latency_ms: row.latency_ms ?? null,
    checked_at: row.checked_at ?? null,
    ping_fail_streak: row.ping_fail_streak ?? 0,
  };
}

/** Extrai KPIs de uma linha de equipamento activo. */
export function kpisFromActiveRow(row: ActiveEquipmentRow): MonitorDeviceKPIs {
  return {
    cpu_percent: row.cpu_percent ?? null,
    memory_percent: row.memory_percent ?? null,
    temperature_c: row.temperature_c ?? null,
    uptime: row.uptime ?? null,
    collected_at: row.telemetry_collected_at ?? null,
  };
}

/** Online/offline simples para badge da UI. */
export function isDeviceOnline(row: Pick<ActiveEquipmentRow, "ping_reachable"> | null | undefined): boolean {
  return row?.ping_reachable === true;
}

/** Cor de status operacional de interface. */
export function ifaceOperColor(status?: string | null): "ok" | "danger" | "muted" {
  const s = String(status ?? "").toLowerCase();
  if (s === "up") return "ok";
  if (s === "down") return "danger";
  return "muted";
}

/** Nome legível de interface. */
export function ifaceDisplayName(row: {
  display_name?: string | null;
  if_name?: string | null;
  descr?: string | null;
  name?: string | null;
  if_index?: number;
}): string {
  const n = row.display_name ?? row.if_name ?? row.descr ?? row.name;
  if (n && String(n).trim()) return String(n).trim();
  if (row.if_index != null) return `if${row.if_index}`;
  return "—";
}
