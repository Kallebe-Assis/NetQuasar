import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  ConnectionMode,
  Controls,
  MiniMap,
  reconnectEdge,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type OnConnect,
  type OnReconnect,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import "./topology/topology.css";
import { Circle, Redo2, Save, Settings, Square, Undo2 } from "lucide-react";
import { ConfirmModal } from "../components/ConfirmModal";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { can, isAdminUser } from "../lib/auth";
import {
  DEFAULT_CONNECTION_TYPE,
  DEFAULT_EDGE_ICON_SIZE,
  DEFAULT_GROUP_SIZE,
  DEFAULT_NODE_SIZE,
  TOPOLOGY_CONNECTION_TYPES,
  type TopologyConnectionType,
} from "../lib/topologyConnectionTypes";
import { ConnectionEdge } from "./topology/ConnectionEdge";
import { DeviceListPanel, TOPOLOGY_DRAG_MIME, TOPOLOGY_MANUAL_DRAG_MIME } from "./topology/DeviceListPanel";
import { DeviceNode } from "./topology/DeviceNode";
import { GroupNode } from "./topology/GroupNode";
import { ManualEquipmentNode } from "./topology/ManualEquipmentNode";
import { TopologySettingsModal, type TopologyProjectSummary } from "./topology/TopologySettingsModal";
import type {
  ConnectionEdgeData,
  DeviceNodeData,
  GroupNodeData,
  ManualNodeData,
  TopologyDevice,
  TopologyDocument,
} from "./topology/types";
import { emptyTopologyDocument } from "./topology/types";
import type { ManualEquipmentKind } from "../lib/topologyManualKinds";

const LAST_PROJECT_KEY = "netquasar.topology.lastProjectId";

const nodeTypes = { device: DeviceNode, pop: GroupNode, manual: ManualEquipmentNode };
const edgeTypes = { typed: ConnectionEdge };

function docToFlow(doc: TopologyDocument, devices: Map<string, TopologyDevice>) {
  const nodes: Node[] = [];
  for (const n of doc.nodes) {
    if (n.manual) {
      nodes.push({
        id: n.id,
        type: "manual",
        position: { x: n.x, y: n.y },
        width: n.width ?? DEFAULT_NODE_SIZE,
        height: n.height ?? DEFAULT_NODE_SIZE,
        parentId: n.parent_id,
        extent: n.parent_id ? "parent" : undefined,
        data: {
          kind: n.manual.kind as ManualEquipmentKind,
          description: n.manual.description ?? "",
          ip: n.manual.ip ?? null,
        } satisfies ManualNodeData,
      });
      continue;
    }
    const dev = devices.get(n.id);
    if (!dev) continue; // equipamento apagado entretanto — não desenha nó fantasma
    nodes.push({
      id: n.id,
      type: "device",
      position: { x: n.x, y: n.y },
      width: n.width ?? DEFAULT_NODE_SIZE,
      height: n.height ?? DEFAULT_NODE_SIZE,
      parentId: n.parent_id,
      extent: n.parent_id ? "parent" : undefined,
      data: { deviceId: dev.id, description: dev.description, category: dev.category, ip: dev.ip } satisfies DeviceNodeData,
    });
  }
  const groupNodes: Node[] = doc.groups.map((g) => ({
    id: g.id,
    type: "pop",
    position: { x: g.x, y: g.y },
    width: g.width,
    height: g.height,
    zIndex: -1, // POP sempre no plano de trás — nunca sobrepõe equipamentos nem a barra de uma ligação seleccionada.
    data: { shape: g.shape, label: g.label, color: g.color } satisfies GroupNodeData,
  }));
  // Grupos primeiro no array (parent antes do filho — exigência do React Flow para z-index/render).
  const orderedNodes = [...groupNodes, ...nodes];
  const edges: Edge[] = doc.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    sourceHandle: e.source_handle,
    targetHandle: e.target_handle,
    type: "typed",
    data: { connType: e.type, iconSize: e.icon_size ?? DEFAULT_EDGE_ICON_SIZE, customLabel: e.label } satisfies ConnectionEdgeData,
  }));
  return { nodes: orderedNodes, edges };
}

function flowToDoc(nodes: Node[], edges: Edge[]): TopologyDocument {
  const doc = emptyTopologyDocument();
  for (const n of nodes) {
    if (n.type === "pop") {
      const data = n.data as GroupNodeData;
      doc.groups.push({
        id: n.id,
        shape: data.shape,
        x: n.position.x,
        y: n.position.y,
        width: (n.width as number) ?? DEFAULT_GROUP_SIZE.width,
        height: (n.height as number) ?? DEFAULT_GROUP_SIZE.height,
        label: data.label,
        color: data.color,
      });
      continue;
    }
    if (n.type === "manual") {
      const data = n.data as ManualNodeData;
      doc.nodes.push({
        id: n.id,
        x: n.position.x,
        y: n.position.y,
        width: n.width as number | undefined,
        height: n.height as number | undefined,
        parent_id: n.parentId,
        manual: { kind: data.kind, description: data.description || undefined, ip: data.ip || undefined },
      });
      continue;
    }
    doc.nodes.push({
      id: n.id,
      x: n.position.x,
      y: n.position.y,
      width: n.width as number | undefined,
      height: n.height as number | undefined,
      parent_id: n.parentId,
    });
  }
  for (const e of edges) {
    const data = (e.data ?? {}) as Partial<ConnectionEdgeData>;
    doc.edges.push({
      id: e.id,
      source: e.source,
      target: e.target,
      source_handle: e.sourceHandle ?? undefined,
      target_handle: e.targetHandle ?? undefined,
      type: data.connType ?? DEFAULT_CONNECTION_TYPE,
      label: data.customLabel,
      icon_size: data.iconSize,
    });
  }
  return doc;
}

// Ordena o array de nós garantindo que todo grupo ("pop") vem antes dos filhos — exigência
// do React Flow para parentId funcionar correctamente (z-index e cálculo de posição relativa).
function reorderForParenting(nodes: Node[]): Node[] {
  const groups = nodes.filter((n) => n.type === "pop");
  const others = nodes.filter((n) => n.type !== "pop");
  return [...groups, ...others];
}

function TopologyCanvas() {
  const { push: pushToast } = useAppToast();
  const qc = useQueryClient();
  const canMutate = isAdminUser() || can("map.manage");
  const reactFlow = useReactFlow();
  const wrapperRef = useRef<HTMLDivElement>(null);
  const addCounterRef = useRef(0);
  // Guarda o id do projecto já hidratado no canvas — diferente de um simples booleano porque
  // trocar de projecto precisa de re-hidratar (o "só uma vez" de antes assumia 1 documento só).
  const hydratedProjectIdRef = useRef<string | null>(null);

  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [dirty, setDirty] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [colorOverrides, setColorOverrides] = useState<Partial<Record<TopologyConnectionType, string>>>({});
  const [activeProjectId, setActiveProjectId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(LAST_PROJECT_KEY);
    } catch {
      return null;
    }
  });
  const [pendingProjectId, setPendingProjectId] = useState<string | null>(null);

  const devicesQ = useQuery({
    queryKey: ["topology-devices"],
    queryFn: () => apiFetch<{ devices: Array<{ id: string; description: string; category: string; ip: string | null }> }>("/api/v1/devices"),
  });
  const projectsQ = useQuery({
    queryKey: ["topology-projects"],
    queryFn: () => apiFetch<{ items: TopologyProjectSummary[] }>("/api/v1/topology/projects"),
  });
  const projects = useMemo(() => projectsQ.data?.items ?? [], [projectsQ.data]);

  // Assim que a lista de projectos chega, garante um projecto activo: o último usado (guardado
  // no navegador) se ainda existir, senão o primeiro da lista.
  useEffect(() => {
    if (projects.length === 0) return;
    if (activeProjectId && projects.some((p) => p.id === activeProjectId)) return;
    setActiveProjectId(projects[0].id);
  }, [projects, activeProjectId]);

  useEffect(() => {
    if (!activeProjectId) return;
    try {
      localStorage.setItem(LAST_PROJECT_KEY, activeProjectId);
    } catch {
      /* localStorage indisponível — só perde a conveniência de lembrar o último projecto */
    }
  }, [activeProjectId]);

  // Só activa depois de confirmar que activeProjectId é um projecto que existe de facto na
  // lista actual — evita disparar com um id obsoleto vindo do localStorage (ex.: projecto
  // apagado noutra sessão) antes do efeito acima ter tido a chance de o corrigir, o que
  // devolveria 404 e travava a tela inteira no ecrã de erro.
  const canvasQ = useQuery({
    queryKey: ["topology-canvas", activeProjectId],
    queryFn: () => apiFetch<TopologyDocument>(`/api/v1/topology/projects/${activeProjectId}`),
    enabled: !!activeProjectId && projects.some((p) => p.id === activeProjectId),
  });

  const devices: TopologyDevice[] = useMemo(
    () => (devicesQ.data?.devices ?? []).map((d) => ({ id: d.id, description: d.description, category: d.category, ip: d.ip })),
    [devicesQ.data],
  );
  const devicesById = useMemo(() => new Map(devices.map((d) => [d.id, d])), [devices]);

  // Hidrata o canvas assim que devices + o projecto activo chegarem — repete sempre que
  // activeProjectId mudar (troca de projecto), não só na primeira vez.
  useEffect(() => {
    if (!activeProjectId) return;
    if (hydratedProjectIdRef.current === activeProjectId) return;
    if (!devicesQ.data || !canvasQ.data) return;
    const { nodes: n, edges: e } = docToFlow(canvasQ.data, devicesById);
    setNodes(n);
    setEdges(e);
    setColorOverrides(canvasQ.data.settings?.connection_colors ?? {});
    setDirty(false);
    hydratedProjectIdRef.current = activeProjectId;
  }, [activeProjectId, devicesQ.data, canvasQ.data, devicesById]);

  // Mantém descrição/categoria/IP dos nós já colocados em sincronia se o cadastro mudar
  // (sem mexer em posição/tamanho), depois da hidratação inicial.
  useEffect(() => {
    if (!hydratedProjectIdRef.current) return;
    setNodes((nds) =>
      nds.map((n) => {
        if (n.type !== "device") return n;
        const dev = devicesById.get(n.id);
        if (!dev) return n;
        const data = n.data as DeviceNodeData;
        if (data.description === dev.description && data.category === dev.category && data.ip === dev.ip) return n;
        return { ...n, data: { ...data, description: dev.description, category: dev.category, ip: dev.ip } };
      }),
    );
  }, [devicesById]);

  const placedIds = useMemo(() => new Set(nodes.filter((n) => n.type === "device").map((n) => n.id)), [nodes]);

  const markDirty = useCallback(() => {
    if (hydratedProjectIdRef.current) setDirty(true);
  }, []);

  const onNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes((nds) => applyNodeChanges(changes, nds));
    markDirty();
  }, [markDirty]);

  const onEdgesChange = useCallback((changes: EdgeChange[]) => {
    setEdges((eds) => applyEdgeChanges(changes, eds));
    markDirty();
  }, [markDirty]);

  // Callbacks reais (mexem no estado do próprio TopologyCanvas) injectados no `data` de cada
  // aresta abaixo — ConnectionEdge.tsx não pode chamar useReactFlow().setEdges directamente
  // porque <ReactFlow edges=…> é controlado por este estado (ver comentário em types.ts).
  const patchEdgeData = useCallback((id: string, patch: Partial<ConnectionEdgeData>) => {
    setEdges((eds) => eds.map((e) => (e.id === id ? { ...e, data: { ...(e.data as ConnectionEdgeData), ...patch } } : e)));
    markDirty();
  }, [markDirty]);
  const removeEdgeById = useCallback((id: string) => {
    setEdges((eds) => eds.filter((e) => e.id !== id));
    markDirty();
  }, [markDirty]);
  const edgesForFlow = useMemo(
    () =>
      edges.map((e) => ({
        ...e,
        data: { ...(e.data as ConnectionEdgeData), colorOverrides, onPatch: patchEdgeData, onRemove: removeEdgeById },
      })),
    [edges, colorOverrides, patchEdgeData, removeEdgeById],
  );

  // removeNode — remove um único equipamento (+ as ligações dele) ou um único POP (desagrupa os
  // equipamentos lá dentro em vez de os apagar também: um POP é só um agrupador visual). Mesmo
  // padrão de callback-via-data de patchEdgeData/removeEdgeById acima — injectado no `data` de
  // cada nó via nodesForFlow, não useReactFlow().setNodes/deleteElements directamente.
  const removeNode = useCallback(
    (id: string) => {
      const target = reactFlow.getNode(id);
      if (!target) return;
      if (target.type === "pop") {
        const childAbsolutes = new Map<string, { x: number; y: number }>();
        for (const n of reactFlow.getNodes()) {
          if (n.parentId === id) {
            const internal = reactFlow.getInternalNode(n.id);
            childAbsolutes.set(n.id, internal?.internals.positionAbsolute ?? n.position);
          }
        }
        setNodes((nds) =>
          nds
            .filter((n) => n.id !== id)
            .map((n) =>
              n.parentId === id
                ? { ...n, parentId: undefined, extent: undefined, position: childAbsolutes.get(n.id) ?? n.position }
                : n,
            ),
        );
      } else {
        setNodes((nds) => nds.filter((n) => n.id !== id));
      }
      setEdges((eds) => eds.filter((e) => e.source !== id && e.target !== id));
      markDirty();
    },
    [reactFlow, markDirty],
  );
  // Mesmo padrão de patchEdgeData acima, mas para nós — hoje só ManualEquipmentNode.tsx usa isto
  // (editar descrição/IP inline, já que não há cadastro por trás para ler esses campos); os
  // outros tipos de nó simplesmente ignoram o onPatch injectado.
  const patchNodeData = useCallback((id: string, patch: Record<string, unknown>) => {
    setNodes((nds) => nds.map((n) => (n.id === id ? { ...n, data: { ...(n.data as Record<string, unknown>), ...patch } } : n)));
    markDirty();
  }, [markDirty]);
  const nodesForFlow = useMemo(
    () =>
      nodes.map((n) => ({
        ...n,
        data: { ...(n.data as Record<string, unknown>), onRemove: removeNode, onPatch: patchNodeData },
      })),
    [nodes, removeNode, patchNodeData],
  );

  // --- desfazer/refazer -------------------------------------------------------------------
  // Histórico "debounced": qualquer mudança em nodes/edges agenda um commit 400ms depois; se
  // outra mudança chegar antes disso (ex.: arrastar um nó, que dispara dezenas de eventos por
  // segundo), o temporizador reinicia — um gesto contínuo (arrastar/redimensionar) vira 1 só
  // passo de undo, não uma pilha de micro-passos. baselineRef guarda o último estado
  // "assentado"; ao assentar de novo, o baseline anterior entra em `past` e o novo estado vira
  // o baseline seguinte. skipRef evita que o próprio undo/redo (que também muda nodes/edges)
  // seja gravado como um novo passo.
  type Snap = { nodes: Node[]; edges: Edge[] };
  const [past, setPast] = useState<Snap[]>([]);
  const [future, setFuture] = useState<Snap[]>([]);
  const baselineRef = useRef<Snap | null>(null);
  const skipRef = useRef(false);
  const debounceRef = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (!hydratedProjectIdRef.current) return;
    if (skipRef.current) {
      skipRef.current = false;
      baselineRef.current = { nodes, edges };
      return;
    }
    window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(() => {
      const baseline = baselineRef.current;
      if (baseline) {
        setPast((p) => [...p.slice(-49), baseline]);
        setFuture([]);
      }
      baselineRef.current = { nodes, edges };
    }, 400);
    return () => window.clearTimeout(debounceRef.current);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes, edges]);

  const undo = useCallback(() => {
    if (past.length === 0) return;
    window.clearTimeout(debounceRef.current);
    const prev = past[past.length - 1];
    setPast(past.slice(0, -1));
    setFuture([...future, { nodes, edges }]);
    skipRef.current = true;
    setNodes(prev.nodes);
    setEdges(prev.edges);
    setDirty(true);
  }, [past, future, nodes, edges]);

  const redo = useCallback(() => {
    if (future.length === 0) return;
    window.clearTimeout(debounceRef.current);
    const next = future[future.length - 1];
    setFuture(future.slice(0, -1));
    setPast([...past, { nodes, edges }]);
    skipRef.current = true;
    setNodes(next.nodes);
    setEdges(next.edges);
    setDirty(true);
  }, [past, future, nodes, edges]);

  // Ctrl+Z / Ctrl+Y (ou Ctrl+Shift+Z) — ignorado quando o foco está num campo de texto (ex.:
  // renomear um POP) para não atropelar o undo nativo do próprio input.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const el = document.activeElement;
      const typing = el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement;
      if (typing || !(e.ctrlKey || e.metaKey)) return;
      if (e.key.toLowerCase() === "z" && !e.shiftKey) {
        e.preventDefault();
        undo();
      } else if (e.key.toLowerCase() === "y" || (e.key.toLowerCase() === "z" && e.shiftKey)) {
        e.preventDefault();
        redo();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [undo, redo]);

  const onConnect: OnConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) => {
        const deselected: Edge[] = eds.map((e) => ({ ...e, selected: false }));
        const newEdge: Edge = {
          ...connection,
          id: `edge-${crypto.randomUUID()}`,
          type: "typed",
          selected: true,
          data: { connType: DEFAULT_CONNECTION_TYPE, iconSize: DEFAULT_EDGE_ICON_SIZE } satisfies ConnectionEdgeData,
        };
        return addEdge(newEdge, deselected);
      });
      markDirty();
    },
    [markDirty],
  );

  // Reconectar uma ligação já existente (arrastar a ponta dela para outro lado/equipamento) —
  // sem isto, a única forma de mudar onde uma ligação chega era apagar e refazer. Exige
  // `onReconnect` explícito (edgesReconnectable sozinho só mostra a alça de arrastar).
  const onReconnect: OnReconnect = useCallback(
    (oldEdge: Edge, newConnection: Connection) => {
      setEdges((eds) => reconnectEdge(oldEdge, newConnection, eds));
      markDirty();
    },
    [markDirty],
  );

  // Reparenting: ao largar um equipamento, verifica se ficou dentro de algum grupo "pop" e
  // ajusta parentId + posição relativa (ou volta a absoluta se saiu de todos os grupos).
  const onNodeDragStop = useCallback(
    (_: unknown, node: Node) => {
      if (node.type !== "device") return;
      const internal = reactFlow.getInternalNode(node.id);
      const abs = internal?.internals.positionAbsolute ?? node.position;
      const w = (internal?.measured.width ?? node.width ?? DEFAULT_NODE_SIZE) as number;
      const h = (internal?.measured.height ?? node.height ?? DEFAULT_NODE_SIZE) as number;
      const centerX = abs.x + w / 2;
      const centerY = abs.y + h / 2;

      const groups = reactFlow.getNodes().filter((n) => n.type === "pop");
      const group = groups.find((g) => {
        const gw = (g.width as number) ?? DEFAULT_GROUP_SIZE.width;
        const gh = (g.height as number) ?? DEFAULT_GROUP_SIZE.height;
        return centerX >= g.position.x && centerX <= g.position.x + gw && centerY >= g.position.y && centerY <= g.position.y + gh;
      });

      setNodes((nds) => {
        let changed = false;
        const next = nds.map((n) => {
          if (n.id !== node.id) return n;
          if (group && n.parentId !== group.id) {
            changed = true;
            return { ...n, parentId: group.id, extent: "parent" as const, position: { x: abs.x - group.position.x, y: abs.y - group.position.y } };
          }
          if (!group && n.parentId) {
            changed = true;
            return { ...n, parentId: undefined, extent: undefined, position: { x: abs.x, y: abs.y } };
          }
          return n;
        });
        return changed ? reorderForParenting(next) : nds;
      });
      markDirty();
    },
    [reactFlow, markDirty],
  );

  const nextGridPosition = useCallback(() => {
    const n = addCounterRef.current++;
    return { x: 80 + (n % 6) * 150, y: 80 + Math.floor(n / 6) * 130 };
  }, []);

  const addDeviceNode = useCallback(
    (device: TopologyDevice) => {
      setNodes((nds) => {
        if (nds.some((n) => n.id === device.id)) {
          toastOk(pushToast, `${device.description} já está no canvas.`);
          return nds;
        }
        const pos = nextGridPosition();
        return reorderForParenting([
          ...nds,
          {
            id: device.id,
            type: "device",
            position: pos,
            width: DEFAULT_NODE_SIZE,
            height: DEFAULT_NODE_SIZE,
            data: { deviceId: device.id, description: device.description, category: device.category, ip: device.ip } satisfies DeviceNodeData,
          },
        ]);
      });
      markDirty();
    },
    [nextGridPosition, markDirty, pushToast],
  );

  const addGroupNode = useCallback(
    (shape: "rect" | "circle") => {
      const id = `pop-${crypto.randomUUID()}`;
      const pos = nextGridPosition();
      setNodes((nds) =>
        reorderForParenting([
          ...nds,
          {
            id,
            type: "pop",
            position: pos,
            width: DEFAULT_GROUP_SIZE.width,
            height: DEFAULT_GROUP_SIZE.height,
            zIndex: -1,
            data: { shape, label: "Novo POP" } satisfies GroupNodeData,
          },
        ]),
      );
      markDirty();
    },
    [nextGridPosition, markDirty],
  );

  const addManualNode = useCallback(
    (kind: ManualEquipmentKind) => {
      const id = `manual-${crypto.randomUUID()}`;
      const pos = nextGridPosition();
      setNodes((nds) =>
        reorderForParenting([
          ...nds,
          {
            id,
            type: "manual",
            position: pos,
            width: DEFAULT_NODE_SIZE,
            height: DEFAULT_NODE_SIZE,
            data: { kind, description: "", ip: null } satisfies ManualNodeData,
          },
        ]),
      );
      markDirty();
    },
    [nextGridPosition, markDirty],
  );

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const rawManual = event.dataTransfer.getData(TOPOLOGY_MANUAL_DRAG_MIME);
      if (rawManual) {
        const kind = rawManual as ManualEquipmentKind;
        const flowPos = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
        const id = `manual-${crypto.randomUUID()}`;
        setNodes((nds) =>
          reorderForParenting([
            ...nds,
            {
              id,
              type: "manual",
              position: flowPos,
              width: DEFAULT_NODE_SIZE,
              height: DEFAULT_NODE_SIZE,
              data: { kind, description: "", ip: null } satisfies ManualNodeData,
            },
          ]),
        );
        markDirty();
        return;
      }
      const raw = event.dataTransfer.getData(TOPOLOGY_DRAG_MIME);
      if (!raw) return;
      const device = JSON.parse(raw) as TopologyDevice;
      const flowPos = reactFlow.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      setNodes((nds) => {
        const existing = nds.find((n) => n.id === device.id);
        if (existing) {
          return nds.map((n) => (n.id === device.id ? { ...n, position: flowPos, parentId: undefined, extent: undefined } : n));
        }
        return reorderForParenting([
          ...nds,
          {
            id: device.id,
            type: "device",
            position: flowPos,
            width: DEFAULT_NODE_SIZE,
            height: DEFAULT_NODE_SIZE,
            data: { deviceId: device.id, description: device.description, category: device.category, ip: device.ip } satisfies DeviceNodeData,
          },
        ]);
      });
      markDirty();
    },
    [reactFlow, markDirty],
  );

  const saveMut = useCallback(async () => {
    if (!activeProjectId) return;
    const doc = flowToDoc(nodes, edges);
    doc.settings = { ...(doc.settings ?? {}), connection_colors: colorOverrides };
    try {
      await apiFetch(`/api/v1/topology/projects/${activeProjectId}`, { method: "PUT", json: doc });
      setDirty(false);
      toastOk(pushToast, "Topologia salva.");
      void qc.invalidateQueries({ queryKey: ["topology-projects"] });
    } catch (e) {
      toastErr(pushToast, e, "Falha ao salvar a topologia.");
    }
  }, [activeProjectId, nodes, edges, colorOverrides, pushToast, qc]);

  function clearAll() {
    setNodes([]);
    setEdges([]);
    setColorOverrides({});
    markDirty();
  }

  function buildExportDoc(): TopologyDocument {
    return flowToDoc(nodes, edges);
  }

  function applyImportedDoc(doc: TopologyDocument) {
    const { nodes: n, edges: e } = docToFlow(doc, devicesById);
    setNodes(n);
    setEdges(e);
    if (doc.settings?.connection_colors) setColorOverrides(doc.settings.connection_colors);
    markDirty();
    toastOk(pushToast, "Diagrama importado — clique em «Salvar» para gravar no projeto atual.");
  }

  function requestSwitchProject(id: string) {
    if (id === activeProjectId) return;
    if (dirty) {
      setPendingProjectId(id);
      return;
    }
    setActiveProjectId(id);
  }

  async function handleCreateProject(name: string) {
    try {
      const res = await apiFetch<{ id: string; name: string }>("/api/v1/topology/projects", { method: "POST", json: { name } });
      await qc.invalidateQueries({ queryKey: ["topology-projects"] });
      toastOk(pushToast, `Projeto «${name}» criado.`);
      requestSwitchProject(res.id);
    } catch (e) {
      toastErr(pushToast, e, "Falha ao criar projeto.");
    }
  }

  async function handleRenameProject(id: string, name: string) {
    try {
      await apiFetch(`/api/v1/topology/projects/${id}`, { method: "PATCH", json: { name } });
      await qc.invalidateQueries({ queryKey: ["topology-projects"] });
      toastOk(pushToast, "Projeto renomeado.");
    } catch (e) {
      toastErr(pushToast, e, "Falha ao renomear projeto.");
    }
  }

  async function handleDeleteProject(id: string) {
    try {
      await apiFetch(`/api/v1/topology/projects/${id}`, { method: "DELETE" });
      if (id === activeProjectId) {
        hydratedProjectIdRef.current = null;
        setActiveProjectId(null);
      }
      await qc.invalidateQueries({ queryKey: ["topology-projects"] });
      toastOk(pushToast, "Projeto removido.");
    } catch (e) {
      toastErr(pushToast, e, "Falha ao remover projeto.");
    }
  }

  if (devicesQ.isLoading || projectsQ.isLoading || canvasQ.isLoading) return <p>A carregar topologia…</p>;
  if (devicesQ.isError) return <div className="msg msg--err">{(devicesQ.error as Error).message}</div>;
  if (projectsQ.isError) return <div className="msg msg--err">{(projectsQ.error as Error).message}</div>;
  if (canvasQ.isError) return <div className="msg msg--err">{(canvasQ.error as Error).message}</div>;

  return (
    <div className="topo-page">
      <div className="page-heading" style={{ marginBottom: 4 }}>
        <h1>Topologia</h1>
      </div>

      <div className="topo-toolbar">
        {projects.length > 1 ? (
          <select
            className="input"
            style={{ maxWidth: 220 }}
            value={activeProjectId ?? ""}
            onChange={(e) => requestSwitchProject(e.target.value)}
            aria-label="Projeto de topologia"
          >
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        ) : null}
        {canMutate && (
          <button type="button" className="btn btn--primary" disabled={!dirty} onClick={() => void saveMut()}>
            <Save size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
            {dirty ? "Salvar" : "Salvo"}
          </button>
        )}
        {canMutate && (
          <button type="button" className="btn btn--icon" title="Desfazer (Ctrl+Z)" disabled={past.length === 0} onClick={undo}>
            <Undo2 size={14} />
          </button>
        )}
        {canMutate && (
          <button type="button" className="btn btn--icon" title="Refazer (Ctrl+Y)" disabled={future.length === 0} onClick={redo}>
            <Redo2 size={14} />
          </button>
        )}
        {canMutate && (
          <button type="button" className="btn" onClick={() => addGroupNode("rect")}>
            <Square size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
            Adicionar POP (quadrado)
          </button>
        )}
        {canMutate && (
          <button type="button" className="btn" onClick={() => addGroupNode("circle")}>
            <Circle size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
            Adicionar POP (círculo)
          </button>
        )}
        <button type="button" className="btn btn--icon" title="Configurações da topologia" onClick={() => setSettingsOpen(true)}>
          <Settings size={14} />
        </button>
        <div className="topo-toolbar__legend">
          {TOPOLOGY_CONNECTION_TYPES.map((t) => (
            <span key={t.id} className="topo-toolbar__legend-item">
              <span className="topo-toolbar__legend-dot" style={{ background: colorOverrides[t.id] ?? t.color }} />
              {t.label}
            </span>
          ))}
        </div>
      </div>

      <div className="topo-body">
        <div className="topo-canvas-wrap" ref={wrapperRef}>
          <ReactFlow
            nodes={nodesForFlow}
            edges={edgesForFlow}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            onNodesChange={canMutate ? onNodesChange : undefined}
            onEdgesChange={canMutate ? onEdgesChange : undefined}
            onConnect={canMutate ? onConnect : undefined}
            onReconnect={canMutate ? onReconnect : undefined}
            edgesReconnectable={canMutate}
            onNodeDragStop={canMutate ? onNodeDragStop : undefined}
            onDrop={canMutate ? onDrop : undefined}
            onDragOver={canMutate ? (e) => e.preventDefault() : undefined}
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

        {canMutate && (
          <DeviceListPanel devices={devices} placedIds={placedIds} onAddDevice={addDeviceNode} onAddManual={addManualNode} />
        )}
      </div>

      <TopologySettingsModal
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        canMutate={canMutate}
        colorOverrides={colorOverrides}
        onColorsChange={(next) => {
          setColorOverrides(next);
          markDirty();
        }}
        projects={projects}
        projectsLoading={projectsQ.isLoading}
        activeProjectId={activeProjectId}
        onSwitchProject={requestSwitchProject}
        onCreateProject={handleCreateProject}
        onRenameProject={handleRenameProject}
        onDeleteProject={handleDeleteProject}
        buildExportDoc={buildExportDoc}
        onImportDoc={applyImportedDoc}
        onClearAll={clearAll}
      />

      <ConfirmModal
        open={!!pendingProjectId}
        title="Trocar de projeto"
        message="Há alterações não salvas neste projeto — trocar de projeto agora descarta-as (o que já está salvo no servidor não é afetado). Continuar?"
        confirmLabel="Trocar mesmo assim"
        danger
        onCancel={() => setPendingProjectId(null)}
        onConfirm={() => {
          if (pendingProjectId) setActiveProjectId(pendingProjectId);
          setPendingProjectId(null);
        }}
      />
    </div>
  );
}

export function TopologyPage() {
  return (
    <ReactFlowProvider>
      <TopologyCanvas />
    </ReactFlowProvider>
  );
}
