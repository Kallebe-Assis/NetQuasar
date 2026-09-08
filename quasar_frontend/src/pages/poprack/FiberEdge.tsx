import { memo } from "react";
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";
import { Trash2 } from "lucide-react";
import { STANDARD_FIBER_SEQUENCE } from "../../lib/fiberSplitter";
import type { FiberEdgeData, FiberEdge as FiberEdgeT } from "./types";

/** Ligação de fibra porta-a-porta — cor real da fibra (mesma paleta STANDARD_FIBER_SEQUENCE
 * usada nos esquemas de splitter/cabo). Seleccionada, mostra swatches para trocar a cor. */
function FiberEdgeInner({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  selected,
  data,
}: EdgeProps<FiberEdgeT & { data: FiberEdgeData }>) {
  const [edgePath, labelX, labelY] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition });
  const color = data?.colorHex || "#64748b";
  const label = (data?.label ?? "").trim();

  function patchData(patch: Partial<FiberEdgeData>) {
    data?.onPatch?.(id, patch);
  }
  function removeEdge() {
    data?.onRemove?.(id);
  }

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={{ stroke: color, strokeWidth: selected ? 4 : 2.5 }} />
      <EdgeLabelRenderer>
        {label && !selected && (
          <div
            className="topo-edge-label"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          >
            {label}
          </div>
        )}
        {selected && (
          <div
            className="topo-edge-toolbar"
            style={{ transform: `translate(-50%, 0) translate(${labelX}px, ${labelY + 8}px)` }}
          >
            <div className="rack-edge-toolbar__swatches">
              {STANDARD_FIBER_SEQUENCE.map((f) => (
                <button
                  key={f.name}
                  type="button"
                  title={f.name}
                  className={`rack-edge-toolbar__swatch${data?.colorName === f.name ? " rack-edge-toolbar__swatch--active" : ""}`}
                  style={{ background: f.hex }}
                  onClick={() => patchData({ colorName: f.name, colorHex: f.hex })}
                />
              ))}
            </div>
            <div className="topo-edge-toolbar__row">
              <input
                className="input topo-edge-toolbar__name"
                style={{ fontSize: 11, padding: "2px 6px" }}
                value={data?.label ?? ""}
                onChange={(e) => patchData({ label: e.target.value })}
                placeholder="Nomear esta fibra…"
                onMouseDown={(e) => e.stopPropagation()}
              />
              <button type="button" className="btn btn--icon" title="Remover ligação" onClick={removeEdge}>
                <Trash2 size={12} />
              </button>
            </div>
          </div>
        )}
      </EdgeLabelRenderer>
    </>
  );
}

export const FiberEdge = memo(FiberEdgeInner);
