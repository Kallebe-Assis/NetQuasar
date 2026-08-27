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

/**
 * Se deve pedir infraestrutura (CTOs, cabos, postes, caixas de emenda/foguete, POPs) ao backend.
 *
 * Antes: com filtro de projecto em "Nenhum" (o valor por omissão ao abrir o mapa) isto devolvia
 * `false` e a infraestrutura NUNCA era pedida — mesmo com as camadas "CTOs"/"POPs" activas por
 * omissão nos toggles do mapa. Isso fazia CTOs/cabos/postes/foguetes parecerem "desaparecidos":
 * só as CTOs mais próximas via GPS (endpoint /map/nearest-ctos, que não depende deste gate)
 * continuavam visíveis. Ver DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md, secção "Mapa".
 *
 * Agora a decisão de pedir infraestrutura cabe só às camadas activas (showCtos/showCables/etc.)
 * e ao viewport actual — o projecto/localidade seleccionados continuam a filtrar o resultado
 * (via project_id/locality_id na query), não a decidir se ele é pedido.
 */
export function shouldLoadMapInfrastructure(_projectFilterId?: string, _localityId?: string): boolean {
  return true;
}
