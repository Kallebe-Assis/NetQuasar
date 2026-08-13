import { useMemo, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  DEFAULT_CONNECTIONS_FILTERS,
  ELEMENT_KIND_LABELS,
  type ConnectionsFilterState,
  type ConnectionsTabId,
  countActiveFilters,
} from "../../lib/connectionsFilters";
import { FIBER_COLORS, PROJECT_STATUSES, CABLE_STATUSES, formatSplitterDisplay, type NetworkCto } from "../../lib/networkInfrastructure";
import { SPLITTER_RATIOS } from "../../lib/fiberSplitter";
import { SideDrawer } from "../../components/SideDrawer";
import { useConnectionsLookups } from "./useConnectionsLookups";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { NETWORK_INFRA_GC_MS, NETWORK_INFRA_STALE_MS } from "../../lib/networkInfraCache";

type Props = {
  open: boolean;
  tab: ConnectionsTabId;
  filters: ConnectionsFilterState;
  onChange: (f: ConnectionsFilterState) => void;
  onClose: () => void;
  onApply: () => void;
};

function FilterField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="side-drawer__field">
      <span className="side-drawer__field-label">{label}</span>
      {children}
    </label>
  );
}

export function ConnectionsFilterDrawer({ open, tab, filters, onChange, onClose, onApply }: Props) {
  const { localities, projects } = useConnectionsLookups(open);

  const ctosQ = useQuery({
    queryKey: queryKeys.networkCtos,
    queryFn: () => apiFetch<{ ctos: NetworkCto[] }>("/api/v1/commercial/network/ctos"),
    enabled: open && tab === "cto",
    staleTime: NETWORK_INFRA_STALE_MS,
    gcTime: NETWORK_INFRA_GC_MS,
  });

  const ctoOptions = useMemo(() => {
    const rows = ctosQ.data?.ctos ?? [];
    const transmitters = new Set<string>();
    const vlans = new Set<string>();
    const extraSplitters = new Set<string>();
    const known = new Set<string>(SPLITTER_RATIOS);
    for (const r of rows) {
      const tx = String(r.transmitter ?? "").trim();
      if (tx) transmitters.add(tx);
      const vlan = String(r.vlan ?? "").trim();
      if (vlan) vlans.add(vlan);
      const sp = formatSplitterDisplay(r.splitter);
      if (sp && sp !== "—" && !known.has(sp)) extraSplitters.add(sp);
    }
    return {
      transmitters: [...transmitters].sort((a, b) => a.localeCompare(b, "pt")),
      vlans: [...vlans].sort((a, b) => a.localeCompare(b, "pt", { numeric: true })),
      splitters: [...SPLITTER_RATIOS, ...extraSplitters],
    };
  }, [ctosQ.data?.ctos]);

  function set<K extends keyof ConnectionsFilterState>(key: K, value: ConnectionsFilterState[K]) {
    onChange({ ...filters, [key]: value });
  }

  return (
    <SideDrawer
      open={open}
      title="Filtros"
      width={tab === "cto" ? 440 : 380}
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
      <p className="side-drawer__hint">{countActiveFilters(filters, tab)} filtro(s) activos nesta aba</p>

      <section className="side-drawer__section">
        <h4 className="side-drawer__section-title">Geral</h4>
        <FilterField label="Projeto">
          <select className="input" value={filters.project_id} onChange={(e) => set("project_id", e.target.value)}>
            <option value="">Todos</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                #{p.display_number} — {p.description}
              </option>
            ))}
          </select>
        </FilterField>
        <FilterField label="Localidade">
          <select className="input" value={filters.locality_id} onChange={(e) => set("locality_id", e.target.value)}>
            <option value="">Todas</option>
            {localities.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        </FilterField>
        <label className="conn-switch">
          <input
            type="checkbox"
            checked={filters.needs_maintenance}
            onChange={(e) => set("needs_maintenance", e.target.checked)}
          />
          Somente com manutenção pendente
        </label>
      </section>

      <section className="side-drawer__section">
        <h4 className="side-drawer__section-title">Visibilidade no mapa</h4>
        <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
          {(Object.keys(ELEMENT_KIND_LABELS) as Array<keyof typeof ELEMENT_KIND_LABELS>).map((k) => (
            <label key={k} className="conn-switch" style={{ display: "flex", marginBottom: 8 }}>
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
      </section>

      {tab === "logins" ? (
        <section className="side-drawer__section">
          <h4 className="side-drawer__section-title">Logins</h4>
          <FilterField label="Tipo de conexão">
            <select
              className="input"
              value={filters.logins.connection_kind}
              onChange={(e) => set("logins", { ...filters.logins, connection_kind: e.target.value })}
            >
              <option value="">Todos</option>
              <option value="pppoe">PPPoE</option>
              <option value="dhcp">DHCP</option>
            </select>
          </FilterField>
          <FilterField label="Meio">
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
          </FilterField>
          <FilterField label="CTO vinculada">
            <input
              className="input"
              value={filters.logins.cto}
              onChange={(e) => set("logins", { ...filters.logins, cto: e.target.value })}
              placeholder="Texto da CTO"
            />
          </FilterField>
        </section>
      ) : null}

      {tab === "cto" ? (
        <section className="side-drawer__section">
          <h4 className="side-drawer__section-title">CTO</h4>
          <FilterField label="Splitter">
            <select
              className="input"
              value={filters.ctos.splitter}
              onChange={(e) => set("ctos", { ...filters.ctos, splitter: e.target.value })}
            >
              <option value="">Todos</option>
              {ctoOptions.splitters.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </FilterField>
          <FilterField label="Transmissor">
            <select
              className="input"
              value={filters.ctos.transmitter}
              onChange={(e) => set("ctos", { ...filters.ctos, transmitter: e.target.value })}
            >
              <option value="">Todos</option>
              {ctoOptions.transmitters.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </FilterField>
          <FilterField label="VLAN">
            <select
              className="input"
              value={filters.ctos.vlan}
              onChange={(e) => set("ctos", { ...filters.ctos, vlan: e.target.value })}
            >
              <option value="">Todas</option>
              {ctoOptions.vlans.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </FilterField>
          <FilterField label="Interface OLT">
            <select
              className="input"
              value={filters.ctos.olt_link}
              onChange={(e) =>
                set("ctos", { ...filters.ctos, olt_link: e.target.value as ConnectionsFilterState["ctos"]["olt_link"] })
              }
            >
              <option value="">Todas</option>
              <option value="linked">Com interface vinculada</option>
              <option value="unlinked">Sem interface</option>
            </select>
          </FilterField>
          <FilterField label="Cor da fibra">
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
          </FilterField>
        </section>
      ) : null}

      {tab === "splice" ? (
        <section className="side-drawer__section">
          <h4 className="side-drawer__section-title">Caixa de emenda</h4>
          <FilterField label="Mínimo de fibras">
            <input
              className="input"
              value={filters.splice_boxes.fiber_count_min}
              onChange={(e) => set("splice_boxes", { ...filters.splice_boxes, fiber_count_min: e.target.value })}
              placeholder="ex. 12"
            />
          </FilterField>
        </section>
      ) : null}

      {tab === "cables" ? (
        <section className="side-drawer__section">
          <h4 className="side-drawer__section-title">Cabos</h4>
          <FilterField label="Tipo">
            <input
              className="input"
              value={filters.cables.cable_type}
              onChange={(e) => set("cables", { ...filters.cables, cable_type: e.target.value })}
            />
          </FilterField>
          <FilterField label="Status">
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
          </FilterField>
        </section>
      ) : null}

      {tab === "poles" ? (
        <section className="side-drawer__section">
          <h4 className="side-drawer__section-title">Postes</h4>
          <FilterField label="Tipo de poste">
            <input
              className="input"
              value={filters.poles.pole_type}
              onChange={(e) => set("poles", { ...filters.poles, pole_type: e.target.value })}
            />
          </FilterField>
        </section>
      ) : null}

      {tab === "projects" ? (
        <section className="side-drawer__section">
          <h4 className="side-drawer__section-title">Projetos</h4>
          <FilterField label="Status">
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
          </FilterField>
        </section>
      ) : null}
    </SideDrawer>
  );
}
