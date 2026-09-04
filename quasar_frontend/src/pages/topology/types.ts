import type { Edge, Node } from "@xyflow/react";
import type { TopologyConnectionType } from "../../lib/topologyConnectionTypes";
import type { ManualEquipmentKind } from "../../lib/topologyManualKinds";

/** Equipamento cadastrado (subconjunto de GET /api/v1/devices usado pela lista/canvas). */
export type TopologyDevice = {
  id: string;
  description: string;
  category: string;
  ip: string | null;
};

export type DeviceNodeData = {
  deviceId: string;
  description: string;
  category: string;
  ip: string | null;
  // Injectado por TopologyPage.tsx — mesma razão dos callbacks em ConnectionEdgeData (estado
  // controlado pelo pai, useReactFlow().setNodes não colaria).
  onRemove?: (id: string) => void;
} & Record<string, unknown>;

export type GroupNodeData = {
  shape: "rect" | "circle";
  label: string;
  color?: string;
  onRemove?: (id: string) => void;
} & Record<string, unknown>;

/** Equipamento "avulso" — elemento de rede (switch/roteador/rádio/conversor de mídia/ONU/OLT)
 * colocado directamente no canvas, sem estar ligado a nenhum registro em GET /api/v1/devices.
 * Descrição e IP vivem só aqui (não há cadastro por trás) e são sempre opcionais. */
export type ManualNodeData = {
  kind: ManualEquipmentKind;
  description: string;
  ip: string | null;
  // Mesmo padrão de callback-via-data de DeviceNodeData/ConnectionEdgeData — injectado por
  // TopologyPage.tsx, necessário porque <ReactFlow nodes=…> é controlado pelo estado do pai.
  onPatch?: (id: string, patch: Partial<ManualNodeData>) => void;
  onRemove?: (id: string) => void;
} & Record<string, unknown>;

export type ConnectionEdgeData = {
  connType: TopologyConnectionType;
  iconSize: number;
  customLabel?: string;
  // Injectado por TopologyPage.tsx a partir de doc.settings.connection_colors — cor
  // personalizada por tipo de conexão (Configurações → Cores das conexões), substitui a cor fixa
  // do catálogo (lib/topologyConnectionTypes.ts) quando definida.
  colorOverrides?: Partial<Record<TopologyConnectionType, string>>;
  // Callbacks injectados por TopologyPage.tsx (não fazem parte do documento gravado — ver
  // flowToDoc, que só lê connType/customLabel/iconSize). Necessários porque <ReactFlow edges=…>
  // é controlado pelo estado do pai: um ConnectionEdge não pode chamar useReactFlow().setEdges
  // directamente (isso só mexe no store interno do React Flow, que o próximo render sobrescreve
  // de volta com o array `edges` do pai, inalterado — é por isso que o tipo de conexão não
  // "colava" antes desta mudança).
  onPatch?: (id: string, patch: Partial<ConnectionEdgeData>) => void;
  onRemove?: (id: string) => void;
} & Record<string, unknown>;

export type TopologyNode =
  | Node<DeviceNodeData, "device">
  | Node<GroupNodeData, "pop">
  | Node<ManualNodeData, "manual">;
export type TopologyEdge = Edge<ConnectionEdgeData, "typed">;

/** Formato gravado em GET/PUT /api/v1/topology — só nodes/edges/groups são úteis para nós;
 * os grupos ficam dentro de "nodes" com type="pop" no estado do React Flow, mas são separados
 * aqui no documento gravado para ficar explícito (grupos != equipamentos). */
export type TopologyDocument = {
  nodes: Array<{
    id: string;
    x: number;
    y: number;
    width?: number;
    height?: number;
    parent_id?: string;
    // Presente só em nós avulsos (ver ManualNodeData) — quando ausente, o nó é um equipamento
    // cadastrado e `id` acima é o device_id (comportamento anterior, inalterado).
    manual?: {
      kind: string;
      description?: string;
      ip?: string | null;
    };
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    // Lado do equipamento (top/bottom/left/right — ver SIDES em DeviceNode.tsx) de onde a
    // ligação sai/chega. Sem isto, toda ligação "esquecia" o lado escolhido ao recarregar a
    // página e o React Flow desenhava tudo a sair do topo por omissão.
    source_handle?: string;
    target_handle?: string;
    type: TopologyConnectionType;
    label?: string;
    icon_size?: number;
  }>;
  groups: Array<{
    id: string;
    shape: "rect" | "circle";
    x: number;
    y: number;
    width: number;
    height: number;
    label: string;
    color?: string;
  }>;
  settings?: {
    // Cor personalizada por tipo de conexão (Configurações → Cores) — chave ausente/tipo ausente
    // usa a cor padrão do catálogo (lib/topologyConnectionTypes.ts).
    connection_colors?: Partial<Record<TopologyConnectionType, string>>;
  };
};

export function emptyTopologyDocument(): TopologyDocument {
  return { nodes: [], edges: [], groups: [] };
}
