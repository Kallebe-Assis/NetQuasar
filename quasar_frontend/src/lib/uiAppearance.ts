import { apiFetch } from "./api";
import { isUiTheme, type UiTheme } from "./theme";

export type UiAppearancePayload = {
  theme?: string;
  updated_at?: string;
  source?: string;
};

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
