export type ConnectionsTabId = "logins" | "cto" | "splice" | "cables" | "poles" | "projects";

export type InfrastructureElementKind = "logins" | "ctos" | "splice_boxes" | "cables" | "poles" | "projects";

export type ConnectionsFilterState = {
  q: string;
  project_id: string;
  locality_id: string;
  needs_maintenance: boolean;
  /** Tipos visíveis no mapa / consultas cruzadas */
  visibleKinds: InfrastructureElementKind[];
  logins: {
    connection_kind: string;
    medium_type: string;
    cto: string;
  };
  ctos: {
    fiber_color: string;
    splitter: string;
  };
  splice_boxes: {
    fiber_count_min: string;
  };
  cables: {
    cable_type: string;
    status: string;
  };
  poles: {
    pole_type: string;
  };
  projects: {
    status: string;
  };
};

export const DEFAULT_CONNECTIONS_FILTERS: ConnectionsFilterState = {
  q: "",
  project_id: "",
  locality_id: "",
  needs_maintenance: false,
  visibleKinds: ["logins", "ctos", "splice_boxes", "cables", "poles", "projects"],
  logins: { connection_kind: "", medium_type: "", cto: "" },
  ctos: { fiber_color: "", splitter: "" },
  splice_boxes: { fiber_count_min: "" },
  cables: { cable_type: "", status: "" },
  poles: { pole_type: "" },
  projects: { status: "" },
};

export const ELEMENT_KIND_LABELS: Record<InfrastructureElementKind, string> = {
  logins: "Logins",
  ctos: "CTOs",
  splice_boxes: "Caixas de emenda",
  cables: "Cabos",
  poles: "Postes",
  projects: "Projetos",
};

export function countActiveFilters(f: ConnectionsFilterState, tab: ConnectionsTabId): number {
  let n = 0;
  if (f.q.trim()) n++;
  if (f.project_id) n++;
  if (f.locality_id) n++;
  if (f.needs_maintenance) n++;
  if (tab === "logins") {
    if (f.logins.connection_kind) n++;
    if (f.logins.medium_type) n++;
    if (f.logins.cto.trim()) n++;
  }
  if (tab === "cto") {
    if (f.ctos.fiber_color) n++;
    if (f.ctos.splitter.trim()) n++;
  }
  if (tab === "splice" && f.splice_boxes.fiber_count_min.trim()) n++;
  if (tab === "cables") {
    if (f.cables.cable_type.trim()) n++;
    if (f.cables.status) n++;
  }
  if (tab === "poles" && f.poles.pole_type.trim()) n++;
  if (tab === "projects" && f.projects.status) n++;
  return n;
}

export function filtersToQueryParams(f: ConnectionsFilterState, tab: ConnectionsTabId): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.project_id) p.set("project_id", f.project_id);
  if (f.locality_id) p.set("locality_id", f.locality_id);
  if (f.needs_maintenance) p.set("needs_maintenance", "1");
  if (tab === "cto") {
    if (f.ctos.fiber_color) p.set("fiber_color", f.ctos.fiber_color);
    if (f.ctos.splitter.trim()) p.set("splitter", f.ctos.splitter.trim());
  }
  if (tab === "cables") {
    if (f.cables.cable_type.trim()) p.set("cable_type", f.cables.cable_type.trim());
    if (f.cables.status) p.set("status", f.cables.status);
  }
  if (tab === "poles" && f.poles.pole_type.trim()) p.set("pole_type", f.poles.pole_type.trim());
  if (tab === "projects" && f.projects.status) p.set("status", f.projects.status);
  return p;
}
