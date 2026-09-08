import type { Edge, Node } from "@xyflow/react";
import { CircleDot, CircleDotDashed, EthernetPort, type LucideIcon } from "lucide-react";

/** Tipo de caixa/rack no diagrama 2D do POP — cada um vira um rectângulo com uma porta por
 * interface (ver RackNode.tsx). "manual" é qualquer coisa sem cadastro (ex.: DIO, patch panel). */
export type RackNodeKind = "olt" | "mikrotik" | "switch" | "dio" | "manual";

export const RACK_KIND_LABELS: Record<RackNodeKind, string> = {
  olt: "OLT",
  mikrotik: "Mikrotik",
  switch: "Switch",
  dio: "DIO",
  manual: "Caixa",
};

/** Tipo de interface de uma porta — tudo opcional (o utilizador pode deixar em branco). */
export type RackPortType = "sfp" | "sfp_plus" | "ether_100" | "ether_1000";

export const RACK_PORT_TYPE_LABELS: Record<RackPortType, string> = {
  sfp: "SFP",
  sfp_plus: "SFP+",
  ether_100: "Ethernet /100",
  ether_1000: "Ethernet /1000",
};

// O pedido original dava o mesmo SVG (circle-dot) para SFP+ e para as duas Ethernet — na
// prática isso deixava os 3 tipos indistinguíveis no diagrama ("ícone de ether não mudou" era
// visualmente verdade, os 3 desenhavam o mesmo círculo). Ethernet passa a usar EthernetPort
// (ícone dedicado do lucide-react) — continua a mesma ideia (só SFP puro é tracejado), mas agora
// dá pra diferenciar SFP+ de Ethernet à primeira vista.
export const RACK_PORT_TYPE_ICONS: Record<RackPortType, LucideIcon> = {
  sfp: CircleDotDashed,
  sfp_plus: CircleDot,
  ether_100: EthernetPort,
  ether_1000: EthernetPort,
};

export type RackPort = { index: number; label: string; description?: string; portType?: RackPortType | null };

export type RackNodeData = {
  kind: RackNodeKind;
  label: string;
  /** Vínculo opcional a um equipamento cadastrado (GET /api/v1/devices) — só informativo aqui,
   * não é usado para puxar dados ao vivo (o diagrama é um documento livre, como a Topologia). */
  deviceId?: string | null;
  ports: RackPort[];
  // Injectados por PopRackTopologyPage.tsx — mesmo padrão controlado de pages/topology/types.ts
  // (onPatch/onRemove via data, nunca useReactFlow().setNodes directamente).
  onPatch?: (id: string, patch: Partial<RackNodeData>) => void;
  onRemove?: (id: string) => void;
  /** Botão de remover no nó chama isto (não onRemove directamente) — a página mostra um
   * ConfirmModal e só chama onRemove de facto depois de confirmado. */
  onRequestRemove?: (id: string) => void;
  /** Abre o modal de edição de portas (nº de portas, descrição e tipo de cada uma) na página. */
  onEditPorts?: (id: string) => void;
} & Record<string, unknown>;

export type FiberEdgeData = {
  colorName: string;
  colorHex: string;
  label?: string;
  onPatch?: (id: string, patch: Partial<FiberEdgeData>) => void;
  onRemove?: (id: string) => void;
} & Record<string, unknown>;

export type RackNode = Node<RackNodeData, "rack">;
export type FiberEdge = Edge<FiberEdgeData, "fiber">;

/** Formato gravado em GET/PUT /api/v1/pops/{id}/rack-diagram — documento opaco (o backend só
 * garante JSON válido, ver handlers_pop_rack.go), mesmo padrão de TopologyDocument. */
export type PopRackDocument = {
  nodes: Array<{
    id: string;
    x: number;
    y: number;
    kind: RackNodeKind;
    label: string;
    device_id?: string | null;
    ports: RackPort[];
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    source_handle?: string;
    target_handle?: string;
    color_name: string;
    color_hex: string;
    label?: string;
  }>;
};

export function emptyPopRackDocument(): PopRackDocument {
  return { nodes: [], edges: [] };
}

export function buildPorts(count: number, prefix = ""): RackPort[] {
  const n = Math.max(1, Math.min(256, Math.round(count) || 1));
  return Array.from({ length: n }, (_, i) => ({ index: i + 1, label: prefix ? `${prefix}${i + 1}` : String(i + 1) }));
}
