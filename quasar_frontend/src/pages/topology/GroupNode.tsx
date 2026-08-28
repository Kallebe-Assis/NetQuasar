import { memo, useState } from "react";
import { NodeResizer, NodeToolbar, Position, useReactFlow, type NodeProps } from "@xyflow/react";
import { Trash2 } from "lucide-react";
import { MIN_GROUP_SIZE } from "../../lib/topologyConnectionTypes";
import type { GroupNodeData, TopologyNode } from "./types";

/**
 * Agrupador visual "POP" — quadrado ou círculo, arrastável/redimensionável, com label editável
 * (duplo clique). Equipamentos soltos dentro da área ficam com parentId = este grupo
 * (reparenting feito em TopologyPage via onNodeDragStop + getIntersectingNodes). Fica sempre no
 * plano de trás (zIndex negativo, atribuído em TopologyPage.tsx) — nunca sobrepõe equipamentos
 * nem a barra de uma ligação seleccionada. NodeToolbar remove só este POP (desagrupa, não apaga
 * os equipamentos lá dentro — ver removeNode em TopologyPage.tsx).
 */
function GroupNodeInner({ id, data, selected }: NodeProps<TopologyNode & { data: GroupNodeData }>) {
  const { updateNode, updateNodeData } = useReactFlow();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(data.label);

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
        <button
          type="button"
          className="btn btn--icon topo-node-toolbar__btn"
          title="Remover este POP (mantém os equipamentos dentro dele)"
          onClick={() => data.onRemove?.(id)}
        >
          <Trash2 size={13} />
        </button>
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
          <span>{data.label || "POP"}</span>
        )}
      </div>
    </div>
  );
}

export const GroupNode = memo(GroupNodeInner);
