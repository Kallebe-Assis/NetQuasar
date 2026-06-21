import type { ConnectionsFilterState } from "../../lib/connectionsFilters";
import type { ConnectionsViewPrefs } from "../../lib/connectionsPreferences";

/** Props comuns a todas as abas de Conexões. */
export type ConnectionsTabProps = {
  canMutate: boolean;
  filters: ConnectionsFilterState;
  prefs: ConnectionsViewPrefs;
  onSearchChange: (q: string) => void;
  onOpenFilters: () => void;
  onOpenSettings: () => void;
  activeFilterCount: number;
};
