import { memo } from "react";
import { Handle, NodeResizer, NodeToolbar, Position, useReactFlow, type NodeProps } from "@xyflow/react";
import { Trash2 } from "lucide-react";
import { deviceCategoryIcon } from "../../lib/deviceCategoryIcons";
import { MAX_NODE_SIZE, MIN_NODE_SIZE } from "../../lib/topologyConnectionTypes";
import type { DeviceNodeData, TopologyNode } from "./types";

// Um handle por lado, todos "source" com isConnectableStart/End — combinado com
// connectionMode="loose" em TopologyPage.tsx, permite arrastar uma ligação a partir de
// QUALQUER um dos 4 lados e terminá-la em qualquer um dos 4 lados de outro equipamento (antes,
// Top/Left só aceitavam ligações a chegar e Bottom/Right só deixavam começar por ali — dava a
// impressão de "só liga em cima/embaixo").
const SIDES: Array<{ id: string; position: Position }> = [
  { id: "top", position: Position.Top },
  { id: "bottom", position: Position.Bottom },
  { id: "left", position: Position.Left },
  { id: "right", position: Position.Right },
];

/**
 * Nó de equipamento no canvas de Topologia — ícone da categoria + descrição + IP.
 * NodeResizer dá as alças de redimensionar quando seleccionado ("aumentar o tamanho do
 * ícone do equipamento"), com proporção travada (é um ícone quadrado). NodeToolbar mostra um
 * botão de remover este equipamento sozinho quando seleccionado.
 */
function DeviceNodeInner({ id, data, selected, width, height }: NodeProps<TopologyNode & { data: DeviceNodeData }>) {
  const { updateNode } = useReactFlow();
  const Icon = deviceCategoryIcon(data.category);
  const size = Math.min(width ?? 64, height ?? 64);

  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 4,
        minWidth: MIN_NODE_SIZE,
        minHeight: MIN_NODE_SIZE,
      }}
    >
      <NodeToolbar isVisible={selected} position={Position.Top} style={{ zIndex: 1000 }}>
        <button
          type="button"
          className="btn btn--icon topo-node-toolbar__btn"
          title="Remover este equipamento do diagrama"
          onClick={() => data.onRemove?.(id)}
        >
          <Trash2 size={13} />
        </button>
      </NodeToolbar>
      <NodeResizer
        isVisible={selected}
        minWidth={MIN_NODE_SIZE}
        minHeight={MIN_NODE_SIZE}
        maxWidth={MAX_NODE_SIZE}
        maxHeight={MAX_NODE_SIZE}
        keepAspectRatio
        onResize={(_, params) => updateNode(id, { width: params.width, height: params.height })}
      />
      {SIDES.map((s) => (
        <Handle
          key={s.id}
          id={s.id}
          type="source"
          position={s.position}
          isConnectableStart
          isConnectableEnd
          style={{ opacity: 0 }}
        />
      ))}
      <div
        className={`topo-device-icon${selected ? " topo-device-icon--selected" : ""}`}
        style={{ width: "100%", flex: "1 1 auto" }}
      >
        <Icon size="70%" strokeWidth={1.6} />
      </div>
      <div className="topo-device-label" style={{ fontSize: Math.max(9, size * 0.16) }}>
        <strong>{data.description}</strong>
        {data.ip ? <span className="mono">{data.ip}</span> : null}
      </div>
    </div>
  );
}

export const DeviceNode = memo(DeviceNodeInner);
