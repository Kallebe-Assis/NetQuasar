import { useEffect } from "react";
import { applyUiTheme, type UiTheme } from "../lib/theme";

/**
 * Aplica `preview` no documento enquanto o componente está montado;
 * ao desmontar, restaura `restoreTheme` (tema activo no servidor / contexto).
 */
export function useThemePreview(preview: UiTheme, restoreTheme: UiTheme): void {
  useEffect(() => {
    applyUiTheme(preview);
  }, [preview]);

  useEffect(() => {
    return () => applyUiTheme(restoreTheme);
  }, [restoreTheme]);
}
