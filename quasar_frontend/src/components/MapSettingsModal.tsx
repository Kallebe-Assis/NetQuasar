import { createPortal } from "react-dom";
import type { MapIconStyles } from "../lib/mapInfrastructureIcons";
import { MAP_PIN_STYLE_OPTIONS, mapPinPreviewSvg, type MapPinRole } from "../lib/mapPinStyles";
import type { MapAppearanceColors } from "../lib/uiAppearance";

type Props = {
  open: boolean;
  onClose: () => void;
  colors: MapAppearanceColors;
  onColorsChange: (next: MapAppearanceColors) => void;
  icons: MapIconStyles;
  onIconsChange: (next: MapIconStyles) => void;
  onSave: () => void;
  savePending: boolean;
};

const COLOR_ROWS: Array<{ key: keyof MapAppearanceColors; label: string }> = [
  { key: "equipment", label: "Equipamentos" },
  { key: "connection", label: "Logins" },
  { key: "cto", label: "CTO" },
  { key: "splice_box", label: "Foguete (caixa de emenda)" },
];

const ICON_ROWS: Array<{ key: keyof MapIconStyles; role: MapPinRole; label: string }> = [
  { key: "equipment", role: "equipment", label: "Equipamentos" },
  { key: "connection", role: "connection", label: "Logins" },
  { key: "cto", role: "cto", label: "CTOs" },
  { key: "splice_box", role: "splice_box", label: "Foguete (caixa de emenda)" },
];

function IconGear() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

export function MapSettingsModal({ open, onClose, colors, onColorsChange, icons, onIconsChange, onSave, savePending }: Props) {
  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal map-settings-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="map-settings-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
          <h3 id="map-settings-title" style={{ margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
            <IconGear /> Configurações do mapa
          </h3>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>

        <section className="map-settings-section">
          <h4 className="map-settings-section__title">Cores</h4>
          <p className="map-settings-section__hint">Cores padrão dos ícones no mapa.</p>
          <div className="map-settings-colors">
            {COLOR_ROWS.map((row) => (
              <label key={row.key} className="map-settings-color-row">
                <span>{row.label}</span>
                <input
                  type="color"
                  value={colors[row.key]}
                  onChange={(e) => onColorsChange({ ...colors, [row.key]: e.target.value })}
                />
              </label>
            ))}
          </div>
        </section>

        <section className="map-settings-section">
          <h4 className="map-settings-section__title">Ícones</h4>
          <p className="map-settings-section__hint">Escolha o estilo de cada tipo (5 opções; o primeiro é o padrão actual).</p>
          <div className="map-settings-icons">
            {ICON_ROWS.map((row) => (
              <div key={row.key} className="map-settings-icon-block">
                <div className="map-settings-icon-block__label">{row.label}</div>
                <div className="map-settings-icon-grid" role="radiogroup" aria-label={`Ícone de ${row.label}`}>
                  {MAP_PIN_STYLE_OPTIONS[row.role].map((opt) => {
                    const selected = icons[row.key] === opt.id;
                    const previewColor =
                      row.key === "equipment"
                        ? colors.equipment
                        : row.key === "connection"
                          ? colors.connection
                          : row.key === "cto"
                            ? colors.cto
                            : colors.splice_box;
                    return (
                      <button
                        key={opt.id}
                        type="button"
                        role="radio"
                        aria-checked={selected}
                        className={`map-settings-icon-opt${selected ? " map-settings-icon-opt--selected" : ""}`}
                        title={opt.label}
                        onClick={() => onIconsChange({ ...icons, [row.key]: opt.id })}
                      >
                        <span
                          className="map-settings-icon-opt__preview"
                          dangerouslySetInnerHTML={{ __html: mapPinPreviewSvg(row.role, opt.id, previewColor) }}
                        />
                        <span className="map-settings-icon-opt__name">{opt.label}</span>
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </section>

        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
          <button type="button" className="btn" onClick={onClose} disabled={savePending}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={savePending} onClick={onSave}>
            {savePending ? "A guardar…" : "Guardar"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export function MapSettingsButton({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" className="btn btn--icon btn--icon-menu" title="Configurações do mapa" aria-label="Configurações do mapa" onClick={onClick}>
      <IconGear />
    </button>
  );
}
