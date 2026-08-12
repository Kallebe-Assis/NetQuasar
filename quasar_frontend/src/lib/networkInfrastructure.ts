export const FIBER_COLORS = [
  "Desconhecido",
  "Verde",
  "Amarelo",
  "Branco",
  "Azul",
  "Vermelho",
  "Violeta",
  "Marrom",
  "Rosa",
  "Preto",
  "Cinza",
  "Laranja",
  "Aqua (Turquesa)",
] as const;

export type FiberColor = (typeof FIBER_COLORS)[number];

export const PROJECT_STATUSES = [
  { value: "planejamento", label: "Planejamento" },
  { value: "em_andamento", label: "Em andamento" },
  { value: "concluido", label: "Concluído" },
  { value: "pausado", label: "Pausado" },
  { value: "cancelado", label: "Cancelado" },
  { value: "inativo", label: "Inativo" },
] as const;

export const CABLE_STATUSES = [
  { value: "ativo", label: "Ativo" },
  { value: "planejado", label: "Planejado" },
  { value: "inativo", label: "Inativo" },
  { value: "manutencao", label: "Manutenção" },
] as const;

export type CommercialLocality = {
  id: string;
  name: string;
  region_code?: string | null;
  latitude?: number | null;
  longitude?: number | null;
};

export type NetworkProject = {
  id: string;
  display_number: number;
  description: string;
  locality_id?: string | null;
  locality_name?: string | null;
  color?: string | null;
  status: string;
  latitude?: number | null;
  longitude?: number | null;
  elements?: {
    ctos?: Array<{ display_number: number; description: string; kind?: string }>;
    splice_boxes?: Array<{ display_number: number; description: string; kind?: string }>;
    cables?: Array<{ display_number: number; description: string; kind?: string }>;
    poles?: Array<{ display_number: number; description: string; kind?: string }>;
  };
};

export type NetworkCto = {
  id: string;
  display_number: number;
  description: string;
  latitude?: number | null;
  longitude?: number | null;
  splitter?: string | null;
  transmitter?: string | null;
  olt_device_id?: string | null;
  pon?: number | null;
  pon_description?: string | null;
  vlan?: number | string | null;
  olt_description?: string | null;
  fiber_color?: string | null;
  notes?: string | null;
  needs_maintenance: boolean;
  project_id?: string | null;
  project_label?: string | null;
  locality_id?: string | null;
  locality_name?: string | null;
  splitter_ports?: Array<{
    port: number;
    color?: string;
    color_hex?: string;
    label?: string;
    status?: string;
    note?: string;
    destination?: string;
  }> | null;
};

export type NetworkSpliceBox = {
  id: string;
  display_number: number;
  description: string;
  latitude?: number | null;
  longitude?: number | null;
  fiber_count?: number | null;
  needs_maintenance: boolean;
  notes?: string | null;
  project_id?: string | null;
  project_label?: string | null;
  /** emenda | distribuicao */
  box_model?: string | null;
  splitter?: string | null;
  fiber_color?: string | null;
  splitter_ports?: Array<{
    port: number;
    color?: string;
    color_hex?: string;
    label?: string;
    status?: string;
    note?: string;
    destination?: string;
  }> | null;
  splice_pairs?: Array<{
    port: number;
    left_color?: string;
    left_color_hex?: string;
    right_color?: string;
    right_color_hex?: string;
    status?: string;
    note?: string;
    destination?: string;
  }> | null;
};

export type NetworkCable = {
  id: string;
  display_number: number;
  description: string;
  cable_type?: string | null;
  fiber_count?: number | null;
  status: string;
  project_id?: string | null;
  project_label?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  path?: Array<{ lat: number; lng: number }> | null;
  fiber_ports?: Array<{
    port: number;
    color?: string;
    color_hex?: string;
    label?: string;
    status?: string;
    note?: string;
    destination?: string;
  }> | null;
};

export type NetworkPole = {
  id: string;
  display_number: number;
  description: string;
  pole_type?: string | null;
  project_id?: string | null;
  project_label?: string | null;
  locality_id?: string | null;
  locality_name?: string | null;
  latitude?: number | null;
  longitude?: number | null;
};

export function fmtCoord(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return "—";
  return v.toFixed(5);
}

export function parseCoordInput(s: string): number | null {
  const t = s.trim().replace(",", ".");
  if (!t) return null;
  const n = Number(t);
  return Number.isFinite(n) ? n : null;
}

export function projectStatusLabel(status: string): string {
  return PROJECT_STATUSES.find((s) => s.value === status)?.label ?? status;
}

export function cableStatusLabel(status: string): string {
  return CABLE_STATUSES.find((s) => s.value === status)?.label ?? status;
}

/** Ex.: 01:08, 1:8 → 1x8 */
export function formatSplitterDisplay(raw?: string | null): string {
  if (!raw?.trim()) return "—";
  const normalized = normalizeSplitterInput(raw);
  return normalized ?? raw.trim();
}

/** Normaliza entrada/saída de splitter para o formato 1x8. */
export function normalizeSplitterInput(raw: string): string | null {
  const t = raw.trim();
  if (!t) return null;
  const m = t.match(/^(\d+)\s*[:xX×]\s*(\d+)$/);
  if (m) return `${parseInt(m[1], 10)}x${parseInt(m[2], 10)}`;
  return t;
}
