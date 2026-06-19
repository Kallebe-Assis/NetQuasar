import { useMemo, useState } from "react";
import { isAdminUser } from "../../lib/auth";
import {
  DEFAULT_CONNECTIONS_FILTERS,
  countActiveFilters,
  type ConnectionsFilterState,
  type ConnectionsTabId,
} from "../../lib/connectionsFilters";
import { saveMapConnectionFilters, loadMapConnectionFilters } from "../../lib/connectionsMapFilters";
import {
  loadConnectionsPrefs,
  saveConnectionsPrefs,
  type ConnectionsViewPrefs,
} from "../../lib/connectionsPreferences";
import { InfoHint } from "../../components/InfoHint";
import { CommercialConnectionsTab } from "../commercial/CommercialConnectionsTab";
import { ConnectionsFilterDrawer } from "./ConnectionsFilterDrawer";
import { ConnectionsSettingsDrawer } from "./ConnectionsSettingsDrawer";
import { InfrastructureTab } from "./InfrastructureTab";
import { ProjectsTab } from "./ProjectsTab";
import { useConnectionsLookups } from "./useConnectionsLookups";

const TABS: Array<{ id: ConnectionsTabId; label: string }> = [
  { id: "logins", label: "Logins" },
  { id: "cto", label: "CTO" },
  { id: "splice", label: "Caixa de Emenda" },
  { id: "cables", label: "Cabos" },
  { id: "poles", label: "Postes" },
  { id: "projects", label: "Projetos" },
];

const LOGIN_COLUMNS = [
  { id: "client_name", label: "Cliente" },
  { id: "login", label: "Login" },
  { id: "connection_kind", label: "Tipo" },
  { id: "medium_type", label: "Meio" },
  { id: "sales_plan", label: "Plano" },
  { id: "ip_address", label: "IP" },
  { id: "coords", label: "Coordenadas" },
  { id: "cto_port", label: "CTO / Porta" },
];

export function ConnectionsPageShell() {
  const canMutate = isAdminUser();
  const [tab, setTab] = useState<ConnectionsTabId>("logins");
  const [filters, setFilters] = useState<ConnectionsFilterState>(() => loadMapConnectionFilters());
  const [draftFilters, setDraftFilters] = useState<ConnectionsFilterState>(filters);
  const [prefs, setPrefs] = useState<ConnectionsViewPrefs>(() => loadConnectionsPrefs("logins"));
  const [draftPrefs, setDraftPrefs] = useState<ConnectionsViewPrefs>(prefs);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  useConnectionsLookups(true);

  const activeFilterCount = useMemo(() => countActiveFilters(filters, tab), [filters, tab]);
  const tabProps = { canMutate, filters, prefs };

  function switchTab(next: ConnectionsTabId) {
    setTab(next);
    setPrefs(loadConnectionsPrefs(next));
  }

  function applyFilters() {
    setFilters(draftFilters);
    saveMapConnectionFilters(draftFilters);
    setFiltersOpen(false);
  }

  function saveSettings() {
    setPrefs(draftPrefs);
    saveConnectionsPrefs(tab, draftPrefs);
    setSettingsOpen(false);
  }

  return (
    <>
      <div className="page-heading">
        <h1>
          Conexões
          <InfoHint label="Sobre infraestrutura e logins">
            <p>Logins de clientes e infraestrutura da rede óptica (CTO, emendas, cabos, postes, projetos).</p>
          </InfoHint>
        </h1>
      </div>

      <nav className="conn-tabs" aria-label="Secções de conexões">
        {TABS.map((t) => (
          <button key={t.id} type="button" className={tab === t.id ? "active" : ""} onClick={() => switchTab(t.id)}>
            {t.label}
          </button>
        ))}
      </nav>

      <div className="conn-toolbar" style={{ marginBottom: 12 }}>
        <button
          type="button"
          className="btn"
          onClick={() => {
            setDraftFilters(filters);
            setFiltersOpen(true);
          }}
        >
          Filtros{activeFilterCount > 0 ? ` (${activeFilterCount})` : ""}
        </button>
        <button
          type="button"
          className="btn"
          onClick={() => {
            setDraftPrefs(prefs);
            setSettingsOpen(true);
          }}
        >
          Configurações
        </button>
      </div>

      {tab === "logins" ? <CommercialConnectionsTab {...tabProps} /> : null}
      {tab === "cto" ? <InfrastructureTab variant="cto" tabId="cto" {...tabProps} /> : null}
      {tab === "splice" ? <InfrastructureTab variant="splice" tabId="splice" {...tabProps} /> : null}
      {tab === "cables" ? <InfrastructureTab variant="cable" tabId="cables" {...tabProps} /> : null}
      {tab === "poles" ? <InfrastructureTab variant="pole" tabId="poles" {...tabProps} /> : null}
      {tab === "projects" ? <ProjectsTab {...tabProps} /> : null}

      <ConnectionsFilterDrawer
        open={filtersOpen}
        tab={tab}
        filters={draftFilters}
        onChange={setDraftFilters}
        onClose={() => setFiltersOpen(false)}
        onApply={applyFilters}
      />
      <ConnectionsSettingsDrawer
        open={settingsOpen}
        tab={tab}
        prefs={draftPrefs}
        columnOptions={tab === "logins" ? LOGIN_COLUMNS : []}
        onChange={setDraftPrefs}
        onClose={() => setSettingsOpen(false)}
        onSave={saveSettings}
      />
    </>
  );
}
