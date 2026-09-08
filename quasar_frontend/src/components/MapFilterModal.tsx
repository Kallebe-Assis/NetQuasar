import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import type { MapDisplayMode } from "./EquipmentMap";
import { MAP_PROJECT_ALL, MAP_PROJECT_NONE } from "../lib/mapProjectFilter";
import { CABLE_FUNCOES, type CableFuncao } from "../lib/networkInfrastructure";
import { CTO_OCCUPANCY_LEGEND } from "../lib/ctoPorts";

export type SpliceModelFilter = "all" | "emenda" | "distribuicao";
export type ConnectionDisplayMode = "cluster" | "individual";
export type CtoColorMode = "default" | "feed" | "occupancy";

const MAP_DEVICE_CATEGORIES = ["Concentrador", "Energia", "Mikrotik", "Switch", "OLT", "Rádio", "Servidor", "Máquina Virtual", "Outros"] as const;

type Locality = { id: string; name: string };
type ProjectOpt = { id: string; display_number: number; description: string };

type Props = {
  open: boolean;
  onClose: () => void;
  displayMode: MapDisplayMode;
  onDisplayMode: (m: MapDisplayMode) => void;
  connectionDisplayMode: ConnectionDisplayMode;
  onConnectionDisplayMode: (m: ConnectionDisplayMode) => void;
  popId: string;
  onPopId: (v: string) => void;
  popsOptions: { id: string; description: string }[];
  popsPending: boolean;
  popsError: boolean;
  category: string;
  onCategory: (v: string) => void;
  projectId: string;
  onProjectId: (v: string) => void;
  projectsOptions: ProjectOpt[];
  showEquipment: boolean;
  onShowEquipment: (v: boolean) => void;
  showCtos: boolean;
  onShowCtos: (v: boolean) => void;
  showCables: boolean;
  onShowCables: (v: boolean) => void;
  showConnections: boolean;
  onShowConnections: (v: boolean) => void;
  showSpliceBoxes: boolean;
  onShowSpliceBoxes: (v: boolean) => void;
  spliceModelFilter: SpliceModelFilter;
  onSpliceModelFilter: (v: SpliceModelFilter) => void;
  /** null = todas as funções (sem filtro). */
  cableFuncaoFilter: Set<CableFuncao> | null;
  onCableFuncaoFilter: (v: Set<CableFuncao> | null) => void;
  showPoles: boolean;
  onShowPoles: (v: boolean) => void;
  showProjects: boolean;
  onShowProjects: (v: boolean) => void;
  showPops: boolean;
  onShowPops: (v: boolean) => void;
  ctoColorMode: CtoColorMode;
  onCtoColorMode: (v: CtoColorMode) => void;
  localities: Locality[];
  localityFlyId: string;
  onLocalityFlyId: (v: string) => void;
  onFlyToLocality: () => void;
  localityFlyPending: boolean;
  localityFlyNote: string | null;
};

/** Estado dos filtros "aplicáveis" (tudo exceto a filtragem por localidade, que já tem o próprio
 * botão "Ir e filtrar" — essa continua imediata). Só é gravado nos estados reais do MapPage
 * (via os onXxx de Props) quando o utilizador clica "Aplicar filtro". */
type Draft = {
  displayMode: MapDisplayMode;
  connectionDisplayMode: ConnectionDisplayMode;
  ctoColorMode: CtoColorMode;
  projectId: string;
  popId: string;
  category: string;
  showEquipment: boolean;
  showCtos: boolean;
  showCables: boolean;
  cableFuncaoFilter: Set<CableFuncao> | null;
  showSpliceBoxes: boolean;
  spliceModelFilter: SpliceModelFilter;
  showPoles: boolean;
  showPops: boolean;
  showProjects: boolean;
  showConnections: boolean;
};

function draftFromProps(p: Props): Draft {
  return {
    displayMode: p.displayMode,
    connectionDisplayMode: p.connectionDisplayMode,
    ctoColorMode: p.ctoColorMode,
    projectId: p.projectId,
    popId: p.popId,
    category: p.category,
    showEquipment: p.showEquipment,
    showCtos: p.showCtos,
    showCables: p.showCables,
    cableFuncaoFilter: p.cableFuncaoFilter,
    showSpliceBoxes: p.showSpliceBoxes,
    spliceModelFilter: p.spliceModelFilter,
    showPoles: p.showPoles,
    showPops: p.showPops,
    showProjects: p.showProjects,
    showConnections: p.showConnections,
  };
}

function IconFilter() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M4 6h16M7 12h10M10 18h4" strokeLinecap="round" />
    </svg>
  );
}

function LayerToggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <label className="toggle">
      <span className="toggle__track">
        <input type="checkbox" role="switch" className="toggle__input" checked={checked} onChange={(e) => onChange(e.target.checked)} />
        <span className="toggle__thumb" aria-hidden />
      </span>
      <span className="toggle__label">{label}</span>
    </label>
  );
}

export function MapFilterModal(props: Props) {
  const [draft, setDraft] = useState<Draft>(() => draftFromProps(props));

  // Reabrir sempre parte do valor actualmente aplicado (não do que ficou de uma edição anterior
  // cancelada/fechada sem aplicar).
  useEffect(() => {
    if (props.open) setDraft(draftFromProps(props));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open]);

  if (!props.open) return null;

  function patch(p: Partial<Draft>) {
    setDraft((d) => ({ ...d, ...p }));
  }

  function apply() {
    props.onDisplayMode(draft.displayMode);
    props.onConnectionDisplayMode(draft.connectionDisplayMode);
    props.onCtoColorMode(draft.ctoColorMode);
    props.onProjectId(draft.projectId);
    props.onPopId(draft.popId);
    props.onCategory(draft.category);
    props.onShowEquipment(draft.showEquipment);
    props.onShowCtos(draft.showCtos);
    props.onShowCables(draft.showCables);
    props.onCableFuncaoFilter(draft.cableFuncaoFilter);
    props.onShowSpliceBoxes(draft.showSpliceBoxes);
    props.onSpliceModelFilter(draft.spliceModelFilter);
    props.onShowPoles(draft.showPoles);
    props.onShowPops(draft.showPops);
    props.onShowProjects(draft.showProjects);
    props.onShowConnections(draft.showConnections);
    props.onClose();
  }

  function cancel() {
    setDraft(draftFromProps(props));
    props.onClose();
  }

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={cancel}>
      <div
        className="modal map-filter-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="map-filter-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
          <h3 id="map-filter-title" style={{ margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
            <IconFilter /> Filtros do mapa
          </h3>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={cancel}>
            ×
          </button>
        </div>

        <div className="map-filter-modal__grid">
          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Vista (equipamentos / infra)</span>
            <select className="select" value={draft.displayMode} onChange={(e) => patch({ displayMode: e.target.value as MapDisplayMode })}>
              <option value="cluster">Agrupado (padrão)</option>
              <option value="scatter">Desagrupado</option>
              <option value="status">Online / Offline</option>
            </select>
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Vista dos logins</span>
            <select
              className="select"
              value={draft.connectionDisplayMode}
              onChange={(e) => patch({ connectionDisplayMode: e.target.value as ConnectionDisplayMode })}
            >
              <option value="cluster">Agrupado (padrão)</option>
              <option value="individual">Individual</option>
            </select>
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Projeto</span>
            <select className="select" value={draft.projectId} onChange={(e) => patch({ projectId: e.target.value })}>
              <option value={MAP_PROJECT_NONE}>Nenhum</option>
              <option value={MAP_PROJECT_ALL}>Todos os projetos</option>
              {props.projectsOptions.map((p) => (
                <option key={p.id} value={p.id}>
                  #{p.display_number} — {p.description}
                </option>
              ))}
            </select>
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>POP</span>
            <select className="select" value={draft.popId} onChange={(e) => patch({ popId: e.target.value })} disabled={props.popsPending}>
              <option value="">Todos os POPs</option>
              {props.popsOptions.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.description}
                </option>
              ))}
            </select>
            {props.popsError ? <span className="msg msg--err" style={{ fontSize: 11 }}>Não foi possível carregar POPs.</span> : null}
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Categoria</span>
            <select className="select" value={draft.category} onChange={(e) => patch({ category: e.target.value })}>
              <option value="">Todas</option>
              {MAP_DEVICE_CATEGORIES.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Cor da CTO</span>
            <select className="select" value={draft.ctoColorMode} onChange={(e) => patch({ ctoColorMode: e.target.value as CtoColorMode })}>
              <option value="default">Padrão</option>
              <option value="feed">Cor da fibra de alimentação</option>
              <option value="occupancy">Ocupação de portas (escala de cores)</option>
            </select>
            {draft.ctoColorMode === "occupancy" ? (
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 2 }}>
                {CTO_OCCUPANCY_LEGEND.map((l) => (
                  <span key={l.label} style={{ display: "inline-flex", alignItems: "center", gap: 4, fontSize: 10.5, color: "var(--muted)" }}>
                    <span style={{ width: 9, height: 9, borderRadius: "50%", background: l.color, display: "inline-block" }} />
                    {l.label}
                  </span>
                ))}
              </div>
            ) : null}
          </label>

          {draft.projectId === MAP_PROJECT_NONE ? (
            <span className="map-filter-modal__full" style={{ fontSize: 11, color: "var(--muted)" }}>
              Mostra a infraestrutura das camadas activas abaixo, sem restringir a nenhum projeto, na área visível do mapa.
            </span>
          ) : draft.projectId === MAP_PROJECT_ALL ? (
            <span className="map-filter-modal__full" style={{ fontSize: 11, color: "var(--muted)" }}>
              Carrega CTOs e restante infraestrutura de todos os projetos activos na área visível (pode ser mais lento).
            </span>
          ) : draft.projectId ? (
            <span className="map-filter-modal__full" style={{ fontSize: 11, color: "var(--muted)" }}>
              O mapa aproxima-se do projeto e mostra apenas a sua infraestrutura (sem equipamentos/logins).
            </span>
          ) : null}

          <div className="map-filter-modal__full" style={{ display: "grid", gap: 8 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Camadas</span>
            <LayerToggle checked={draft.showEquipment} onChange={(v) => patch({ showEquipment: v })} label="Equipamentos" />
            <LayerToggle checked={draft.showCtos} onChange={(v) => patch({ showCtos: v })} label="CTOs (viewport)" />
            <LayerToggle checked={draft.showCables} onChange={(v) => patch({ showCables: v })} label="Cabos (viewport)" />
            {draft.showCables ? (
              <div style={{ marginLeft: 24, display: "flex", flexDirection: "column", gap: 6 }}>
                <span style={{ fontSize: 11, color: "var(--muted)" }}>Função do cabo (marque uma ou mais)</span>
                <div className="map-filter-chip-group">
                  {CABLE_FUNCOES.map((f) => {
                    const checked = draft.cableFuncaoFilter == null || draft.cableFuncaoFilter.has(f.value);
                    return (
                      <label key={f.value} className="map-filter-chip">
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={(e) => {
                            const all = new Set(CABLE_FUNCOES.map((x) => x.value));
                            const base = draft.cableFuncaoFilter ?? all;
                            const next = new Set(base);
                            if (e.target.checked) next.add(f.value);
                            else next.delete(f.value);
                            // Tudo marcado (ou nada desmarcado) volta a "sem filtro" (null) — evita
                            // gravar/comparar um Set igual a "todas as funções" para sempre.
                            patch({ cableFuncaoFilter: next.size === 0 || next.size === all.size ? null : next });
                          }}
                        />
                        {f.label}
                      </label>
                    );
                  })}
                </div>
              </div>
            ) : null}
            <LayerToggle checked={draft.showSpliceBoxes} onChange={(v) => patch({ showSpliceBoxes: v })} label="Caixas de emenda / foguete" />
            {draft.showSpliceBoxes ? (
              <label style={{ marginLeft: 24, display: "flex", flexDirection: "column", gap: 4, maxWidth: 260 }}>
                <span style={{ fontSize: 11, color: "var(--muted)" }}>Tipo de foguete</span>
                <select
                  className="select"
                  value={draft.spliceModelFilter}
                  onChange={(e) => patch({ spliceModelFilter: e.target.value as SpliceModelFilter })}
                >
                  <option value="all">Todos</option>
                  <option value="emenda">Só emenda</option>
                  <option value="distribuicao">Só distribuição</option>
                </select>
              </label>
            ) : null}
            <LayerToggle checked={draft.showPoles} onChange={(v) => patch({ showPoles: v })} label="Postes" />
            <LayerToggle checked={draft.showPops} onChange={(v) => patch({ showPops: v })} label="POPs" />
            <LayerToggle checked={draft.showProjects} onChange={(v) => patch({ showProjects: v })} label="Projetos" />
            <LayerToggle checked={draft.showConnections} onChange={(v) => patch({ showConnections: v })} label="Logins no mapa" />
          </div>

          <div className="map-filter-modal__full" style={{ borderTop: "1px solid var(--border)", paddingTop: 12 }}>
            <span style={{ fontSize: 12, color: "var(--muted)", display: "block", marginBottom: 6 }}>
              Filtrar por localidade
            </span>
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <select className="select" style={{ flex: 1, minWidth: 180 }} value={props.localityFlyId} onChange={(e) => props.onLocalityFlyId(e.target.value)}>
                <option value="">— Todas as localidades —</option>
                {props.localities.map((l) => (
                  <option key={l.id} value={l.id}>
                    {l.name}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn--primary" disabled={!props.localityFlyId || props.localityFlyPending} onClick={props.onFlyToLocality}>
                {props.localityFlyPending ? "…" : "Ir e filtrar"}
              </button>
            </div>
            {props.localityFlyNote ? (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>{props.localityFlyNote}</p>
            ) : (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
                Com localidade seleccionada, o mapa só pede CTOs/cabos/postes dessa localidade (ou dos seus projectos) e
                centra a vista nela. Esta acção é imediata — não depende do botão "Aplicar filtro" abaixo.
              </p>
            )}
          </div>
        </div>

        <div className="map-filter-modal__foot">
          <button type="button" className="btn" onClick={cancel}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" onClick={apply}>
            Aplicar filtro
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export function MapFilterButton({ onClick, activeCount }: { onClick: () => void; activeCount?: number }) {
  return (
    <button type="button" className="btn btn--icon btn--icon-menu" title="Filtros do mapa" aria-label="Filtros do mapa" onClick={onClick} style={{ position: "relative" }}>
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <path d="M4 6h16M7 12h10M10 18h4" strokeLinecap="round" />
      </svg>
      {activeCount != null && activeCount > 0 ? (
        <span style={{ position: "absolute", top: 2, right: 2, width: 8, height: 8, borderRadius: "50%", background: "var(--accent, #3b82f6)" }} aria-hidden />
      ) : null}
    </button>
  );
}
