import type { ReactNode } from "react";
import { RefreshCw } from "lucide-react";

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
}: Props) {
  return (
    <div className="conn-toolbar">
      {children}
      <div className="conn-toolbar__spacer" aria-hidden />
      {onReload ? (
        <button
          type="button"
          className="btn btn--icon"
          title={reloadTitle}
          aria-label={reloadTitle}
          disabled={reloading}
          onClick={onReload}
        >
          <RefreshCw size={16} className={reloading ? "map-refresh-spin" : undefined} />
        </button>
      ) : null}
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
      <button type="button" className="btn" onClick={onOpenFilters}>
        Filtros{activeFilterCount > 0 ? ` (${activeFilterCount})` : ""}
      </button>
      <button type="button" className="btn" onClick={onOpenSettings}>
        Configurações
      </button>
    </div>
  );
}
