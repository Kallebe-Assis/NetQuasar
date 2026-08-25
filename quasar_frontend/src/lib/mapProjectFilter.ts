/** Valores do filtro de projecto no mapa. */
export const MAP_PROJECT_NONE = "none";
export const MAP_PROJECT_ALL = "all";

export function isMapProjectNone(id: string): boolean {
  return !id.trim() || id.trim() === MAP_PROJECT_NONE;
}

export function isMapProjectAll(id: string): boolean {
  return id.trim() === MAP_PROJECT_ALL;
}

/** UUID do projecto seleccionado, ou null se Nenhum / Todos. */
export function mapProjectUuid(id: string): string | null {
  const v = id.trim();
  if (!v || v === MAP_PROJECT_NONE || v === MAP_PROJECT_ALL) return null;
  return v;
}

export function shouldLoadMapInfrastructure(projectFilterId: string, localityId?: string): boolean {
  if (localityId?.trim()) return true;
  return !isMapProjectNone(projectFilterId);
}
