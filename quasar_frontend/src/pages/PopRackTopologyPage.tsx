import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  ConnectionMode,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type OnConnect,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import "./topology/topology.css";
import "./poprack/poprack.css";
import { ArrowLeft, Plus, Save } from "lucide-react";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { can, isAdminUser } from "../lib/auth";
import { APP_ROUTES } from "../app/routes";
import { STANDARD_FIBER_SEQUENCE } from "../lib/fiberSplitter";
import { ConfirmModal } from "../components/ConfirmModal";
import { FiberEdge } from "./poprack/FiberEdge";
import { PortsEditModal } from "./poprack/PortsEditModal";
import { RackNode } from "./poprack/RackNode";
import {
  buildPorts,
  emptyPopRackDocument,
  RACK_KIND_LABELS,
  type FiberEdgeData,
  type PopRackDocument,
  type RackNodeData,
  type RackNodeKind,
  type RackPort,
} from "./poprack/types";

const nodeTypes = { rack: RackNode };
const edgeTypes = { fiber: FiberEdge };
const DEFAULT_FIBER = STANDARD_FIBER_SEQUENCE[0];

/** Categoria de equipamento (cadastro em Equipamentos) que corresponde a cada tipo de caixa do
 * diagrama — usada para filtrar a lista ao "puxar" um equipamento cadastrado. DIO/manual não têm
 * cadastro correspondente (nunca aparecem como device na tela de Equipamentos). */
const RACK_KIND_DEVICE_CATEGORY: Partial<Record<RackNodeKind, string>> = {
  olt: "OLT",
  mikrotik: "Mikrotik",
  switch: "Switch",
};

type DeviceOption = { id: string; description: string; category: string; max_pons: number | null };

function docToFlow(doc: PopRackDocument): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = doc.nodes.map((n) => ({
    id: n.id,
    type: "rack",
    position: { x: n.x, y: n.y },
    data: { kind: n.kind, label: n.label, deviceId: n.device_id ?? null, ports: n.ports } satisfies RackNodeData,
  }));
  const edges: Edge[] = doc.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    sourceHandle: e.source_handle,
    targetHandle: e.target_handle,
    type: "fiber",
    data: { colorName: e.color_name, colorHex: e.color_hex, label: e.label } satisfies FiberEdgeData,
  }));
  return { nodes, edges };
}

function flowToDoc(nodes: Node[], edges: Edge[]): PopRackDocument {
  const doc = emptyPopRackDocument();
  for (const n of nodes) {
    const data = n.data as RackNodeData;
    doc.nodes.push({
      id: n.id,
      x: n.position.x,
      y: n.position.y,
      kind: data.kind,
      label: data.label,
      device_id: data.deviceId ?? undefined,
      ports: data.ports,
    });
  }
  for (const e of edges) {
    const data = (e.data ?? {}) as Partial<FiberEdgeData>;
    doc.edges.push({
      id: e.id,
      source: e.source,
      target: e.target,
      source_handle: e.sourceHandle ?? undefined,
      target_handle: e.targetHandle ?? undefined,
      color_name: data.colorName ?? DEFAULT_FIBER.name,
      color_hex: data.colorHex ?? DEFAULT_FIBER.hex,
      label: data.label,
    });
  }
  return doc;
}

function PopRackCanvas({ popId }: { popId: string }) {
  const { push: pushToast } = useAppToast();
  const qc = useQueryClient();
  const canMutate = isAdminUser() || can("map.manage");
  const addCounterRef = useRef(0);
  const hydratedRef = useRef(false);

  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [dirty, setDirty] = useState(false);
  const [addModal, setAddModal] = useState<RackNodeKind | null>(null);
  const [addLabel, setAddLabel] = useState("");
  const [addPorts, setAddPorts] = useState("8");
  const [addDeviceId, setAddDeviceId] = useState("");
  const [removeRequestId, setRemoveRequestId] = useState<string | null>(null);
  const [editPortsId, setEditPortsId] = useState<string | null>(null);

  const popQ = useQuery({
    queryKey: ["pop-detail", popId],
    queryFn: () => apiFetch<{ description: string }>(`/api/v1/pops/${popId}`),
  });

  const canvasQ = useQuery({
    queryKey: ["pop-rack-diagram", popId],
    queryFn: () => apiFetch<PopRackDocument>(`/api/v1/pops/${popId}/rack-diagram`),
  });

  // Lista de equipamentos cadastrados para o botão "Selecionar equipamento" no modal de
  // adicionar — filtrada por categoria conforme o tipo de caixa (RACK_KIND_DEVICE_CATEGORY).
  const devicesQ = useQuery({
    queryKey: ["poprack-devices"],
    queryFn: () => apiFetch<{ devices: DeviceOption[] }>("/api/v1/devices"),
    staleTime: 5 * 60 * 1000,
  });
  const deviceOptionsForKind = useMemo(() => {
    if (!addModal) return [];
    const cat = RACK_KIND_DEVICE_CATEGORY[addModal];
    if (!cat) return [];
    return (devicesQ.data?.devices ?? []).filter((d) => d.category?.toLowerCase() === cat.toLowerCase());
  }, [addModal, devicesQ.data]);

  useEffect(() => {
    if (hydratedRef.current) return;
    if (!canvasQ.data) return;
    const { nodes: n, edges: e } = docToFlow(canvasQ.data);
    setNodes(n);
    setEdges(e);
    setDirty(false);
    hydratedRef.current = true;
  }, [canvasQ.data]);

  const markDirty = useCallback(() => {
    if (hydratedRef.current) setDirty(true);
  }, []);

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => {
      setNodes((nds) => applyNodeChanges(changes, nds));
      markDirty();
    },
    [markDirty],
  );
  const onEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      setEdges((eds) => applyEdgeChanges(changes, eds));
      markDirty();
    },
    [markDirty],
  );

  const patchNodeData = useCallback(
    (id: string, patch: Partial<RackNodeData>) => {
      setNodes((nds) => nds.map((n) => (n.id === id ? { ...n, data: { ...(n.data as RackNodeData), ...patch } } : n)));
      markDirty();
    },
    [markDirty],
  );
  const removeNode = useCallback(
    (id: string) => {
      setNodes((nds) => nds.filter((n) => n.id !== id));
      setEdges((eds) => eds.filter((e) => e.source !== id && e.target !== id));
      markDirty();
    },
    [markDirty],
  );
  // Botão de remover no nó só pede confirmação (ConfirmModal abaixo) — a remoção de facto só
  // acontece em confirmRemoveNode, chamada pelo onConfirm do modal.
  const requestRemoveNode = useCallback((id: string) => setRemoveRequestId(id), []);
  function confirmRemoveNode() {
    if (removeRequestId) removeNode(removeRequestId);
    setRemoveRequestId(null);
  }
  const openEditPorts = useCallback((id: string) => setEditPortsId(id), []);
  function saveEditedPorts(ports: RackPort[]) {
    if (editPortsId) patchNodeData(editPortsId, { ports });
  }
  const editingPortsNode = useMemo(() => nodes.find((n) => n.id === editPortsId) ?? null, [nodes, editPortsId]);
  const patchEdgeData = useCallback(
    (id: string, patch: Partial<FiberEdgeData>) => {
      setEdges((eds) => eds.map((e) => (e.id === id ? { ...e, data: { ...(e.data as FiberEdgeData), ...patch } } : e)));
      markDirty();
    },
    [markDirty],
  );
  const removeEdgeById = useCallback(
    (id: string) => {
      setEdges((eds) => eds.filter((e) => e.id !== id));
      markDirty();
    },
    [markDirty],
  );

  const nodesForFlow = useMemo(
    () =>
      nodes.map((n) => ({
        ...n,
        data: {
          ...(n.data as RackNodeData),
          onPatch: patchNodeData,
          onRemove: removeNode,
          onRequestRemove: requestRemoveNode,
          onEditPorts: openEditPorts,
        },
      })),
    [nodes, patchNodeData, removeNode, requestRemoveNode, openEditPorts],
  );
  const edgesForFlow = useMemo(
    () => edges.map((e) => ({ ...e, data: { ...(e.data as FiberEdgeData), onPatch: patchEdgeData, onRemove: removeEdgeById } })),
    [edges, patchEdgeData, removeEdgeById],
  );

  const onConnect: OnConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) => {
        const deselected: Edge[] = eds.map((e) => ({ ...e, selected: false }));
        const newEdge: Edge = {
          ...connection,
          id: `fiber-${crypto.randomUUID()}`,
          type: "fiber",
          selected: true,
          data: { colorName: DEFAULT_FIBER.name, colorHex: DEFAULT_FIBER.hex } satisfies FiberEdgeData,
        };
        return addEdge(newEdge, deselected);
      });
      markDirty();
    },
    [markDirty],
  );

  function openAddModal(kind: RackNodeKind) {
    addCounterRef.current += 1;
    setAddLabel(`${RACK_KIND_LABELS[kind]} ${addCounterRef.current}`);
    setAddPorts(kind === "olt" ? "16" : kind === "dio" ? "12" : "8");
    setAddDeviceId("");
    setAddModal(kind);
  }

  // Selecionar um equipamento cadastrado preenche nome (sempre) e, para OLT, o total de PONs a
  // partir de max_pons — só um ponto de partida, o utilizador ainda pode alterar os dois campos
  // antes de confirmar (pedido explícito: "só auto-preenchido mas o usuário vai poder alterar").
  function selectDeviceForAdd(deviceId: string) {
    setAddDeviceId(deviceId);
    if (!deviceId) return;
    const dev = deviceOptionsForKind.find((d) => d.id === deviceId);
    if (!dev) return;
    setAddLabel(dev.description);
    if (addModal === "olt" && dev.max_pons != null && dev.max_pons > 0) {
      setAddPorts(String(dev.max_pons));
    }
  }

  function confirmAddNode() {
    if (!addModal) return;
    const label = addLabel.trim() || RACK_KIND_LABELS[addModal];
    const portCount = Math.max(1, Math.min(256, Number(addPorts) || 1));
    const id = `rack-${crypto.randomUUID()}`;
    const node: Node = {
      id,
      type: "rack",
      position: { x: 80 + ((nodes.length * 40) % 400), y: 80 + ((nodes.length * 60) % 300) },
      data: { kind: addModal, label, deviceId: addDeviceId || null, ports: buildPorts(portCount) } satisfies RackNodeData,
    };
    setNodes((nds) => [...nds, node]);
    markDirty();
    setAddModal(null);
  }

  async function save() {
    const doc = flowToDoc(nodes, edges);
    try {
      await apiFetch(`/api/v1/pops/${popId}/rack-diagram`, { method: "PUT", json: doc });
      setDirty(false);
      toastOk(pushToast, "Diagrama do POP salvo.");
      void qc.invalidateQueries({ queryKey: ["pop-rack-diagram", popId] });
    } catch (e) {
      toastErr(pushToast, e, "Falha ao salvar o diagrama.");
    }
  }

  if (canvasQ.isPending) return <p style={{ padding: 16 }}>Carregando diagrama…</p>;
  if (canvasQ.isError) return <div className="msg msg--err" style={{ margin: 16 }}>Falha ao carregar o diagrama do POP.</div>;

  return (
    <div className="topo-page">
      <div className="topo-toolbar">
        <Link to={APP_ROUTES.pops} className="btn btn--icon" title="Voltar a Localidades/POPs" aria-label="Voltar">
          <ArrowLeft size={16} />
        </Link>
        <h1 style={{ fontSize: 16, margin: 0 }}>
          Topologia 2D — {popQ.data?.description ?? "POP"}
        </h1>
        {canMutate && (
          <>
            <button type="button" className="btn btn--sm" onClick={() => openAddModal("olt")}>
              <Plus size={13} style={{ verticalAlign: -2 }} /> OLT
            </button>
            <button type="button" className="btn btn--sm" onClick={() => openAddModal("mikrotik")}>
              <Plus size={13} style={{ verticalAlign: -2 }} /> Mikrotik
            </button>
            <button type="button" className="btn btn--sm" onClick={() => openAddModal("switch")}>
              <Plus size={13} style={{ verticalAlign: -2 }} /> Switch
            </button>
            <button type="button" className="btn btn--sm" onClick={() => openAddModal("dio")}>
              <Plus size={13} style={{ verticalAlign: -2 }} /> DIO
            </button>
            <button type="button" className="btn btn--sm" onClick={() => openAddModal("manual")}>
              <Plus size={13} style={{ verticalAlign: -2 }} /> Caixa
            </button>
          </>
        )}
        <div className="topo-toolbar__legend">
          {dirty ? <span style={{ color: "var(--warn, #d29922)" }}>Alterações não salvas</span> : null}
          {canMutate && (
            <button type="button" className="btn btn--primary btn--sm" onClick={save} disabled={!dirty}>
              <Save size={13} style={{ verticalAlign: -2, marginRight: 4 }} /> Salvar
            </button>
          )}
        </div>
      </div>

      <div className="topo-body">
        <div className="topo-canvas-wrap" style={{ flex: 1 }}>
          <ReactFlow
            nodes={nodesForFlow}
            edges={edgesForFlow}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            onNodesChange={canMutate ? onNodesChange : undefined}
            onEdgesChange={canMutate ? onEdgesChange : undefined}
            onConnect={canMutate ? onConnect : undefined}
            nodesDraggable={canMutate}
            nodesConnectable={canMutate}
            elementsSelectable
            deleteKeyCode={canMutate ? ["Backspace", "Delete"] : null}
            connectionMode={ConnectionMode.Loose}
            elevateNodesOnSelect={false}
            fitView
            minZoom={0.2}
            maxZoom={2}
          >
            <Background gap={20} />
            <Controls />
            <MiniMap pannable zoomable />
          </ReactFlow>
        </div>
      </div>

      {addModal ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setAddModal(null)}>
          <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 360 }} onMouseDown={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Adicionar {RACK_KIND_LABELS[addModal]}</h3>
            {deviceOptionsForKind.length > 0 ? (
              <div className="field">
                <label>Equipamento cadastrado (opcional)</label>
                <select className="select" value={addDeviceId} onChange={(e) => selectDeviceForAdd(e.target.value)}>
                  <option value="">— Digitar manualmente —</option>
                  {deviceOptionsForKind.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.description}
                      {addModal === "olt" && d.max_pons ? ` (${d.max_pons} PONs)` : ""}
                    </option>
                  ))}
                </select>
                <span style={{ fontSize: 11, color: "var(--muted)" }}>
                  Preenche nome{addModal === "olt" ? " e total de PONs" : ""} — pode alterar antes de adicionar.
                </span>
              </div>
            ) : null}
            <div className="field" style={{ marginTop: 10 }}>
              <label>Nome</label>
              <input className="input" value={addLabel} onChange={(e) => setAddLabel(e.target.value)} />
            </div>
            <div className="field" style={{ marginTop: 10 }}>
              <label>Número de portas / interfaces</label>
              <input
                className="input mono"
                type="number"
                min={1}
                max={256}
                value={addPorts}
                onChange={(e) => setAddPorts(e.target.value)}
              />
            </div>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
              <button type="button" className="btn" onClick={() => setAddModal(null)}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" onClick={confirmAddNode}>
                Adicionar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <ConfirmModal
        open={!!removeRequestId}
        title="Remover equipamento"
        message="Remover esta caixa do diagrama? As ligações de fibra dela também serão removidas."
        confirmLabel="Remover"
        danger
        onCancel={() => setRemoveRequestId(null)}
        onConfirm={confirmRemoveNode}
      />
      <PortsEditModal
        open={!!editPortsId}
        nodeLabel={editingPortsNode ? (editingPortsNode.data as RackNodeData).label : ""}
        ports={editingPortsNode ? (editingPortsNode.data as RackNodeData).ports : []}
        onSave={saveEditedPorts}
        onClose={() => setEditPortsId(null)}
      />
    </div>
  );
}

export function PopRackTopologyPage() {
  const { popId } = useParams<{ popId: string }>();
  if (!popId) return <div className="msg msg--err" style={{ margin: 16 }}>POP não especificado.</div>;
  return (
    <ReactFlowProvider>
      <PopRackCanvas popId={popId} />
    </ReactFlowProvider>
  );
}
