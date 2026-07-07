import { createPortal } from "react-dom";
import type { MapDisplayMode } from "./EquipmentMap";

const MAP_DEVICE_CATEGORIES = ["Concentrador", "Energia", "Mikrotik", "Switch", "OLT", "Rádio", "Servidor", "Máquina Virtual", "Outros"] as const;

type Locality = { id: string; name: string };

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
  showEquipment: boolean;
  onShowEquipment: (v: boolean) => void;
  showConnections: boolean;
  onShowConnections: (v: boolean) => void;
  showInfrastructure: boolean;
  onShowInfrastructure: (v: boolean) => void;
  equipColorDraft: string;
  onEquipColorDraft: (v: string) => void;
  connColorDraft: string;
  onConnColorDraft: (v: string) => void;
  onSaveColors: () => void;
  saveColorsPending: boolean;
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
            <label className="toggle">
              <span className="toggle__track">
                <input type="checkbox" role="switch" className="toggle__input" checked={props.showEquipment} onChange={(e) => props.onShowEquipment(e.target.checked)} />
                <span className="toggle__thumb" aria-hidden />
              </span>
              <span className="toggle__label">Equipamentos</span>
            </label>
            <label className="toggle">
              <span className="toggle__track">
                <input type="checkbox" role="switch" className="toggle__input" checked={props.showConnections} onChange={(e) => props.onShowConnections(e.target.checked)} />
                <span className="toggle__thumb" aria-hidden />
              </span>
              <span className="toggle__label">Logins no mapa</span>
            </label>
            <label className="toggle">
              <span className="toggle__track">
                <input type="checkbox" role="switch" className="toggle__input" checked={props.showInfrastructure} onChange={(e) => props.onShowInfrastructure(e.target.checked)} />
                <span className="toggle__thumb" aria-hidden />
              </span>
              <span className="toggle__label">Infraestrutura</span>
            </label>
          </div>

          <div className="row" style={{ flexWrap: "wrap", gap: 10, alignItems: "center" }}>
            <label className="row" style={{ gap: 8, alignItems: "center" }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Cor equip.</span>
              <input type="color" value={props.equipColorDraft} onChange={(e) => props.onEquipColorDraft(e.target.value)} style={{ width: 36, height: 28, padding: 0, border: "1px solid var(--border)", borderRadius: 4 }} />
            </label>
            <label className="row" style={{ gap: 8, alignItems: "center" }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Cor login</span>
              <input type="color" value={props.connColorDraft} onChange={(e) => props.onConnColorDraft(e.target.value)} style={{ width: 36, height: 28, padding: 0, border: "1px solid var(--border)", borderRadius: 4 }} />
            </label>
            <button type="button" className="btn" disabled={props.saveColorsPending} onClick={props.onSaveColors}>
              Salvar cores
            </button>
          </div>

          <div style={{ borderTop: "1px solid var(--border)", paddingTop: 12 }}>
            <span style={{ fontSize: 12, color: "var(--muted)", display: "block", marginBottom: 6 }}>Ir até localidade</span>
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <select className="select" style={{ flex: 1, minWidth: 180 }} value={props.localityFlyId} onChange={(e) => props.onLocalityFlyId(e.target.value)}>
                <option value="">— Seleccionar localidade —</option>
                {props.localities.map((l) => (
                  <option key={l.id} value={l.id}>
                    {l.name}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn--primary" disabled={!props.localityFlyId || props.localityFlyPending} onClick={props.onFlyToLocality}>
                {props.localityFlyPending ? "…" : "Ir no mapa"}
              </button>
            </div>
            {props.localityFlyNote ? (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>{props.localityFlyNote}</p>
            ) : (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
                Usa a média das coordenadas de equipamentos e infraestrutura cadastrados nesta localidade.
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
