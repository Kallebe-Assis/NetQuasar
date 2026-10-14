import type { ActiveEquipmentRow } from "./types";

/** Online/offline simples para badge da UI. */
export function isDeviceOnline(row: Pick<ActiveEquipmentRow, "ping_reachable"> | null | undefined): boolean {
  return row?.ping_reachable === true;
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
