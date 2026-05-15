export type UiTheme = "dark" | "light";

export const UI_THEME_STORAGE_KEY = "netquasar-ui-theme";

export function isUiTheme(v: string | null | undefined): v is UiTheme {
  return v === "dark" || v === "light";
}

/** Aplica tema no documento e cache local (evita flash no próximo carregamento). */
export function applyUiTheme(theme: UiTheme): void {
  const root = document.documentElement;
  root.setAttribute("data-theme", theme);
  try {
    localStorage.setItem(UI_THEME_STORAGE_KEY, theme);
  } catch {
    /* ignore */
  }
}

export function readCachedUiTheme(): UiTheme | null {
  try {
    const v = localStorage.getItem(UI_THEME_STORAGE_KEY);
    return isUiTheme(v) ? v : null;
  } catch {
    return null;
  }
}

export function uiThemeLabel(theme: UiTheme): string {
  return theme === "light" ? "Claro" : "Escuro";
}
