import { apiFetch } from "./api";
import { CABLE_FUNCOES, type CableFuncao } from "./networkInfrastructure";
import { CTO_MAP_PIN_COLOR, DEFAULT_INFRA_MAP_COLORS, DEFAULT_MAP_ICON_STYLES, type MapIconStyles } from "./mapInfrastructureIcons";
import { normalizeMapPinStyle } from "./mapPinStyles";
import { isUiTheme, type UiTheme } from "./theme";

export type UiAppearancePayload = {
  theme?: string;
  map_equipment_color?: string;
  map_connection_color?: string;
  map_cto_color?: string;
  map_splice_color?: string;
  map_equipment_icon?: string;
  map_connection_icon?: string;
  map_cto_icon?: string;
  map_splice_icon?: string;
  // Foguete de emenda e foguete de distribuição têm cor/ícone próprios — o mapa em si usa
  // sempre estes 4 campos (map_splice_color/icon acima ficam só de referência histórica).
  map_splice_emenda_color?: string;
  map_splice_distribuicao_color?: string;
  map_splice_emenda_icon?: string;
  map_splice_distribuicao_icon?: string;
  // Cor por função de cabo — chave = CableFuncao, valor = hex. Sempre vem completo (6 chaves) do
  // backend (ver mergeCableFuncaoColors no handler Go).
  map_cable_funcao_colors?: Partial<Record<CableFuncao, string>>;
  // Ícone/imagem/visibilidade por categoria de equipamento (Concentrador, Energia, Mikrotik...).
  map_equipment_categories?: Record<string, EquipmentCategoryConfig>;
  // URL de imagem importada por tipo (substitui o ícone do catálogo) — chaves: equipment,
  // connection, cto, splice_emenda, splice_distribuicao.
  map_icon_image_urls?: Partial<Record<MapIconImageRole, string>>;
  updated_at?: string;
  source?: string;
};

export type EquipmentCategoryConfig = { icon?: string; image_url?: string; hidden?: boolean };

export type MapIconImageRole = "equipment" | "connection" | "cto" | "splice_emenda" | "splice_distribuicao";

/** Mesmas 9 categorias de lib/deviceCategoryIcons.ts / MapFilterModal.tsx. */
export const MAP_EQUIPMENT_CATEGORIES = [
  "Concentrador",
  "Energia",
  "Mikrotik",
  "Switch",
  "OLT",
  "Rádio",
  "Servidor",
  "Máquina Virtual",
  "Outros",
] as const;

export const DEFAULT_MAP_EQUIPMENT_COLOR = "#3388ff";
export const DEFAULT_MAP_CONNECTION_COLOR = "#3b82f6";
export const DEFAULT_MAP_CTO_COLOR = CTO_MAP_PIN_COLOR.toLowerCase();
export const DEFAULT_MAP_SPLICE_COLOR = DEFAULT_INFRA_MAP_COLORS.splice_box;
export const DEFAULT_MAP_SPLICE_EMENDA_COLOR = "#d97706";
export const DEFAULT_MAP_SPLICE_DISTRIBUICAO_COLOR = "#7c3aed";

/** Espelha defaultMapCableFuncaoColors em handlers_settings_ui.go — só usado antes da 1ª resposta
 * da API chegar (a API já devolve sempre as 6 chaves preenchidas). */
export const DEFAULT_MAP_CABLE_FUNCAO_COLORS: Record<CableFuncao, string> = {
  backbone_link: "#0891b2",
  transporte: "#16a34a",
  backbone_ftth: "#2563eb",
  cto: "#eab308",
  multipla: "#dc2626",
  outro: "#64748b",
};

/** Cores de todos os elementos do mapa — o mesmo objecto alimenta tanto `EquipmentMap`
 * (prop `colors: MapColors`, que só lê os campos que conhece) como o modal de Configurações. */
export type MapAppearanceColors = {
  equipment: string;
  connection: string;
  cto: string;
  splice_box: string;
  splice_box_emenda: string;
  splice_box_distribuicao: string;
  cable_funcao: Record<CableFuncao, string>;
};

export function mapColorsFromAppearance(data: UiAppearancePayload | undefined): MapAppearanceColors {
  const cableFuncao = { ...DEFAULT_MAP_CABLE_FUNCAO_COLORS };
  const stored = data?.map_cable_funcao_colors;
  if (stored) {
    for (const f of CABLE_FUNCOES) {
      const v = stored[f.value];
      if (v && /^#[0-9a-fA-F]{6}$/.test(v.trim())) cableFuncao[f.value] = v.trim().toLowerCase();
    }
  }
  return {
    equipment: (data?.map_equipment_color ?? DEFAULT_MAP_EQUIPMENT_COLOR).trim() || DEFAULT_MAP_EQUIPMENT_COLOR,
    connection: (data?.map_connection_color ?? DEFAULT_MAP_CONNECTION_COLOR).trim() || DEFAULT_MAP_CONNECTION_COLOR,
    cto: (data?.map_cto_color ?? DEFAULT_MAP_CTO_COLOR).trim() || DEFAULT_MAP_CTO_COLOR,
    splice_box: (data?.map_splice_color ?? DEFAULT_MAP_SPLICE_COLOR).trim() || DEFAULT_MAP_SPLICE_COLOR,
    splice_box_emenda: (data?.map_splice_emenda_color ?? DEFAULT_MAP_SPLICE_EMENDA_COLOR).trim() || DEFAULT_MAP_SPLICE_EMENDA_COLOR,
    splice_box_distribuicao:
      (data?.map_splice_distribuicao_color ?? DEFAULT_MAP_SPLICE_DISTRIBUICAO_COLOR).trim() ||
      DEFAULT_MAP_SPLICE_DISTRIBUICAO_COLOR,
    cable_funcao: cableFuncao,
  };
}

export function mapIconsFromAppearance(data: UiAppearancePayload | undefined): MapIconStyles {
  return {
    equipment: normalizeMapPinStyle("equipment", data?.map_equipment_icon ?? DEFAULT_MAP_ICON_STYLES.equipment),
    connection: normalizeMapPinStyle("connection", data?.map_connection_icon ?? DEFAULT_MAP_ICON_STYLES.connection),
    cto: normalizeMapPinStyle("cto", data?.map_cto_icon ?? DEFAULT_MAP_ICON_STYLES.cto),
    splice_box: normalizeMapPinStyle("splice_box", data?.map_splice_icon ?? DEFAULT_MAP_ICON_STYLES.splice_box),
    splice_box_emenda: normalizeMapPinStyle("splice_box", data?.map_splice_emenda_icon ?? DEFAULT_MAP_ICON_STYLES.splice_box_emenda),
    splice_box_distribuicao: normalizeMapPinStyle(
      "splice_box",
      data?.map_splice_distribuicao_icon ?? DEFAULT_MAP_ICON_STYLES.splice_box_distribuicao,
    ),
  };
}

export function mapEquipmentCategoriesFromAppearance(
  data: UiAppearancePayload | undefined,
): Record<string, EquipmentCategoryConfig> {
  const stored = data?.map_equipment_categories ?? {};
  const out: Record<string, EquipmentCategoryConfig> = {};
  for (const cat of MAP_EQUIPMENT_CATEGORIES) {
    const cfg = stored[cat];
    if (cfg) out[cat] = { icon: cfg.icon, image_url: cfg.image_url, hidden: !!cfg.hidden };
  }
  return out;
}

export function mapIconImageUrlsFromAppearance(
  data: UiAppearancePayload | undefined,
): Partial<Record<MapIconImageRole, string>> {
  return { ...(data?.map_icon_image_urls ?? {}) };
}

export async function fetchUiAppearance(): Promise<UiAppearancePayload> {
  return apiFetch<UiAppearancePayload>("/api/v1/settings/ui-appearance");
}

export function themeFromAppearancePayload(raw: string | undefined, fallback: UiTheme): UiTheme {
  const v = (raw ?? "").trim().toLowerCase();
  if (v === "light") return "light";
  if (v === "dark") return "dark";
  return fallback;
}

/** Garante que o valor em cache do React Query é sempre o payload da API (nunca só a string do tema). */
export function normalizeUiAppearanceCacheValue(data: unknown): UiAppearancePayload | undefined {
  if (data == null) return undefined;
  if (typeof data === "string") {
    const theme = isUiTheme(data) ? data : undefined;
    return theme ? { theme } : undefined;
  }
  if (typeof data === "object" && data !== null && "theme" in data) {
    return data as UiAppearancePayload;
  }
  return undefined;
}
