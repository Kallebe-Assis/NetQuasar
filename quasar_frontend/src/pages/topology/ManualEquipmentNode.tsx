import { memo } from "react";
import { Handle, NodeResizer, NodeToolbar, Position, useReactFlow, type NodeProps } from "@xyflow/react";
import { Trash2 } from "lucide-react";
import { manualKindMeta } from "../../lib/topologyManualKinds";
import { MAX_NODE_SIZE, MIN_NODE_SIZE } from "../../lib/topologyConnectionTypes";
import type { ManualNodeData, TopologyNode } from "./types";

// Mesmo conjunto de 4 lados de DeviceNode.tsx — ver comentário lá sobre connectionMode="loose".
const SIDES: Array<{ id: string; position: Position }> = [
  { id: "top", position: Position.Top },
  { id: "bottom", position: Position.Bottom },
  { id: "left", position: Position.Left },
  { id: "right", position: Position.Right },
];

/**
 * Nó "avulso" no canvas de Topologia — um elemento de rede (switch/roteador/rádio/conversor de
 * mídia/ONU/OLT) sem cadastro em GET /api/v1/devices (ver lib/topologyManualKinds.ts). Descrição
 * e IP vivem só neste nó e são editáveis inline, na barra flutuante, quando seleccionado — ao
 * contrário de DeviceNode.tsx, aqui não há registro nenhum por trás para ler esses campos.
 */
function ManualEquipmentNodeInner({ id, data, selected, width, height }: NodeProps<TopologyNode & { data: ManualNodeData }>) {
  const { updateNode } = useReactFlow();
  const meta = manualKindMeta(data.kind);
  const Icon = meta.icon;
  const size = Math.min(width ?? 64, height ?? 64);

  function patch(p: Partial<ManualNodeData>) {
    data.onPatch?.(id, p);
  }

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
        <div className="topo-manual-toolbar">
          <input
            className="input"
            style={{ fontSize: 11, padding: "2px 6px" }}
            value={data.description ?? ""}
            placeholder="Descrição (opcional)"
            onChange={(e) => patch({ description: e.target.value })}
            onMouseDown={(e) => e.stopPropagation()}
          />
          <input
            className="input mono"
            style={{ fontSize: 11, padding: "2px 6px" }}
            value={data.ip ?? ""}
            placeholder="IP (opcional)"
            onChange={(e) => patch({ ip: e.target.value })}
            onMouseDown={(e) => e.stopPropagation()}
          />
          <button
            type="button"
            className="btn btn--icon topo-node-toolbar__btn"
            title="Remover este equipamento do diagrama"
            onClick={() => data.onRemove?.(id)}
          >
            <Trash2 size={13} />
          </button>
        </div>
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
        className={`topo-device-icon topo-device-icon--manual${selected ? " topo-device-icon--selected" : ""}`}
        style={{ width: "100%", flex: "1 1 auto" }}
        title={`${meta.label} — equipamento avulso (sem cadastro)`}
      >
        <Icon size="70%" strokeWidth={1.6} />
      </div>
      <div className="topo-device-label" style={{ fontSize: Math.max(9, size * 0.16) }}>
        <strong>{data.description || meta.label}</strong>
        {data.ip ? <span className="mono">{data.ip}</span> : null}
      </div>
    </div>
  );
}

export const ManualEquipmentNode = memo(ManualEquipmentNodeInner);
