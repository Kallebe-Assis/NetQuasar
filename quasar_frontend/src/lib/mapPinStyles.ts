/**
 * Catálogo de estilos de ícones do mapa (5 opções por tipo).
 * O primeiro de cada lista é o padrão actual da aplicação.
 */

export type MapPinRole = "equipment" | "connection" | "cto" | "splice_box";

export type MapPinStyleOption = {
  id: string;
  label: string;
};

export const DEFAULT_MAP_PIN_STYLES: Record<MapPinRole, string> = {
  equipment: "pin",
  connection: "user",
  cto: "pin",
  splice_box: "rocket",
};

export const MAP_PIN_STYLE_OPTIONS: Record<MapPinRole, MapPinStyleOption[]> = {
  equipment: [
    { id: "pin", label: "Alfinete" },
    { id: "server", label: "Servidor" },
    { id: "radio", label: "Rádio" },
    { id: "chip", label: "Chip" },
    { id: "building", label: "Edifício" },
    { id: "antenna", label: "Antena" },
    { id: "router", label: "Roteador" },
    { id: "battery", label: "Bateria" },
    { id: "box", label: "Caixa" },
    { id: "tower", label: "Torre" },
  ],
  connection: [
    { id: "user", label: "Usuário" },
    { id: "home", label: "Casa" },
    { id: "wifi", label: "Wi‑Fi" },
    { id: "key", label: "Chave" },
    { id: "signal", label: "Sinal" },
    { id: "smartphone", label: "Celular" },
    { id: "laptop", label: "Notebook" },
    { id: "star", label: "Estrela" },
    { id: "flag", label: "Bandeira" },
    { id: "tag", label: "Etiqueta" },
  ],
  cto: [
    { id: "pin", label: "Alfinete" },
    { id: "cabinet", label: "Armário" },
    { id: "hub", label: "Hub" },
    { id: "drop", label: "Gota" },
    { id: "ring", label: "Anel" },
    { id: "layers", label: "Camadas" },
    { id: "target", label: "Alvo" },
    { id: "square", label: "Quadrado" },
    { id: "shield", label: "Escudo" },
    { id: "bookmark", label: "Marcador" },
  ],
  splice_box: [
    { id: "rocket", label: "Foguete" },
    { id: "joint", label: "Emenda" },
    { id: "bolt", label: "Raio" },
    { id: "diamond", label: "Losango" },
    { id: "hex", label: "Hexágono" },
    { id: "flame", label: "Chama" },
    { id: "link", label: "Elo" },
    { id: "circle-dot", label: "Alvo redondo" },
    { id: "anchor", label: "Âncora" },
    { id: "triangle", label: "Triângulo" },
  ],
};

export function normalizeMapPinStyle(role: MapPinRole, raw: string | null | undefined): string {
  const id = (raw ?? "").trim();
  const allowed = MAP_PIN_STYLE_OPTIONS[role];
  if (allowed.some((o) => o.id === id)) return id;
  return DEFAULT_MAP_PIN_STYLES[role];
}

/** Paths SVG (viewBox 0 0 24 24) — stroke-based, excepto fills marcados. */
const STROKE_PATHS: Record<string, string> = {
  // equipment
  server: `<rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/>`,
  radio: `<path d="M4.9 16.1C1 12.2 1 5.8 4.9 1.9"/><path d="M7.8 4.7a6.14 6.14 0 0 0-.8 7.5"/><circle cx="12" cy="12" r="2"/><path d="M16.2 4.8c2 2 2.3 5.2.7 7.5"/><path d="M19.1 1.9a9.96 9.96 0 0 1 0 14.2"/><line x1="12" x2="12" y1="20" y2="23"/><line x1="8" x2="16" y1="23" y2="23"/>`,
  chip: `<rect width="16" height="16" x="4" y="4" rx="2"/><rect width="6" height="6" x="9" y="9" rx="1"/><path d="M15 2v2"/><path d="M15 20v2"/><path d="M2 15h2"/><path d="M2 9h2"/><path d="M20 15h2"/><path d="M20 9h2"/><path d="M9 2v2"/><path d="M9 20v2"/>`,
  building: `<rect width="16" height="20" x="4" y="2" rx="2" ry="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01"/><path d="M16 6h.01"/><path d="M12 6h.01"/><path d="M12 10h.01"/><path d="M12 14h.01"/><path d="M16 10h.01"/><path d="M16 14h.01"/><path d="M8 10h.01"/><path d="M8 14h.01"/>`,
  antenna: `<path d="M12 2 5 21h14z"/><path d="M9 15h6"/><circle cx="12" cy="7" r="1.5"/>`,
  router: `<rect x="3" y="9" width="18" height="8" rx="2"/><circle cx="8" cy="13" r="1"/><circle cx="12" cy="13" r="1"/><path d="M7 9V6a5 5 0 0 1 10 0v3"/>`,
  battery: `<rect x="3" y="7" width="16" height="10" rx="2"/><path d="M21 10v4"/><path d="M7 11v2"/>`,
  box: `<path d="M3 8 12 4l9 4-9 4-9-4z"/><path d="M3 8v8l9 4 9-4V8"/><path d="M12 12v8"/>`,
  tower: `<path d="M12 2 4 22h4l4-14 4 14h4z"/><path d="M8 16h8"/>`,

  // connection
  home: `<path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>`,
  wifi: `<path d="M12 20h.01"/><path d="M2 8.82a15 15 0 0 1 20 0"/><path d="M5 12.859a10 10 0 0 1 14 0"/><path d="M8.5 16.429a5 5 0 0 1 7 0"/>`,
  key: `<path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4"/><path d="m21 2-9.6 9.6"/><circle cx="7.5" cy="15.5" r="5.5"/>`,
  signal: `<path d="M2 20h.01"/><path d="M7 20v-4"/><path d="M12 20v-8"/><path d="M17 20V8"/><path d="M22 4v16"/>`,
  smartphone: `<rect x="6" y="2" width="12" height="20" rx="2"/><path d="M11 18h2"/>`,
  laptop: `<rect x="3" y="4" width="18" height="12" rx="1"/><path d="M2 20h20"/>`,
  star: `<path d="m12 2 2.9 6.6 7.1.6-5.4 4.7 1.6 7-6.2-3.8-6.2 3.8 1.6-7L2.6 9.2l7.1-.6z"/>`,
  flag: `<path d="M4 22V4"/><path d="M4 4h13l-2.5 4L17 12H4"/>`,
  tag: `<path d="M3 3h8l10 10-8 8L3 11z"/><circle cx="8" cy="8" r="1.5"/>`,

  // cto
  cabinet: `<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18"/><path d="M9 21V9"/>`,
  hub: `<circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14"/>`,
  ring: `<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="4"/>`,
  layers: `<path d="M12 3 2 9l10 6 10-6z"/><path d="M2 15l10 6 10-6"/>`,
  target: `<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.5"/>`,
  square: `<rect x="4" y="4" width="16" height="16" rx="2"/>`,
  shield: `<path d="M12 2 4 5v6c0 5 3.5 9 8 11 4.5-2 8-6 8-11V5z"/>`,
  bookmark: `<path d="M6 2h12v20l-6-4-6 4z"/>`,

  // splice
  rocket: `<path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"/><path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09"/><path d="M9 12a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.4 22.4 0 0 1-4 2z"/><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 .05 5 .05"/>`,
  joint: `<path d="M8 12h8"/><path d="M4 8v8"/><path d="M20 8v8"/><circle cx="8" cy="12" r="2.5"/><circle cx="16" cy="12" r="2.5"/>`,
  bolt: `<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>`,
  diamond: `<path d="M2.7 10.3a2.41 2.41 0 0 0 0 3.41l7.59 7.59a2.41 2.41 0 0 0 3.41 0l7.59-7.59a2.41 2.41 0 0 0 0-3.41l-7.59-7.59a2.41 2.41 0 0 0-3.41 0z"/>`,
  hex: `<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>`,
  flame: `<path d="M12 22c4 0 7-3 7-7 0-4-3-6-4-9-1 2-2 3-3 3-2 0-3-2-3-4-3 2-5 6-5 10 0 4 3 7 8 7z"/>`,
  link: `<path d="M9 17H7a5 5 0 0 1 0-10h2"/><path d="M15 7h2a5 5 0 0 1 0 10h-2"/><path d="M8 12h8"/>`,
  "circle-dot": `<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="2"/>`,
  anchor: `<circle cx="12" cy="5" r="2"/><path d="M12 7v14"/><path d="M5 12H2a10 10 0 0 0 20 0h-3"/>`,
  triangle: `<path d="M12 3 2 21h20z"/>`,
};

/** SVG HTML para pré-visualização no modal (24×24). */
export function mapPinPreviewSvg(role: MapPinRole, styleId: string, color: string): string {
  const style = normalizeMapPinStyle(role, styleId);
  return buildPinSvg(role, style, color, 28);
}

export function buildPinSvg(role: MapPinRole, styleId: string, color: string, size: number): string {
  const style = normalizeMapPinStyle(role, styleId);
  const stroke = "rgba(0,0,0,0.32)";

  if (role === "equipment") {
    if (style === "pin") {
      return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${Math.round(size * 1.27)}" viewBox="0 0 30 38" aria-hidden="true"><path fill="${color}" stroke="${stroke}" stroke-width="1" d="M15 2C8.4 2 3 7.3 3 13.8c0 6.2 9.8 16.5 11.6 18.4L15 36l0.4-3.8C17.2 30.3 27 20 27 13.8 27 7.3 21.6 2 15 2z"/><circle cx="15" cy="14" r="4.2" fill="#fff" opacity="0.95"/></svg>`;
    }
    const inner = STROKE_PATHS[style] ?? STROKE_PATHS.server;
    return strokeIconSvg(size, color, inner, true);
  }

  if (role === "connection") {
    if (style === "user") {
      return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 28 28" aria-hidden="true"><circle cx="14" cy="14" r="12" fill="${color}" stroke="${stroke}" stroke-width="1.2"/><circle cx="14" cy="11" r="4" fill="#fff" opacity="0.95"/><path d="M7 22c0-3.9 3.1-7 7-7s7 3.1 7 7" fill="#fff" opacity="0.95"/></svg>`;
    }
    const inner = STROKE_PATHS[style] ?? STROKE_PATHS.home;
    return strokeIconSvg(size, color, inner, true);
  }

  if (role === "cto") {
    if (style === "pin") {
      return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 22s8-5.5 8-12a8 8 0 1 0-16 0c0 6.5 8 12 8 12z" fill="${color}" stroke="rgba(0,0,0,0.4)" stroke-width="1.4" stroke-linejoin="round"/><circle cx="12" cy="10" r="2.6" fill="#fff" stroke="rgba(0,0,0,0.35)" stroke-width="1.2"/></svg>`;
    }
    if (style === "drop") {
      return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z" fill="${color}" stroke="rgba(0,0,0,0.35)" stroke-width="1.2" stroke-linejoin="round"/></svg>`;
    }
    const inner = STROKE_PATHS[style] ?? STROKE_PATHS.cabinet;
    // CTO filled styles: stroke only, no white disc
    return strokeIconSvg(size, color, inner, false);
  }

  // splice_box — preenchido com a cor definida (sem círculo branco)
  if (style === "rocket") {
    return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" aria-hidden="true"><g fill="${color}" stroke="rgba(0,0,0,0.35)" stroke-width="1.1" stroke-linecap="round" stroke-linejoin="round">${STROKE_PATHS.rocket}</g></svg>`;
  }
  const spliceInner = STROKE_PATHS[style] ?? STROKE_PATHS.rocket;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" aria-hidden="true"><g fill="${color}" stroke="rgba(0,0,0,0.35)" stroke-width="1.1" stroke-linecap="round" stroke-linejoin="round">${spliceInner}</g></svg>`;
}

function strokeIconSvg(size: number, color: string, inner: string, withDisc: boolean): string {
  const disc = withDisc
    ? `<circle cx="12" cy="12" r="11" fill="${color}" stroke="rgba(0,0,0,0.28)" stroke-width="1"/><g stroke="#fff" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${inner}</g>`
    : `<g stroke="${color}" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${inner}</g>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" aria-hidden="true">${disc}</svg>`;
}

/** Ícone importado (ex.: exportado de um diagrama do Lucidchart) em vez de um estilo do
 * catálogo — usado quando um image_url está definido para o tipo/categoria (Configurações do
 * mapa → Ícones → "Importar ícone"). Falha de carga (URL quebrado) só encolhe a imagem, não
 * quebra o marcador — sem onerror inline (CSP), um <img> partido já aparece pequeno/vazio.*/
export function buildImagePinHtml(url: string, size: number): string {
  const esc = url.replace(/"/g, "&quot;").replace(/</g, "&lt;");
  return `<div style="width:${size}px;height:${size}px;display:flex;align-items:center;justify-content:center;background:#fff;border-radius:8px;border:1.5px solid rgba(0,0,0,.25);overflow:hidden"><img src="${esc}" alt="" style="max-width:100%;max-height:100%;object-fit:contain;display:block"/></div>`;
}

export function pinLayout(role: MapPinRole, styleId: string): {
  size: number;
  iconSize: [number, number];
  iconAnchor: [number, number];
  popupAnchor: [number, number];
} {
  const style = normalizeMapPinStyle(role, styleId);
  if (role === "equipment" && style === "pin") {
    return { size: 30, iconSize: [30, 38], iconAnchor: [15, 36], popupAnchor: [0, -34] };
  }
  if (role === "connection" && style === "user") {
    return { size: 28, iconSize: [28, 28], iconAnchor: [14, 14], popupAnchor: [0, -14] };
  }
  if (role === "cto" && (style === "pin" || style === "drop")) {
    return { size: 22, iconSize: [22, 22], iconAnchor: [11, 22], popupAnchor: [0, -20] };
  }
  if (role === "equipment") {
    return { size: 30, iconSize: [30, 30], iconAnchor: [15, 15], popupAnchor: [0, -15] };
  }
  if (role === "connection") {
    return { size: 28, iconSize: [28, 28], iconAnchor: [14, 14], popupAnchor: [0, -14] };
  }
  if (role === "cto") {
    return { size: 26, iconSize: [26, 26], iconAnchor: [13, 13], popupAnchor: [0, -13] };
  }
  // splice
  return { size: 26, iconSize: [26, 26], iconAnchor: [13, 13], popupAnchor: [0, -13] };
}
