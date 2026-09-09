import type { Edge, Node } from "@xyflow/react";
import { CircleDot, CircleDotDashed, EthernetPort, Link2, MapPin, Signpost, Waves, type LucideIcon } from "lucide-react";

/** Tipo de caixa/rack no diagrama 2D do POP — cada um vira um rectângulo com uma porta por
 * interface (ver RackNode.tsx). "manual" é qualquer coisa sem cadastro (ex.: DIO, patch panel).
 * "saida" é o marcador de "sai do POP" — liga-se a uma porta PON (ou qualquer outra) para indicar
 * que aquela fibra, com aquela cor (ver FiberEdge.tsx), segue para fora do POP rumo à
 * distribuição/cliente; não representa um equipamento real, só o ponto onde o diagrama "corta"
 * a fibra que continua lá fora. Por isso é menor e visualmente diferente das outras caixas. */
export type RackNodeKind = "olt" | "mikrotik" | "switch" | "dio" | "manual" | "saida";

export const RACK_KIND_LABELS: Record<RackNodeKind, string> = {
  olt: "OLT",
  mikrotik: "Mikrotik",
  switch: "Switch",
  dio: "DIO",
  manual: "Caixa",
  saida: "Saída",
};

/** Sub-tipo de uma caixa "Saída" — o QUE aquela fibra que sai do POP representa. Escolhido
 * directamente no nó (RackNode.tsx), não no modal de adicionar. "localidade" ganha um segundo
 * selector para vincular a uma localidade cadastrada de verdade (GET /api/v1/commercial/localities). */
export type RackExitKind = "distribuicao" | "transporte" | "link" | "localidade";

export const RACK_EXIT_KIND_LABELS: Record<RackExitKind, string> = {
  distribuicao: "Distribuição / cliente",
  transporte: "Transporte",
  link: "Link",
  localidade: "Localidade",
};

// Waves para "transporte" — mesmo ícone já usado no tipo de conexão "transporte" da Topologia
// geral (ConnectionEdge.tsx), pra manter a mesma linguagem visual entre as duas telas.
export const RACK_EXIT_KIND_ICONS: Record<RackExitKind, LucideIcon> = {
  distribuicao: Signpost,
  transporte: Waves,
  link: Link2,
  localidade: MapPin,
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
  /** Só kind="saida" — ver RackExitKind. Ausente = "distribuicao" (o comportamento original). */
  exitKind?: RackExitKind | null;
  /** Só kind="saida" e exitKind="localidade" — vínculo real a commercial_localities. */
  localityId?: string | null;
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

/** Tipo do cabo representado pela ligação — "fiber" (o comportamento original: cores reais de
 * fibra, ver STANDARD_FIBER_SEQUENCE) ou "ethernet" (cabo de rede — paleta própria, mais curta,
 * ver NETWORK_CABLE_COLORS). Ausente = "fiber" (documentos antigos, todos eram fibra). */
export type CableType = "fiber" | "ethernet";

export const CABLE_TYPE_LABELS: Record<CableType, string> = {
  fiber: "Fibra óptica",
  ethernet: "Cabo de rede",
};

/** Paleta curta para cabo de rede — cores de capa comuns (não tem o significado "posição no tubo"
 * das cores de fibra), mais uma escolha livre via o selector hexadecimal ao lado. */
export const NETWORK_CABLE_COLORS = [
  { name: "Azul", hex: "#2563eb" },
  { name: "Cinza", hex: "#64748b" },
  { name: "Amarelo", hex: "#eab308" },
  { name: "Verde", hex: "#16a34a" },
  { name: "Laranja", hex: "#ea580c" },
  { name: "Preto", hex: "#0f172a" },
] as const;

/** Traço da linha — independente do tipo de cabo, só visual (ajuda a distinguir ligações que se
 * cruzam/sobrepõem no diagrama). Ausente = "solid" (comportamento original). */
export type EdgeDashStyle = "solid" | "dashed" | "dotted";

export const EDGE_DASH_LABELS: Record<EdgeDashStyle, string> = {
  solid: "Contínua",
  dashed: "Tracejada",
  dotted: "Pontilhada",
};

/** strokeDasharray por estilo — undefined (não definir o atributo) para "solid". */
export const EDGE_DASH_PATTERNS: Record<EdgeDashStyle, string | undefined> = {
  solid: undefined,
  dashed: "9 6",
  dotted: "1.5 4.5",
};

/** Ponto intermédio do caminho de uma ligação, em coordenadas do canvas (mesmo referencial de
 * node.position) — permite ao utilizador "desenhar" o percurso do cabo em vez de deixar sempre a
 * curva automática entre origem e destino (ver FiberEdge.tsx: arrastar para mover, botão "+" no
 * meio de cada troço para adicionar, duplo-clique para remover). Vazio/ausente = comportamento
 * original (curva Bezier directa origem→destino). */
export type EdgeWaypoint = { x: number; y: number };

export type FiberEdgeData = {
  colorName: string;
  colorHex: string;
  label?: string;
  cableType?: CableType | null;
  dashStyle?: EdgeDashStyle | null;
  waypoints?: EdgeWaypoint[];
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
    exit_kind?: RackExitKind | null;
    locality_id?: string | null;
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
    cable_type?: CableType | null;
    dash_style?: EdgeDashStyle | null;
    waypoints?: EdgeWaypoint[];
  }>;
};

export function emptyPopRackDocument(): PopRackDocument {
  return { nodes: [], edges: [] };
}

export function buildPorts(count: number, prefix = ""): RackPort[] {
  const n = Math.max(1, Math.min(256, Math.round(count) || 1));
  return Array.from({ length: n }, (_, i) => ({ index: i + 1, label: prefix ? `${prefix}${i + 1}` : String(i + 1) }));
}
