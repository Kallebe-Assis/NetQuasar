import type { ConnectionsFilterState } from "./connectionsFilters";
import type { InfraVariant } from "./infraCsvImport";

/** Lista de infraestrutura em cache — evita refetch a cada tecla nos filtros. */
export const NETWORK_INFRA_STALE_MS = 10 * 60 * 1000;
export const NETWORK_INFRA_GC_MS = 30 * 60 * 1000;

type InfraRow = {
  display_number: number;
  description: string;
  project_id?: string | null;
  locality_id?: string | null;
  needs_maintenance?: boolean;
  splitter?: string | null;
  transmitter?: string | null;
  fiber_color?: string | null;
  cable_type?: string | null;
  status?: string;
  pole_type?: string | null;
  fiber_count?: number | null;
};

function matchesInfraSearch(row: InfraRow, q: string, variant: InfraVariant): boolean {
  const desc = row.description.toLowerCase();
  const num = String(row.display_number);
  if (num === q || desc.includes(q)) return true;
  if (variant === "cto") {
    return (row.splitter ?? "").toLowerCase().includes(q) || (row.transmitter ?? "").toLowerCase().includes(q);
  }
  return false;
}

export function filterInfrastructureRows<T extends InfraRow>(
  rows: T[],
  variant: InfraVariant,
  filters: ConnectionsFilterState,
  debouncedQ: string,
): T[] {
  let out = rows;
  const q = debouncedQ.trim().toLowerCase();

  if (filters.project_id) {
    out = out.filter((r) => r.project_id === filters.project_id);
  }
  if (filters.locality_id) {
    out = out.filter((r) => r.locality_id === filters.locality_id);
  }
  if (filters.needs_maintenance) {
    out = out.filter((r) => Boolean(r.needs_maintenance));
  }
  if (q) {
    out = out.filter((r) => matchesInfraSearch(r, q, variant));
  }
  if (variant === "cto") {
    if (filters.ctos.fiber_color) {
      out = out.filter((r) => r.fiber_color === filters.ctos.fiber_color);
    }
    const sp = filters.ctos.splitter.trim().toLowerCase();
    if (sp) {
      out = out.filter((r) => (r.splitter ?? "").toLowerCase().includes(sp));
    }
  }
  if (variant === "cable") {
    const ct = filters.cables.cable_type.trim().toLowerCase();
    if (ct) out = out.filter((r) => (r.cable_type ?? "").toLowerCase().includes(ct));
    if (filters.cables.status) out = out.filter((r) => r.status === filters.cables.status);
  }
  if (variant === "pole") {
    const pt = filters.poles.pole_type.trim().toLowerCase();
    if (pt) out = out.filter((r) => (r.pole_type ?? "").toLowerCase().includes(pt));
  }
  if (variant === "splice" && filters.splice_boxes.fiber_count_min.trim()) {
    const min = Number(filters.splice_boxes.fiber_count_min);
    if (Number.isFinite(min)) {
      out = out.filter((r) => r.fiber_count != null && r.fiber_count >= min);
    }
  }
  return out;
}

export function filterProjectRows<T extends InfraRow & { status?: string }>(
  rows: T[],
  filters: ConnectionsFilterState,
  debouncedQ: string,
): T[] {
  let out = rows;
  const q = debouncedQ.trim().toLowerCase();

  if (filters.locality_id) {
    out = out.filter((r) => r.locality_id === filters.locality_id);
  }
  if (filters.projects.status) {
    out = out.filter((r) => r.status === filters.projects.status);
  }
  if (q) {
    out = out.filter((r) => r.description.toLowerCase().includes(q) || String(r.display_number) === q);
  }
  return out;
}
