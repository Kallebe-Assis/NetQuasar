import L from "leaflet";

export type InfraMapKind = "cto" | "splice_box" | "cable" | "pole" | "project";

export const INFRA_MAP_KIND_LABELS: Record<InfraMapKind, string> = {
  cto: "CTO",
  splice_box: "Caixa de emenda",
  cable: "Cabo",
  pole: "Poste",
  project: "Projeto",
};

export const DEFAULT_INFRA_MAP_COLORS: Record<InfraMapKind, string> = {
  cto: "#7c3aed",
  splice_box: "#d97706",
  cable: "#0891b2",
  pole: "#475569",
  project: "#2563eb",
};

const INFRA_SVG_INNER: Record<InfraMapKind, string> = {
  cto: `<path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0"/><circle cx="12" cy="10" r="3"/>`,
  pole: `<path d="M10 9H4L2 7l2-2h6"/><path d="M14 5h6l2 2-2 2h-6"/><path d="M10 22V4a2 2 0 1 1 4 0v18"/><path d="M8 22h8"/>`,
  splice_box: `<path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"/><path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09"/><path d="M9 12a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.4 22.4 0 0 1-4 2z"/><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 .05 5 .05"/>`,
  cable: `<path d="M17 19a1 1 0 0 1-1-1v-2a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a1 1 0 0 1-1 1z"/><path d="M17 21v-2"/><path d="M19 14V6.5a1 1 0 0 0-7 0v11a1 1 0 0 1-7 0V10"/><path d="M21 21v-2"/><path d="M3 5V3"/><path d="M4 10a2 2 0 0 1-2-2V6a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2a2 2 0 0 1-2 2z"/><path d="M7 5V3"/>`,
  project: `<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>`,
};

function infraSvgHtml(kind: InfraMapKind, color: string, size = 26): string {
  const inner = INFRA_SVG_INNER[kind];
  const stroke = "rgba(0,0,0,0.28)";
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="11" fill="#fff" stroke="${stroke}" stroke-width="1"/><g transform="translate(0,0)">${inner}</g></svg>`;
}

const infraIconCache = new Map<string, L.DivIcon>();

export function infrastructurePinIcon(kind: InfraMapKind, color?: string | null, label?: string | null): L.DivIcon {
  const fill = color?.trim() || DEFAULT_INFRA_MAP_COLORS[kind];
  const labelKey = label?.trim() ? label.trim().slice(0, 48) : "";
  const key = `infra:${kind}:${fill}:${labelKey}`;
  const cached = infraIconCache.get(key);
  if (cached) return cached;
  const pin = infraSvgHtml(kind, fill);
  const labelHtml =
    kind === "cto" && labelKey
      ? `<div style="margin-top:2px;max-width:96px;padding:1px 4px;border-radius:4px;background:rgba(255,255,255,0.92);border:1px solid rgba(0,0,0,0.12);color:#1e293b;font:600 10px/1.2 system-ui,sans-serif;text-align:center;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;box-shadow:0 1px 3px rgba(0,0,0,.12)">${escapeMapLabel(labelKey)}</div>`
      : "";
  const html = `<div style="display:flex;flex-direction:column;align-items:center;line-height:0">${pin}${labelHtml}</div>`;
  const iconH = labelHtml ? 42 : 26;
  const icon = L.divIcon({
    className: "map-infra-pin-wrap",
    html,
    iconSize: [96, iconH],
    iconAnchor: [48, labelHtml ? iconH - 2 : 13],
    popupAnchor: [0, labelHtml ? -iconH + 4 : -13],
  });
  infraIconCache.set(key, icon);
  return icon;
}

function escapeMapLabel(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

export function isInfraMapKind(v: string): v is InfraMapKind {
  return v === "cto" || v === "splice_box" || v === "cable" || v === "pole" || v === "project";
}
