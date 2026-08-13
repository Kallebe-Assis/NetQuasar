/** Cores padrão de fibra óptica (cabos nacionais — ordem TIA/EIA adaptada). */
export const STANDARD_FIBER_SEQUENCE = [
  { name: "Verde", hex: "#16a34a", hint: "Fibra 1 (tubo piloto)" },
  { name: "Amarelo", hex: "#eab308", hint: "Fibra 2 (tubo direcional)" },
  { name: "Branco", hex: "#f8fafc", hint: "Fibra 3" },
  { name: "Azul", hex: "#2563eb", hint: "Fibra 4" },
  { name: "Vermelho", hex: "#dc2626", hint: "Fibra 5" },
  { name: "Violeta", hex: "#7c3aed", hint: "Fibra 6 (roxo)" },
  { name: "Marrom", hex: "#92400e", hint: "Fibra 7" },
  { name: "Rosa", hex: "#ec4899", hint: "Fibra 8" },
  { name: "Preto", hex: "#0f172a", hint: "Fibra 9" },
  { name: "Cinza", hex: "#64748b", hint: "Fibra 10" },
  { name: "Laranja", hex: "#ea580c", hint: "Fibra 11" },
  { name: "Aqua", hex: "#06b6d4", hint: "Fibra 12 (outras)" },
] as const;

export type SplitterPortStatus = "livre" | "ocupada" | "reserva" | "defeito";

export type SplitterPort = {
  port: number;
  color: string;
  color_hex: string;
  label: string;
  hint?: string;
  status: SplitterPortStatus;
  note: string;
  destination: string;
};

export const SPLITTER_RATIOS = ["1x2", "1x4", "1x8", "1x16", "1x32", "1x64"] as const;

export const SPLITTER_PORT_STATUSES: Array<{ value: SplitterPortStatus; label: string }> = [
  { value: "livre", label: "Livre" },
  { value: "ocupada", label: "Ocupada" },
  { value: "reserva", label: "Reserva técnica" },
  { value: "defeito", label: "Defeito" },
];

/** Destino da fibra (select fixo). */
export const FIBER_DESTINATIONS = [
  { value: "disponivel", label: "Disponível" },
  { value: "cliente", label: "Cliente" },
  { value: "cto", label: "CTO" },
] as const;

export type FiberDestination = (typeof FIBER_DESTINATIONS)[number]["value"];

export function normalizeFiberDestination(raw?: string | null): FiberDestination {
  const t = (raw ?? "").trim().toLowerCase();
  if (!t || t === "—" || t === "-" || t === "disponivel" || t === "disponível" || t === "livre") return "disponivel";
  if (t === "cliente" || t.includes("cliente")) return "cliente";
  if (t === "cto") return "cto";
  return "disponivel";
}

export function destinationLabel(raw?: string | null): string {
  const v = normalizeFiberDestination(raw);
  return FIBER_DESTINATIONS.find((d) => d.value === v)?.label ?? "Disponível";
}

/** Extrai o número de saídas de "1x8", "1:16", "01x32", etc. */
export function parseSplitterOutputs(raw?: string | null): number | null {
  if (!raw?.trim()) return null;
  const m = raw.trim().match(/^(\d+)\s*[:xX×]\s*(\d+)$/);
  if (!m) return null;
  const out = parseInt(m[2], 10);
  if (!Number.isFinite(out) || out < 1 || out > 128) return null;
  return out;
}

export function fiberSpecForPort(port: number) {
  const idx = ((port - 1) % STANDARD_FIBER_SEQUENCE.length + STANDARD_FIBER_SEQUENCE.length) % STANDARD_FIBER_SEQUENCE.length;
  const spec = STANDARD_FIBER_SEQUENCE[idx];
  return {
    color: spec.name,
    color_hex: spec.hex,
    label: `Fibra ${port}`,
    hint: port <= 12 ? spec.hint : `${spec.name} (ciclo ${Math.floor((port - 1) / 12) + 1})`,
  };
}

export function fiberSpecByName(name?: string | null) {
  const raw = (name ?? "").trim().toLowerCase();
  if (!raw || raw === "desconhecido") {
    return { name: "Desconhecido", hex: "#94a3b8", hint: "Cor da fibra de alimentação não definida" };
  }
  const found = STANDARD_FIBER_SEQUENCE.find((s) => {
    const n = s.name.toLowerCase();
    return raw === n || raw.includes(n) || (n === "aqua" && raw.includes("turquesa")) || (n === "violeta" && raw.includes("roxo"));
  });
  return found
    ? { name: found.name, hex: found.hex, hint: found.hint }
    : { name: "Desconhecido", hex: "#94a3b8", hint: "Cor da fibra de alimentação não definida" };
}

/** Exibe cor da fibra de alimentação da CTO (padrão: Desconhecido). */
export function formatFeedFiberColor(raw?: string | null): string {
  const t = (raw ?? "").trim();
  return t || "Desconhecido";
}

export function buildDefaultSplitterPorts(outputs: number, existing?: SplitterPort[] | null): SplitterPort[] {
  const byPort = new Map((existing ?? []).map((p) => [p.port, p]));
  const out: SplitterPort[] = [];
  for (let i = 1; i <= outputs; i++) {
    const spec = fiberSpecForPort(i);
    const prev = byPort.get(i);
    out.push({
      port: i,
      color: spec.color,
      color_hex: spec.color_hex,
      label: prev?.label?.trim() || spec.label,
      hint: spec.hint,
      status: prev?.status ?? "livre",
      note: prev?.note ?? "",
      destination: normalizeFiberDestination(prev?.destination),
    });
  }
  return out;
}

export const CABLE_FIBER_COUNTS = [2, 6, 12, 24, 36, 48, 72, 144] as const;
export type CableFiberCount = (typeof CABLE_FIBER_COUNTS)[number];

export function isCableFiberCount(n: number): n is CableFiberCount {
  return (CABLE_FIBER_COUNTS as readonly number[]).includes(n);
}

export function statusLabel(status: SplitterPortStatus): string {
  return SPLITTER_PORT_STATUSES.find((s) => s.value === status)?.label ?? status;
}

export function lightFiberBorder(color: string): boolean {
  return color === "Branco" || color === "Amarelo" || color === "Desconhecido";
}

export type SpliceBoxModel = "emenda" | "distribuicao";

export type SplicePair = {
  port: number;
  left_color: string;
  left_color_hex: string;
  right_color: string;
  right_color_hex: string;
  status: SplitterPortStatus;
  note: string;
  destination: string;
};

export function buildDefaultSplicePairs(count: number, existing?: SplicePair[] | null): SplicePair[] {
  const byPort = new Map((existing ?? []).map((p) => [p.port, p]));
  const out: SplicePair[] = [];
  for (let i = 1; i <= count; i++) {
    const spec = fiberSpecForPort(i);
    const prev = byPort.get(i);
    out.push({
      port: i,
      left_color: prev?.left_color || spec.color,
      left_color_hex: prev?.left_color_hex || spec.color_hex,
      right_color: prev?.right_color || spec.color,
      right_color_hex: prev?.right_color_hex || spec.color_hex,
      status: prev?.status ?? "livre",
      note: prev?.note ?? "",
      destination: prev?.destination?.trim() ?? "",
    });
  }
  return out;
}
