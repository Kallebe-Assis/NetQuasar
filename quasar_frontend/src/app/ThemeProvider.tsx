import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { apiFetch } from "../lib/api";
import { AUTH_CHANGED_EVENT, getAuthToken } from "../lib/auth";
import { applyUiTheme, readCachedUiTheme, type UiTheme } from "../lib/theme";
import { queryKeys } from "../lib/queryKeys";
import { fetchUiAppearance, normalizeUiAppearanceCacheValue, themeFromAppearancePayload } from "../lib/uiAppearance";
import { fetchMyPreferences } from "../lib/userPreferences";

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
  const [authed, setAuthed] = useState(() => !!getAuthToken());

  useEffect(() => {
    const sync = () => setAuthed(!!getAuthToken());
    window.addEventListener(AUTH_CHANGED_EVENT, sync);
    return () => window.removeEventListener(AUTH_CHANGED_EVENT, sync);
  }, []);

  const prefsQ = useQuery({
    queryKey: queryKeys.mePreferences,
    queryFn: fetchMyPreferences,
    enabled: authed,
    staleTime: 30_000,
    retry: 1,
  });

  const appearanceQ = useQuery({
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
    enabled: !authed,
    staleTime: 30_000,
    retry: 1,
  });

  const appearance = normalizeUiAppearanceCacheValue(appearanceQ.data);
  const theme: UiTheme = authed
    ? (prefsQ.data?.theme ?? cached)
    : themeFromAppearancePayload(appearance?.theme, cached);

  useEffect(() => {
    applyUiTheme(theme);
  }, [theme]);

  const value = useMemo(
    () => ({
      theme,
      isLoading: authed ? prefsQ.isLoading : appearanceQ.isLoading,
      refetch: () => {
        if (authed) void prefsQ.refetch();
        else void appearanceQ.refetch();
      },
    }),
    [theme, authed, prefsQ, appearanceQ],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
