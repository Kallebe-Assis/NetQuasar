import { memo, useState } from "react";
import { BaseEdge, EdgeLabelRenderer, getBezierPath, useReactFlow, type EdgeProps } from "@xyflow/react";
import { Plus, Trash2 } from "lucide-react";
import { STANDARD_FIBER_SEQUENCE } from "../../lib/fiberSplitter";
import {
  CABLE_TYPE_LABELS,
  EDGE_DASH_LABELS,
  EDGE_DASH_PATTERNS,
  NETWORK_CABLE_COLORS,
  type CableType,
  type EdgeDashStyle,
  type EdgeWaypoint,
  type FiberEdgeData,
  type FiberEdge as FiberEdgeT,
} from "./types";

type Point = { x: number; y: number };

/** Caminho em segmentos rectos origem→pontos→destino — usado só quando há pontos intermédios
 * (waypoints); sem eles o comportamento é o original (curva Bezier directa, ver getBezierPath). */
function polylinePath(points: Point[]): string {
  return points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ");
}

/** Ângulos de "auto ajuste" (a cada 45°: horizontal, vertical, diagonal) — arrastar um ponto do
 * caminho (waypoint) encosta nesses ângulos em relação aos pontos vizinhos (o troço antes/depois
 * dele), como as guias inteligentes de ferramentas de desenho. Devolve o ponto ajustado quando
 * cai dentro do threshold (em unidades do canvas, já compensado pelo zoom pelo chamador), senão
 * devolve o ponto tal como veio. */
function snapToNeighborAngles(point: Point, neighbors: Array<Point | undefined>, thresholdFlow: number): Point {
  let best: Point | null = null;
  let bestPerp = thresholdFlow;
  for (const anchor of neighbors) {
    if (!anchor) continue;
    const dx = point.x - anchor.x;
    const dy = point.y - anchor.y;
    const dist = Math.hypot(dx, dy);
    if (dist < 1e-6) continue;
    const angle = Math.atan2(dy, dx);
    const step = Math.PI / 4; // 8 direções: 0°/45°/90°/135°/180°/225°/270°/315°
    const snapAngle = Math.round(angle / step) * step;
    const perp = Math.abs(dist * Math.sin(angle - snapAngle));
    if (perp < bestPerp) {
      bestPerp = perp;
      best = { x: anchor.x + dist * Math.cos(snapAngle), y: anchor.y + dist * Math.sin(snapAngle) };
    }
  }
  return best ?? point;
}

/** Ponto a t (0–1) ao longo do comprimento total do polyline — usado para posicionar o rótulo/
 * toolbar no "meio visual" do caminho real, não só a média dos pontos. */
function pointAtT(points: Point[], t: number): Point {
  if (points.length === 0) return { x: 0, y: 0 };
  const segLens: number[] = [];
  let total = 0;
  for (let i = 1; i < points.length; i++) {
    const len = Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y);
    segLens.push(len);
    total += len;
  }
  if (total === 0) return points[0];
  let target = total * t;
  for (let i = 0; i < segLens.length; i++) {
    if (target <= segLens[i] || i === segLens.length - 1) {
      const ratio = segLens[i] === 0 ? 0 : Math.min(1, target / segLens[i]);
      const a = points[i];
      const b = points[i + 1];
      return { x: a.x + (b.x - a.x) * ratio, y: a.y + (b.y - a.y) * ratio };
    }
    target -= segLens[i];
  }
  return points[points.length - 1];
}

/** Ligação de fibra/cabo de rede porta-a-porta. Seleccionada, mostra: tipo de cabo (fibra/rede),
 * cor (swatches da paleta do tipo + selector hexadecimal livre), traço (contínua/tracejada/
 * pontilhada) e nome. O caminho é a curva automática de sempre enquanto não houver nenhum ponto
 * intermédio; assim que o utilizador adiciona um (botão "+" no meio de um troço, visível quando
 * seleccionada), o caminho passa a ser desenhado em segmentos rectos por esses pontos — que podem
 * ser arrastados (mover) ou removidos (duplo-clique) — para o utilizador organizar o percurso do
 * cabo em vez de depender só da curva automática. */
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
  const reactFlow = useReactFlow();
  const [liveWaypoints, setLiveWaypoints] = useState<EdgeWaypoint[] | null>(null);
  const waypoints = liveWaypoints ?? data?.waypoints ?? [];
  const cableType: CableType = data?.cableType ?? "fiber";
  const dashStyle: EdgeDashStyle = data?.dashStyle ?? "solid";
  const color = data?.colorHex || "#64748b";
  const label = (data?.label ?? "").trim();
  const palette = cableType === "ethernet" ? NETWORK_CABLE_COLORS : STANDARD_FIBER_SEQUENCE;

  const points: Point[] = [{ x: sourceX, y: sourceY }, ...waypoints, { x: targetX, y: targetY }];
  const hasWaypoints = waypoints.length > 0;
  const [bezierPath, bezierLabelX, bezierLabelY] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition });
  const edgePath = hasWaypoints ? polylinePath(points) : bezierPath;
  const mid = hasWaypoints ? pointAtT(points, 0.5) : { x: bezierLabelX, y: bezierLabelY };
  const labelX = mid.x;
  const labelY = mid.y;
  // Card de edição fica sempre ABAIXO do ponto mais baixo do caminho inteiro (origem, destino e
  // todos os waypoints), não só do ponto médio — antes ficava a +36px do meio da curva e, com
  // caminhos mais largos/organizados manualmente, acabava em cima de um troço que o utilizador
  // queria clicar (reportado). Assim nunca sobrepõe a própria ligação.
  const toolbarY = Math.max(sourceY, targetY, ...waypoints.map((w) => w.y)) + 22;

  function patchData(patch: Partial<FiberEdgeData>) {
    data?.onPatch?.(id, patch);
  }
  function removeEdge() {
    data?.onRemove?.(id);
  }

  function startDragWaypoint(index: number, e: React.PointerEvent) {
    e.stopPropagation();
    e.preventDefault();
    let current = [...(data?.waypoints ?? [])];
    function onMove(ev: PointerEvent) {
      const raw = reactFlow.screenToFlowPosition({ x: ev.clientX, y: ev.clientY });
      // Vizinhos deste ponto na cadeia origem→waypoints→destino — o "auto ajuste" encosta o
      // ponto arrastado no ângulo (0/45/90/…) em relação a QUEM ele liga directamente, não a
      // todos os outros pontos do caminho (ver snapToNeighborAngles acima).
      const pts: Point[] = [{ x: sourceX, y: sourceY }, ...current, { x: targetX, y: targetY }];
      const threshold = 10 / reactFlow.getZoom();
      const pos = snapToNeighborAngles(raw, [pts[index], pts[index + 2]], threshold);
      current = current.map((p, i) => (i === index ? pos : p));
      setLiveWaypoints(current);
    }
    function onUp() {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      patchData({ waypoints: current });
      setLiveWaypoints(null);
    }
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }

  function addWaypointAtSegment(segmentIndex: number) {
    const a = points[segmentIndex];
    const b = points[segmentIndex + 1];
    const next = [...waypoints];
    next.splice(segmentIndex, 0, { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 });
    patchData({ waypoints: next });
  }

  function removeWaypoint(index: number) {
    patchData({ waypoints: waypoints.filter((_, i) => i !== index) });
  }

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        style={{ stroke: color, strokeWidth: selected ? 4 : 2.5, strokeDasharray: EDGE_DASH_PATTERNS[dashStyle] }}
      />
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
          <>
            {/* Pontos intermédios do caminho — arrastar move, duplo-clique remove. */}
            {waypoints.map((p, i) => (
              <div
                key={i}
                className="rack-edge-waypoint"
                title="Arrastar para mover · duplo-clique para remover"
                style={{ transform: `translate(-50%, -50%) translate(${p.x}px, ${p.y}px)` }}
                onPointerDown={(e) => startDragWaypoint(i, e)}
                onDoubleClick={(e) => {
                  e.stopPropagation();
                  removeWaypoint(i);
                }}
              />
            ))}
            {/* Botão "+" no meio de cada troço — adiciona um ponto ali para começar a organizar o
                caminho manualmente. */}
            {points.slice(0, -1).map((p, i) => {
              const next = points[i + 1];
              const mx = (p.x + next.x) / 2;
              const my = (p.y + next.y) / 2;
              return (
                <button
                  key={i}
                  type="button"
                  className="rack-edge-waypoint-add"
                  title="Adicionar ponto de organização do caminho"
                  style={{ transform: `translate(-50%, -50%) translate(${mx}px, ${my}px)` }}
                  onPointerDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    addWaypointAtSegment(i);
                  }}
                >
                  <Plus size={9} />
                </button>
              );
            })}
            <div
              className="topo-edge-toolbar"
              style={{ transform: `translate(-50%, 0) translate(${labelX}px, ${toolbarY}px)` }}
            >
              <div className="rack-edge-toolbar__row">
                {(Object.entries(CABLE_TYPE_LABELS) as Array<[CableType, string]>).map(([v, lbl]) => (
                  <button
                    key={v}
                    type="button"
                    className={`btn btn--sm${cableType === v ? " is-active" : ""}`}
                    style={{ fontSize: 10, padding: "2px 7px" }}
                    onClick={() => patchData({ cableType: v })}
                  >
                    {lbl}
                  </button>
                ))}
              </div>
              <div className="rack-edge-toolbar__swatches">
                {palette.map((f) => (
                  <button
                    key={f.name}
                    type="button"
                    title={f.name}
                    className={`rack-edge-toolbar__swatch${data?.colorName === f.name ? " rack-edge-toolbar__swatch--active" : ""}`}
                    style={{ background: f.hex }}
                    onClick={() => patchData({ colorName: f.name, colorHex: f.hex })}
                  />
                ))}
                <label className="rack-edge-toolbar__swatch rack-edge-toolbar__swatch--hex" title="Escolher outra cor…">
                  <input
                    type="color"
                    value={color}
                    onChange={(e) => patchData({ colorName: "Personalizada", colorHex: e.target.value })}
                  />
                </label>
              </div>
              <div className="rack-edge-toolbar__row">
                {(Object.entries(EDGE_DASH_LABELS) as Array<[EdgeDashStyle, string]>).map(([v, lbl]) => (
                  <button
                    key={v}
                    type="button"
                    className={`btn btn--sm${dashStyle === v ? " is-active" : ""}`}
                    style={{ fontSize: 10, padding: "2px 7px" }}
                    onClick={() => patchData({ dashStyle: v })}
                  >
                    {lbl}
                  </button>
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
                {hasWaypoints ? (
                  <button
                    type="button"
                    className="btn btn--icon"
                    title="Redefinir caminho (voltar à curva automática)"
                    onClick={() => patchData({ waypoints: [] })}
                  >
                    ↺
                  </button>
                ) : null}
                <button type="button" className="btn btn--icon" title="Remover ligação" onClick={removeEdge}>
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
          </>
        )}
      </EdgeLabelRenderer>
    </>
  );
}

export const FiberEdge = memo(FiberEdgeInner);
