import type { ReactNode } from "react";
import { Bolt, Funnel, RefreshCw } from "lucide-react";

type Props = {
  children: ReactNode;
  search: string;
  onSearchChange: (q: string) => void;
  searchPlaceholder?: string;
  onOpenFilters: () => void;
  onOpenSettings: () => void;
  activeFilterCount: number;
  onReload?: () => void;
  reloading?: boolean;
  reloadTitle?: string;
  /** CTO: pesquisa → filtro → config → actualizar → extra */
  layout?: "actions-first" | "search-first";
  extraActions?: ReactNode;
};

export function ConnectionsTabToolbar({
  children,
  search,
  onSearchChange,
  searchPlaceholder = "Pesquisar…",
  onOpenFilters,
  onOpenSettings,
  activeFilterCount,
  onReload,
  reloading = false,
  reloadTitle = "Recarregar da base de dados",
  layout = "actions-first",
  extraActions,
}: Props) {
  const searchFirst = layout === "search-first";

  const searchBox = (
    <label className="conn-toolbar__search">
      <input
        className="input"
        type="search"
        aria-label="Pesquisa"
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        placeholder={searchPlaceholder}
        autoComplete="off"
      />
    </label>
  );

  const filterBtn = (
    <button
      type="button"
      className={`btn btn--icon conn-toolbar__icon-btn${activeFilterCount > 0 ? " is-active" : ""}`}
      onClick={onOpenFilters}
      title={activeFilterCount > 0 ? `Filtros (${activeFilterCount})` : "Filtros"}
      aria-label={activeFilterCount > 0 ? `Filtros (${activeFilterCount})` : "Filtros"}
    >
      <Funnel size={16} />
      {activeFilterCount > 0 ? <span className="conn-toolbar__badge">{activeFilterCount}</span> : null}
    </button>
  );

  const settingsBtn = (
    <button
      type="button"
      className="btn btn--icon conn-toolbar__icon-btn"
      onClick={onOpenSettings}
      title="Configurações"
      aria-label="Configurações"
    >
      <Bolt size={16} />
    </button>
  );

  const reloadBtn = onReload ? (
    <button
      type="button"
      className="btn btn--icon conn-toolbar__icon-btn"
      title={reloadTitle}
      aria-label={reloadTitle}
      disabled={reloading}
      onClick={onReload}
    >
      <RefreshCw size={16} className={reloading ? "map-refresh-spin" : undefined} />
    </button>
  ) : null;

  if (searchFirst) {
    return (
      <div className="conn-toolbar conn-toolbar--search-first">
        {searchBox}
        {filterBtn}
        {settingsBtn}
        {reloadBtn}
        {extraActions}
        <div className="conn-toolbar__spacer" aria-hidden />
        {children}
      </div>
    );
  }

  return (
    <div className="conn-toolbar">
      {children}
      <div className="conn-toolbar__spacer" aria-hidden />
      {reloadBtn}
      {searchBox}
      {filterBtn}
      {settingsBtn}
    </div>
  );
}
