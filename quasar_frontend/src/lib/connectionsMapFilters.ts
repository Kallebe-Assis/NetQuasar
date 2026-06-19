import type { ConnectionsFilterState } from "./connectionsFilters";
import { DEFAULT_CONNECTIONS_FILTERS } from "./connectionsFilters";

const STORAGE_KEY = "netquasar:connections:map-filters";

export function loadMapConnectionFilters(): ConnectionsFilterState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULT_CONNECTIONS_FILTERS };
    const parsed = JSON.parse(raw) as Partial<ConnectionsFilterState>;
    return {
      ...DEFAULT_CONNECTIONS_FILTERS,
      ...parsed,
      logins: { ...DEFAULT_CONNECTIONS_FILTERS.logins, ...parsed.logins },
      ctos: { ...DEFAULT_CONNECTIONS_FILTERS.ctos, ...parsed.ctos },
      splice_boxes: { ...DEFAULT_CONNECTIONS_FILTERS.splice_boxes, ...parsed.splice_boxes },
      cables: { ...DEFAULT_CONNECTIONS_FILTERS.cables, ...parsed.cables },
      poles: { ...DEFAULT_CONNECTIONS_FILTERS.poles, ...parsed.poles },
      projects: { ...DEFAULT_CONNECTIONS_FILTERS.projects, ...parsed.projects },
      visibleKinds: parsed.visibleKinds?.length ? parsed.visibleKinds : DEFAULT_CONNECTIONS_FILTERS.visibleKinds,
    };
  } catch {
    return { ...DEFAULT_CONNECTIONS_FILTERS };
  }
}

export function saveMapConnectionFilters(filters: ConnectionsFilterState): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(filters));
  } catch {
    /* ignore */
  }
}

export function mapShowsLogins(filters: ConnectionsFilterState): boolean {
  return filters.visibleKinds.includes("logins");
}

export function mapShowsInfrastructure(filters: ConnectionsFilterState): boolean {
  return filters.visibleKinds.some((k) => k !== "logins" && k !== "projects");
}
