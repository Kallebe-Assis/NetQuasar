import { apiFetch } from "./api";
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
  updated_at?: string;
  source?: string;
};

export const DEFAULT_MAP_EQUIPMENT_COLOR = "#3388ff";
export const DEFAULT_MAP_CONNECTION_COLOR = "#3b82f6";
export const DEFAULT_MAP_CTO_COLOR = CTO_MAP_PIN_COLOR.toLowerCase();
export const DEFAULT_MAP_SPLICE_COLOR = DEFAULT_INFRA_MAP_COLORS.splice_box;

export type MapAppearanceColors = {
  equipment: string;
  connection: string;
  cto: string;
  splice_box: string;
};

export function mapColorsFromAppearance(data: UiAppearancePayload | undefined): MapAppearanceColors {
  return {
    equipment: (data?.map_equipment_color ?? DEFAULT_MAP_EQUIPMENT_COLOR).trim() || DEFAULT_MAP_EQUIPMENT_COLOR,
    connection: (data?.map_connection_color ?? DEFAULT_MAP_CONNECTION_COLOR).trim() || DEFAULT_MAP_CONNECTION_COLOR,
    cto: (data?.map_cto_color ?? DEFAULT_MAP_CTO_COLOR).trim() || DEFAULT_MAP_CTO_COLOR,
    splice_box: (data?.map_splice_color ?? DEFAULT_MAP_SPLICE_COLOR).trim() || DEFAULT_MAP_SPLICE_COLOR,
  };
}

export function mapIconsFromAppearance(data: UiAppearancePayload | undefined): MapIconStyles {
  return {
    equipment: normalizeMapPinStyle("equipment", data?.map_equipment_icon ?? DEFAULT_MAP_ICON_STYLES.equipment),
    connection: normalizeMapPinStyle("connection", data?.map_connection_icon ?? DEFAULT_MAP_ICON_STYLES.connection),
    cto: normalizeMapPinStyle("cto", data?.map_cto_icon ?? DEFAULT_MAP_ICON_STYLES.cto),
    splice_box: normalizeMapPinStyle("splice_box", data?.map_splice_icon ?? DEFAULT_MAP_ICON_STYLES.splice_box),
  };
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
