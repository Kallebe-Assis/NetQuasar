import { memo, useState } from "react";
import { NodeResizer, NodeToolbar, Position, useReactFlow, type NodeProps } from "@xyflow/react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { ExternalLink, Trash2 } from "lucide-react";
import { MIN_GROUP_SIZE } from "../../lib/topologyConnectionTypes";
import { apiFetch } from "../../lib/api";
import { APP_ROUTES } from "../../app/routes";
import type { GroupNodeData, TopologyNode } from "./types";

/**
 * Agrupador visual "POP" — quadrado ou círculo, arrastável/redimensionável, com label editável
 * (duplo clique). Equipamentos soltos dentro da área ficam com parentId = este grupo
 * (reparenting feito em TopologyPage via onNodeDragStop + getIntersectingNodes). Fica sempre no
 * plano de trás (zIndex negativo, atribuído em TopologyPage.tsx) — nunca sobrepõe equipamentos
 * nem a barra de uma ligação seleccionada. NodeToolbar remove só este POP (desagrupa, não apaga
 * os equipamentos lá dentro — ver removeNode em TopologyPage.tsx), e — vinculando este
 * agrupador a um POP real (GET /api/v1/pops) — abre a Topologia 2D desse POP específico.
 * updateNodeData é seguro aqui (ao contrário de setEdges directo, o bug corrigido em
 * ConnectionEdge.tsx): num flow controlado, ele dispara onNodesChange (applyNodeChanges), que
 * TopologyPage.tsx já aplica ao próprio estado — não é sobrescrito no próximo render.
 */
function GroupNodeInner({ id, data, selected }: NodeProps<TopologyNode & { data: GroupNodeData }>) {
  const { updateNode, updateNodeData } = useReactFlow();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(data.label);

  const popsQ = useQuery({
    queryKey: ["topology-pops-picker"],
    queryFn: () => apiFetch<{ pops: Array<{ id: string; description: string }> }>("/api/v1/pops"),
    staleTime: 5 * 60 * 1000,
  });
  const pops = popsQ.data?.pops ?? [];

  function commit() {
    updateNodeData(id, { label: draft.trim() || "POP" });
    setEditing(false);
  }

  return (
    <div
      className="topo-group"
      style={{
        width: "100%",
        height: "100%",
        borderRadius: data.shape === "circle" ? "50%" : 12,
        borderColor: data.color || "var(--border)",
      }}
    >
      <NodeToolbar isVisible={selected} position={Position.Top} style={{ zIndex: 1000 }}>
        <div className="topo-group-toolbar">
          <select
            className="input"
            style={{ fontSize: 11, padding: "2px 4px", maxWidth: 160 }}
            value={data.popId ?? ""}
            onChange={(e) => updateNodeData(id, { popId: e.target.value || null })}
            onPointerDown={(e) => e.stopPropagation()}
            title="Vincular este agrupador a um POP real — abre um atalho para a topologia 2D dele"
          >
            <option value="">— Sem POP vinculado —</option>
            {pops.map((p) => (
              <option key={p.id} value={p.id}>
                {p.description}
              </option>
            ))}
          </select>
          {data.popId ? (
            <Link
              to={APP_ROUTES.popRack(data.popId)}
              className="btn btn--icon topo-node-toolbar__btn"
              title="Abrir topologia 2D deste POP"
              onPointerDown={(e) => e.stopPropagation()}
            >
              <ExternalLink size={13} />
            </Link>
          ) : null}
          <button
            type="button"
            className="btn btn--icon topo-node-toolbar__btn"
            title="Remover este POP (mantém os equipamentos dentro dele)"
            onClick={() => data.onRemove?.(id)}
          >
            <Trash2 size={13} />
          </button>
        </div>
      </NodeToolbar>
      <NodeResizer
        isVisible={selected}
        minWidth={MIN_GROUP_SIZE.width}
        minHeight={MIN_GROUP_SIZE.height}
        lineStyle={{ borderColor: data.color || "var(--accent)" }}
        handleStyle={{ background: data.color || "var(--accent)" }}
        onResize={(_, params) => updateNode(id, { width: params.width, height: params.height })}
      />
      <div className="topo-group__label" onDoubleClick={() => setEditing(true)}>
        {editing ? (
          <input
            autoFocus
            className="input"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === "Enter") commit();
              if (e.key === "Escape") {
                setDraft(data.label);
                setEditing(false);
              }
            }}
            style={{ fontSize: 12, padding: "2px 6px", width: 160 }}
          />
        ) : (
          <span>
            {data.label || "POP"}
            {data.popId ? (
              <Link
                to={APP_ROUTES.popRack(data.popId)}
                title="Abrir topologia 2D deste POP"
                style={{ marginLeft: 6, verticalAlign: -2 }}
                onClick={(e) => e.stopPropagation()}
              >
                <ExternalLink size={12} />
              </Link>
            ) : null}
          </span>
        )}
      </div>
    </div>
  );
}

export const GroupNode = memo(GroupNodeInner);
