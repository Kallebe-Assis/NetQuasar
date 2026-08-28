import { memo } from "react";
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";
import { Cable, Minus, Plus, Radio, Router, Trash2, Waves } from "lucide-react";
import {
  MAX_EDGE_ICON_SIZE,
  MIN_EDGE_ICON_SIZE,
  TOPOLOGY_CONNECTION_TYPES,
  connectionTypeMeta,
} from "../../lib/topologyConnectionTypes";
import type { ConnectionEdgeData, TopologyEdge } from "./types";

const TYPE_ICON: Record<string, typeof Cable> = {
  fibra: Cable,
  transporte: Waves,
  radio: Radio,
  utp: Router,
  vpn: Router,
  outro: Minus,
};

/**
 * Conexão tipada entre 2 equipamentos — cor/traço por tipo (lib/topologyConnectionTypes.ts),
 * ícone do tipo no meio da aresta, redimensionável. Seleccionada, mostra uma barra flutuante
 * para trocar o tipo, redimensionar o ícone ou remover.
 */
function ConnectionEdgeInner({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  selected,
  data,
}: EdgeProps<TopologyEdge & { data: ConnectionEdgeData }>) {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });
  const meta = connectionTypeMeta(data?.connType);
  const Icon = TYPE_ICON[meta.id] ?? Minus;
  const iconSize = data?.iconSize ?? 18;

  function patchData(patch: Partial<ConnectionEdgeData>) {
    data?.onPatch?.(id, patch);
  }
  function removeEdge() {
    data?.onRemove?.(id);
  }

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        style={{ stroke: meta.color, strokeWidth: selected ? 3 : 2, strokeDasharray: meta.dash }}
      />
      <EdgeLabelRenderer>
        <div
          className="topo-edge-icon"
          style={{
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            width: iconSize,
            height: iconSize,
            color: meta.color,
          }}
        >
          <Icon size={iconSize} strokeWidth={2} />
        </div>
        {selected && (
          <div
            className="topo-edge-toolbar"
            style={{ transform: `translate(-50%, 0) translate(${labelX}px, ${labelY + iconSize / 2 + 10}px)` }}
          >
            <select
              className="input"
              style={{ fontSize: 11, padding: "2px 6px" }}
              value={meta.id}
              onChange={(e) => patchData({ connType: e.target.value as ConnectionEdgeData["connType"] })}
            >
              {TOPOLOGY_CONNECTION_TYPES.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.label}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="btn btn--icon"
              title="Diminuir ícone"
              onClick={() => patchData({ iconSize: Math.max(MIN_EDGE_ICON_SIZE, iconSize - 4) })}
            >
              <Minus size={12} />
            </button>
            <button
              type="button"
              className="btn btn--icon"
              title="Aumentar ícone"
              onClick={() => patchData({ iconSize: Math.min(MAX_EDGE_ICON_SIZE, iconSize + 4) })}
            >
              <Plus size={12} />
            </button>
            <button type="button" className="btn btn--icon" title="Remover conexão" onClick={removeEdge}>
              <Trash2 size={12} />
            </button>
          </div>
        )}
      </EdgeLabelRenderer>
    </>
  );
}

export const ConnectionEdge = memo(ConnectionEdgeInner);
