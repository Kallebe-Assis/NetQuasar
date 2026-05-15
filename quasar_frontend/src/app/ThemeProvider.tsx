import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useMemo, type ReactNode } from "react";
import { apiFetch } from "../lib/api";
import { getAuthToken } from "../lib/auth";
import { applyUiTheme, readCachedUiTheme, type UiTheme } from "../lib/theme";
import { queryKeys } from "../lib/queryKeys";
import { fetchUiAppearance, normalizeUiAppearanceCacheValue, themeFromAppearancePayload } from "../lib/uiAppearance";

type ThemeContextValue = {
  theme: UiTheme;
  isLoading: boolean;
  refetch: () => void;
};

const ThemeContext = createContext<ThemeContextValue>({
  theme: "dark",
  isLoading: true,
  refetch: () => {},
});

export function useUiTheme() {
  return useContext(ThemeContext);
}

type SetupStatusResponse = { database_configured?: boolean; ui_theme?: string };

export function ThemeProvider({ children }: { children: ReactNode }) {
  const cached = readCachedUiTheme() ?? "dark";

  const q = useQuery({
    queryKey: queryKeys.uiAppearance,
    queryFn: async () => {
      if (getAuthToken()) {
        return fetchUiAppearance();
      }
      const setup = await apiFetch<SetupStatusResponse>("/api/v1/setup/status");
      if (setup.database_configured) {
        return { theme: setup.ui_theme };
      }
      return { theme: cached };
    },
    staleTime: 30_000,
    retry: 1,
  });

  const appearance = normalizeUiAppearanceCacheValue(q.data);
  const theme = themeFromAppearancePayload(appearance?.theme, cached);

  useEffect(() => {
    applyUiTheme(theme);
  }, [theme]);

  const value = useMemo(
    () => ({
      theme,
      isLoading: q.isLoading,
      refetch: () => void q.refetch(),
    }),
    [theme, q.isLoading, q],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
