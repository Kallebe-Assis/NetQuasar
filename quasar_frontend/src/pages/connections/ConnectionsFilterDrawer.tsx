import {
  DEFAULT_CONNECTIONS_FILTERS,
  ELEMENT_KIND_LABELS,
  type ConnectionsFilterState,
  type ConnectionsTabId,
  countActiveFilters,
} from "../../lib/connectionsFilters";
import { FIBER_COLORS, PROJECT_STATUSES, CABLE_STATUSES } from "../../lib/networkInfrastructure";
import { SideDrawer } from "../../components/SideDrawer";
import { useConnectionsLookups } from "./useConnectionsLookups";

type Props = {
  open: boolean;
  tab: ConnectionsTabId;
  filters: ConnectionsFilterState;
  onChange: (f: ConnectionsFilterState) => void;
  onClose: () => void;
  onApply: () => void;
};

export function ConnectionsFilterDrawer({ open, tab, filters, onChange, onClose, onApply }: Props) {
  const { localities, projects } = useConnectionsLookups(open);

  function set<K extends keyof ConnectionsFilterState>(key: K, value: ConnectionsFilterState[K]) {
    onChange({ ...filters, [key]: value });
  }

  return (
    <SideDrawer
      open={open}
      title="Filtros"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn" onClick={() => onChange({ ...DEFAULT_CONNECTIONS_FILTERS })}>
            Limpar
          </button>
          <button type="button" className="btn btn--primary" onClick={onApply}>
            Aplicar
          </button>
        </>
      }
    >
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        {countActiveFilters(filters, tab)} filtro(s) activos nesta aba
      </p>

      <label className="field" style={{ display: "block", marginBottom: 12 }}>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Pesquisa geral</span>
        <input className="input" value={filters.q} onChange={(e) => set("q", e.target.value)} placeholder="Texto, ID…" />
      </label>

      <label className="field" style={{ display: "block", marginBottom: 12 }}>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Projeto</span>
        <select className="input" value={filters.project_id} onChange={(e) => set("project_id", e.target.value)}>
          <option value="">Todos</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              #{p.display_number} — {p.description}
            </option>
          ))}
        </select>
      </label>

      <label className="field" style={{ display: "block", marginBottom: 12 }}>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Localidade</span>
        <select className="input" value={filters.locality_id} onChange={(e) => set("locality_id", e.target.value)}>
          <option value="">Todas</option>
          {localities.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name}
            </option>
          ))}
        </select>
      </label>

      <label className="conn-switch" style={{ marginBottom: 16 }}>
        <input
          type="checkbox"
          checked={filters.needs_maintenance}
          onChange={(e) => set("needs_maintenance", e.target.checked)}
        />
        Somente com manutenção pendente
      </label>

      <details style={{ marginBottom: 14 }}>
        <summary style={{ fontSize: 12, cursor: "pointer", color: "var(--muted)" }}>Visibilidade no mapa</summary>
        <fieldset style={{ border: "none", padding: "8px 0 0", margin: 0 }}>
          {(Object.keys(ELEMENT_KIND_LABELS) as Array<keyof typeof ELEMENT_KIND_LABELS>).map((k) => (
            <label key={k} className="conn-switch" style={{ display: "flex", marginBottom: 6 }}>
              <input
                type="checkbox"
                checked={filters.visibleKinds.includes(k)}
                onChange={(e) => {
                  const next = e.target.checked
                    ? [...filters.visibleKinds, k]
                    : filters.visibleKinds.filter((x) => x !== k);
                  set("visibleKinds", next.length ? next : [k]);
                }}
              />
              {ELEMENT_KIND_LABELS[k]}
            </label>
          ))}
        </fieldset>
      </details>

      {tab === "logins" ? (
        <>
          <h4 style={{ fontSize: 13, margin: "0 0 8px" }}>Logins</h4>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Tipo de conexão</span>
            <select
              className="input"
              value={filters.logins.connection_kind}
              onChange={(e) => set("logins", { ...filters.logins, connection_kind: e.target.value })}
            >
              <option value="">Todos</option>
              <option value="pppoe">PPPoE</option>
              <option value="dhcp">DHCP</option>
            </select>
          </label>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Meio</span>
            <select
              className="input"
              value={filters.logins.medium_type}
              onChange={(e) => set("logins", { ...filters.logins, medium_type: e.target.value })}
            >
              <option value="">Todos</option>
              <option value="fibra">Fibra</option>
              <option value="radio">Rádio</option>
              <option value="cabo_utp">Cabo UTP</option>
            </select>
          </label>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>CTO vinculada</span>
            <input
              className="input"
              value={filters.logins.cto}
              onChange={(e) => set("logins", { ...filters.logins, cto: e.target.value })}
              placeholder="Texto da CTO"
            />
          </label>
        </>
      ) : null}

      {tab === "cto" ? (
        <>
          <h4 style={{ fontSize: 13, margin: "0 0 8px" }}>CTO</h4>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Cor da fibra</span>
            <select
              className="input"
              value={filters.ctos.fiber_color}
              onChange={(e) => set("ctos", { ...filters.ctos, fiber_color: e.target.value })}
            >
              <option value="">Todas</option>
              {FIBER_COLORS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Splitter</span>
            <input
              className="input"
              value={filters.ctos.splitter}
              onChange={(e) => set("ctos", { ...filters.ctos, splitter: e.target.value })}
            />
          </label>
        </>
      ) : null}

      {tab === "cables" ? (
        <>
          <h4 style={{ fontSize: 13, margin: "0 0 8px" }}>Cabos</h4>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Tipo</span>
            <input
              className="input"
              value={filters.cables.cable_type}
              onChange={(e) => set("cables", { ...filters.cables, cable_type: e.target.value })}
            />
          </label>
          <label className="field" style={{ display: "block", marginBottom: 10 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Status</span>
            <select
              className="input"
              value={filters.cables.status}
              onChange={(e) => set("cables", { ...filters.cables, status: e.target.value })}
            >
              <option value="">Todos</option>
              {CABLE_STATUSES.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
          </label>
        </>
      ) : null}

      {tab === "poles" ? (
        <label className="field" style={{ display: "block", marginBottom: 10 }}>
          <span style={{ fontSize: 11, color: "var(--muted)" }}>Tipo de poste</span>
          <input
            className="input"
            value={filters.poles.pole_type}
            onChange={(e) => set("poles", { ...filters.poles, pole_type: e.target.value })}
          />
        </label>
      ) : null}

      {tab === "projects" ? (
        <label className="field" style={{ display: "block", marginBottom: 10 }}>
          <span style={{ fontSize: 11, color: "var(--muted)" }}>Status</span>
          <select
            className="input"
            value={filters.projects.status}
            onChange={(e) => set("projects", { ...filters.projects, status: e.target.value })}
          >
            <option value="">Todos</option>
            {PROJECT_STATUSES.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
      ) : null}
    </SideDrawer>
  );
}
