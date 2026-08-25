import { createPortal } from "react-dom";
import type { MapDisplayMode } from "./EquipmentMap";

const MAP_DEVICE_CATEGORIES = ["Concentrador", "Energia", "Mikrotik", "Switch", "OLT", "Rádio", "Servidor", "Máquina Virtual", "Outros"] as const;

type Locality = { id: string; name: string };
type ProjectOpt = { id: string; display_number: number; description: string };

type Props = {
  open: boolean;
  onClose: () => void;
  displayMode: MapDisplayMode;
  onDisplayMode: (m: MapDisplayMode) => void;
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
  showPoles: boolean;
  onShowPoles: (v: boolean) => void;
  showProjects: boolean;
  onShowProjects: (v: boolean) => void;
  showPops: boolean;
  onShowPops: (v: boolean) => void;
  ctoColorByFeed: boolean;
  onCtoColorByFeed: (v: boolean) => void;
  localities: Locality[];
  localityFlyId: string;
  onLocalityFlyId: (v: string) => void;
  onFlyToLocality: () => void;
  localityFlyPending: boolean;
  localityFlyNote: string | null;
};

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
  if (!props.open) return null;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={props.onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="map-filter-title"
        style={{ maxWidth: 560, width: "min(96vw, 560px)" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
          <h3 id="map-filter-title" style={{ margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
            <IconFilter /> Filtros do mapa
          </h3>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={props.onClose}>
            ×
          </button>
        </div>

        <div style={{ display: "grid", gap: 12 }}>
          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Vista</span>
            <select className="select" value={props.displayMode} onChange={(e) => props.onDisplayMode(e.target.value as MapDisplayMode)}>
              <option value="cluster">Agrupado (padrão)</option>
              <option value="scatter">Desagrupado</option>
              <option value="status">Online / Offline</option>
            </select>
          </label>

          <label className="toggle">
            <span className="toggle__track">
              <input
                type="checkbox"
                role="switch"
                className="toggle__input"
                checked={props.ctoColorByFeed}
                onChange={(e) => props.onCtoColorByFeed(e.target.checked)}
              />
              <span className="toggle__thumb" aria-hidden />
            </span>
            <span className="toggle__label">CTOs com cor da fibra de alimentação</span>
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Projeto</span>
            <select className="select" value={props.projectId} onChange={(e) => props.onProjectId(e.target.value)}>
              <option value="">Todos os projetos</option>
              {props.projectsOptions.map((p) => (
                <option key={p.id} value={p.id}>
                  #{p.display_number} — {p.description}
                </option>
              ))}
            </select>
            {props.projectId ? (
              <span style={{ fontSize: 11, color: "var(--muted)" }}>
                O mapa aproxima-se do projeto e mostra apenas a sua infraestrutura (sem equipamentos/logins).
              </span>
            ) : null}
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>POP</span>
            <select className="select" value={props.popId} onChange={(e) => props.onPopId(e.target.value)} disabled={props.popsPending}>
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
            <select className="select" value={props.category} onChange={(e) => props.onCategory(e.target.value)}>
              <option value="">Todas</option>
              {MAP_DEVICE_CATEGORIES.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>

          <div style={{ display: "grid", gap: 8 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Camadas</span>
            <LayerToggle checked={props.showEquipment} onChange={props.onShowEquipment} label="Equipamentos" />
            <LayerToggle checked={props.showCtos} onChange={props.onShowCtos} label="CTOs (viewport)" />
            <LayerToggle checked={props.showCables} onChange={props.onShowCables} label="Cabos (viewport)" />
            <LayerToggle checked={props.showSpliceBoxes} onChange={props.onShowSpliceBoxes} label="Caixas de emenda / foguete" />
            <LayerToggle checked={props.showPoles} onChange={props.onShowPoles} label="Postes" />
            <LayerToggle checked={props.showPops} onChange={props.onShowPops} label="POPs" />
            <LayerToggle checked={props.showProjects} onChange={props.onShowProjects} label="Projetos" />
            <LayerToggle checked={props.showConnections} onChange={props.onShowConnections} label="Logins no mapa" />
          </div>

          <div style={{ borderTop: "1px solid var(--border)", paddingTop: 12 }}>
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
                centra a vista nela.
              </p>
            )}
          </div>
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
