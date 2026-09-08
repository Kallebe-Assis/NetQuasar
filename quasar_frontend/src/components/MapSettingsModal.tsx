import { FileUp } from "lucide-react";
import { createPortal } from "react-dom";
import type { MapIconStyles } from "../lib/mapInfrastructureIcons";
import { MAP_PIN_STYLE_OPTIONS, mapPinPreviewSvg, type MapPinRole } from "../lib/mapPinStyles";
import { deviceCategoryIcon } from "../lib/deviceCategoryIcons";
import { CABLE_FUNCOES } from "../lib/networkInfrastructure";
import {
  MAP_EQUIPMENT_CATEGORIES,
  type EquipmentCategoryConfig,
  type MapAppearanceColors,
  type MapIconImageRole,
} from "../lib/uiAppearance";

type Props = {
  open: boolean;
  onClose: () => void;
  colors: MapAppearanceColors;
  onColorsChange: (next: MapAppearanceColors) => void;
  icons: MapIconStyles;
  onIconsChange: (next: MapIconStyles) => void;
  equipmentCategories: Record<string, EquipmentCategoryConfig>;
  onEquipmentCategoriesChange: (next: Record<string, EquipmentCategoryConfig>) => void;
  iconImageUrls: Partial<Record<MapIconImageRole, string>>;
  onIconImageUrlsChange: (next: Partial<Record<MapIconImageRole, string>>) => void;
  onSave: () => void;
  savePending: boolean;
  canImport?: boolean;
  onImportClick?: () => void;
};

const COLOR_ROWS: Array<{ key: "equipment" | "connection" | "cto" | "splice_box_emenda" | "splice_box_distribuicao"; label: string }> = [
  { key: "equipment", label: "Equipamentos" },
  { key: "connection", label: "Logins" },
  { key: "cto", label: "CTO" },
  { key: "splice_box_emenda", label: "Foguete — emenda" },
  { key: "splice_box_distribuicao", label: "Foguete — distribuição" },
];

const ICON_ROWS: Array<{
  key: "equipment" | "connection" | "cto" | "splice_box_emenda" | "splice_box_distribuicao";
  role: MapPinRole;
  imageKey: MapIconImageRole;
  label: string;
}> = [
  { key: "equipment", role: "equipment", imageKey: "equipment", label: "Equipamentos" },
  { key: "connection", role: "connection", imageKey: "connection", label: "Logins" },
  { key: "cto", role: "cto", imageKey: "cto", label: "CTOs" },
  { key: "splice_box_emenda", role: "splice_box", imageKey: "splice_emenda", label: "Foguete — emenda" },
  { key: "splice_box_distribuicao", role: "splice_box", imageKey: "splice_distribuicao", label: "Foguete — distribuição" },
];

function previewColorFor(key: (typeof ICON_ROWS)[number]["key"], colors: MapAppearanceColors): string {
  switch (key) {
    case "equipment":
      return colors.equipment;
    case "connection":
      return colors.connection;
    case "cto":
      return colors.cto;
    case "splice_box_emenda":
      return colors.splice_box_emenda;
    case "splice_box_distribuicao":
      return colors.splice_box_distribuicao;
  }
}

function IconGear() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

export function MapSettingsModal({
  open,
  onClose,
  colors,
  onColorsChange,
  icons,
  onIconsChange,
  equipmentCategories,
  onEquipmentCategoriesChange,
  iconImageUrls,
  onIconImageUrlsChange,
  onSave,
  savePending,
  canImport,
  onImportClick,
}: Props) {
  if (!open) return null;

  function setCategory(cat: string, patch: Partial<EquipmentCategoryConfig>) {
    onEquipmentCategoriesChange({ ...equipmentCategories, [cat]: { ...equipmentCategories[cat], ...patch } });
  }

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
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            {canImport && onImportClick ? (
              <button
                type="button"
                className="btn btn--sm"
                title="Importar KML/KMZ ou substituir um projeto existente"
                onClick={onImportClick}
              >
                <FileUp size={13} style={{ marginRight: 4, verticalAlign: -2 }} />
                Importar
              </button>
            ) : null}
            <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
              ×
            </button>
          </div>
        </div>

        <div className="map-settings-columns">
          <section className="map-settings-section">
            <h4 className="map-settings-section__title">Cores</h4>
            <p className="map-settings-section__hint">
              Cores padrão dos ícones no mapa. Foguete de emenda e de distribuição têm cor própria.
            </p>
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

            <h4 className="map-settings-section__title" style={{ marginTop: 14 }}>
              Cores por função de cabo
            </h4>
            <p className="map-settings-section__hint">
              Cada função do cabo (Configurações → Elementos) tem uma cor própria na linha desenhada no mapa.
            </p>
            <div className="map-settings-colors">
              {CABLE_FUNCOES.map((f) => (
                <label key={f.value} className="map-settings-color-row">
                  <span>{f.label}</span>
                  <input
                    type="color"
                    value={colors.cable_funcao[f.value]}
                    onChange={(e) =>
                      onColorsChange({ ...colors, cable_funcao: { ...colors.cable_funcao, [f.value]: e.target.value } })
                    }
                  />
                </label>
              ))}
            </div>
          </section>

          <section className="map-settings-section">
            <h4 className="map-settings-section__title">Ícones</h4>
            <p className="map-settings-section__hint">
              Escolha um estilo do catálogo (10 opções) ou importe uma imagem própria — ex.: um ícone exportado do
              Lucidchart — colando o link dela em "URL da imagem".
            </p>
            <div className="map-settings-icons">
              {ICON_ROWS.map((row) => (
                <div key={row.key} className="map-settings-icon-block">
                  <div className="map-settings-icon-block__label">{row.label}</div>
                  <div className="map-settings-icon-grid" role="radiogroup" aria-label={`Ícone de ${row.label}`}>
                    {MAP_PIN_STYLE_OPTIONS[row.role].map((opt) => {
                      const selected = icons[row.key] === opt.id && !iconImageUrls[row.imageKey];
                      const previewColor = previewColorFor(row.key, colors);
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
                  <label className="map-settings-image-url">
                    <span>URL da imagem (opcional — importar em vez de usar o catálogo acima)</span>
                    <input
                      type="url"
                      className="input"
                      placeholder="https://…"
                      value={iconImageUrls[row.imageKey] ?? ""}
                      onChange={(e) => onIconImageUrlsChange({ ...iconImageUrls, [row.imageKey]: e.target.value })}
                    />
                  </label>
                </div>
              ))}
            </div>
          </section>

          <section className="map-settings-section map-settings-section--wide">
            <h4 className="map-settings-section__title">Equipamentos por categoria</h4>
            <p className="map-settings-section__hint">
              Ícone e visibilidade no mapa por categoria de equipamento. Sem personalização, usa o ícone padrão de
              "Equipamentos" acima e fica visível.
            </p>
            <div className="map-settings-categories">
              <div className="map-settings-categories__head">
                <span>Categoria</span>
                <span>Visível no mapa</span>
                <span>Ícone</span>
                <span>URL da imagem (opcional)</span>
              </div>
              {MAP_EQUIPMENT_CATEGORIES.map((cat) => {
                const cfg = equipmentCategories[cat] ?? {};
                const Icon = deviceCategoryIcon(cat);
                return (
                  <div key={cat} className="map-settings-categories__row">
                    <span className="map-settings-categories__name">
                      <Icon size={14} /> {cat}
                    </span>
                    <label className="toggle" style={{ justifySelf: "start" }}>
                      <span className="toggle__track">
                        <input
                          type="checkbox"
                          role="switch"
                          className="toggle__input"
                          checked={!cfg.hidden}
                          onChange={(e) => setCategory(cat, { hidden: !e.target.checked })}
                        />
                        <span className="toggle__thumb" aria-hidden />
                      </span>
                    </label>
                    <select
                      className="select"
                      value={cfg.icon ?? ""}
                      onChange={(e) => setCategory(cat, { icon: e.target.value })}
                    >
                      <option value="">
                        Padrão ({MAP_PIN_STYLE_OPTIONS.equipment.find((o) => o.id === icons.equipment)?.label ?? icons.equipment})
                      </option>
                      {MAP_PIN_STYLE_OPTIONS.equipment.map((opt) => (
                        <option key={opt.id} value={opt.id}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                    <input
                      type="url"
                      className="input"
                      placeholder="https://…"
                      value={cfg.image_url ?? ""}
                      onChange={(e) => setCategory(cat, { image_url: e.target.value })}
                    />
                  </div>
                );
              })}
            </div>
          </section>
        </div>

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
