import type { Edge, Node } from "@xyflow/react";
import type { TopologyConnectionType } from "../../lib/topologyConnectionTypes";

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

export type ConnectionEdgeData = {
  connType: TopologyConnectionType;
  iconSize: number;
  customLabel?: string;
  // Callbacks injectados por TopologyPage.tsx (não fazem parte do documento gravado — ver
  // flowToDoc, que só lê connType/customLabel/iconSize). Necessários porque <ReactFlow edges=…>
  // é controlado pelo estado do pai: um ConnectionEdge não pode chamar useReactFlow().setEdges
  // directamente (isso só mexe no store interno do React Flow, que o próximo render sobrescreve
  // de volta com o array `edges` do pai, inalterado — é por isso que o tipo de conexão não
  // "colava" antes desta mudança).
  onPatch?: (id: string, patch: Partial<ConnectionEdgeData>) => void;
  onRemove?: (id: string) => void;
} & Record<string, unknown>;

export type TopologyNode = Node<DeviceNodeData, "device"> | Node<GroupNodeData, "pop">;
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
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
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
};

export function emptyTopologyDocument(): TopologyDocument {
  return { nodes: [], edges: [], groups: [] };
}
