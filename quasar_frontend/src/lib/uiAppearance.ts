import { apiFetch } from "./api";
import { isUiTheme, type UiTheme } from "./theme";

export type UiAppearancePayload = {
  theme?: string;
  map_equipment_color?: string;
  map_connection_color?: string;
  updated_at?: string;
  source?: string;
};

export const DEFAULT_MAP_EQUIPMENT_COLOR = "#3388ff";
export const DEFAULT_MAP_CONNECTION_COLOR = "#3b82f6";

export function mapColorsFromAppearance(data: UiAppearancePayload | undefined) {
  return {
    equipment: (data?.map_equipment_color ?? DEFAULT_MAP_EQUIPMENT_COLOR).trim() || DEFAULT_MAP_EQUIPMENT_COLOR,
    connection: (data?.map_connection_color ?? DEFAULT_MAP_CONNECTION_COLOR).trim() || DEFAULT_MAP_CONNECTION_COLOR,
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
