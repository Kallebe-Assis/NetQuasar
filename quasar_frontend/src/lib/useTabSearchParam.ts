import { useSearchParams } from "react-router-dom";

/**
 * Sincroniza a aba activa de uma página com `?tab=` na URL — permite deep-link directo (link
 * partilhável, reload preserva a aba) e é a base para o menu lateral abrir uma página já numa
 * aba específica. Mesmo padrão usado antes em ConnectionsPageShell/SettingsPage/FleetElementsPage,
 * agora extraído para evitar repetir o boilerplate em cada página com abas internas.
 */
export function useTabSearchParam<T extends string>(validTabs: readonly T[], defaultTab: T): [T, (next: T) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const raw = searchParams.get("tab");
  const tab = validTabs.includes(raw as T) ? (raw as T) : defaultTab;

  function selectTab(next: T) {
    const params = new URLSearchParams(searchParams);
    params.set("tab", next);
    setSearchParams(params, { replace: true });
  }

  return [tab, selectTab];
}
