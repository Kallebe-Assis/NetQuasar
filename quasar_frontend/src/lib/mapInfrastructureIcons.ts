import L from "leaflet";
import { buildPinSvg, normalizeMapPinStyle, pinLayout, type MapPinRole } from "./mapPinStyles";

export type InfraMapKind = "cto" | "splice_box" | "cable" | "pole" | "project" | "pop";

export const INFRA_MAP_KIND_LABELS: Record<InfraMapKind, string> = {
  cto: "CTO",
  splice_box: "Caixa de emenda",
  cable: "Cabo",
  pole: "Poste",
  project: "Projeto",
  pop: "POP",
};

/** Cor padrão do alfinete CTO no mapa. */
export const CTO_MAP_PIN_COLOR = "#0D0663";

export const DEFAULT_INFRA_MAP_COLORS: Record<InfraMapKind, string> = {
  cto: CTO_MAP_PIN_COLOR,
  splice_box: "#d97706",
  cable: "#0891b2",
  pole: "#475569",
  project: "#2563eb",
  pop: "#7c3aed",
};

const LEGACY_STROKE: Record<"cable" | "pole" | "project" | "pop", string> = {
  pole: `<path d="M10 9H4L2 7l2-2h6"/><path d="M14 5h6l2 2-2 2h-6"/><path d="M10 22V4a2 2 0 1 1 4 0v18"/><path d="M8 22h8"/>`,
  cable: `<path d="M17 19a1 1 0 0 1-1-1v-2a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a1 1 0 0 1-1 1z"/><path d="M17 21v-2"/><path d="M19 14V6.5a1 1 0 0 0-7 0v11a1 1 0 0 1-7 0V10"/><path d="M21 21v-2"/><path d="M3 5V3"/><path d="M4 10a2 2 0 0 1-2-2V6a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2a2 2 0 0 1-2 2z"/><path d="M7 5V3"/>`,
  project: `<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>`,
  pop: `<path d="M3 21h18"/><path d="M5 21V7l7-4 7 4v14"/><path d="M9 21v-6h6v6"/>`,
};

function legacyInfraSvg(kind: "cable" | "pole" | "project" | "pop", color: string, size: number): string {
  const inner = LEGACY_STROKE[kind];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${inner}</svg>`;
}

export type MapIconStyles = {
  equipment: string;
  connection: string;
  cto: string;
  splice_box: string;
};

export const DEFAULT_MAP_ICON_STYLES: MapIconStyles = {
  equipment: "pin",
  connection: "user",
  cto: "pin",
  splice_box: "rocket",
};

const infraIconCache = new Map<string, L.DivIcon>();

function escapeMapLabel(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

export function infrastructurePinIcon(
  kind: InfraMapKind,
  color?: string | null,
  label?: string | null,
  styleId?: string | null,
): L.DivIcon {
  const fill = color?.trim() || DEFAULT_INFRA_MAP_COLORS[kind];
  const labelKey = label?.trim() ? label.trim().slice(0, 48) : "";
  const style =
    kind === "cto" || kind === "splice_box"
      ? normalizeMapPinStyle(kind as MapPinRole, styleId)
      : "default";
  const key = `infra:v6:${kind}:${fill}:${style}:${labelKey}`;
  const cached = infraIconCache.get(key);
  if (cached) return cached;

  let pin: string;
  let layout = { size: 26, iconSize: [26, 26] as [number, number], iconAnchor: [13, 13] as [number, number], popupAnchor: [0, -13] as [number, number] };

  if (kind === "cto" || kind === "splice_box") {
    layout = pinLayout(kind, style);
    pin = buildPinSvg(kind, style, fill, layout.size);
  } else {
    pin = legacyInfraSvg(kind, fill, 26);
  }

  const labelHtml =
    kind === "cto" && labelKey
      ? `<div style="margin-top:2px;max-width:96px;padding:1px 4px;border-radius:4px;background:rgba(255,255,255,0.92);border:1px solid rgba(0,0,0,0.12);color:#1e293b;font:600 10px/1.2 system-ui,sans-serif;text-align:center;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;box-shadow:0 1px 3px rgba(0,0,0,.12)">${escapeMapLabel(labelKey)}</div>`
      : "";
  const html = `<div style="display:flex;flex-direction:column;align-items:center;line-height:0;filter:drop-shadow(0 1px 2px rgba(0,0,0,.28))">${pin}${labelHtml}</div>`;
  const iconH = labelHtml ? layout.iconSize[1] + 16 : layout.iconSize[1];
  const iconW = Math.max(layout.iconSize[0], labelHtml ? 96 : layout.iconSize[0]);
  const icon = L.divIcon({
    className: "map-infra-pin-wrap",
    html,
    iconSize: [iconW, iconH],
    iconAnchor: [iconW / 2, labelHtml ? iconH - 2 : layout.iconAnchor[1]],
    popupAnchor: [0, labelHtml ? -iconH + 4 : layout.popupAnchor[1]],
  });
  infraIconCache.set(key, icon);
  return icon;
}

export function isInfraMapKind(v: string): v is InfraMapKind {
  return v === "cto" || v === "splice_box" || v === "cable" || v === "pole" || v === "project" || v === "pop";
}
