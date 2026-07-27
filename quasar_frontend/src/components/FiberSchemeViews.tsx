import {
  destinationLabel,
  FIBER_DESTINATIONS,
  lightFiberBorder,
  normalizeFiberDestination,
  SPLITTER_PORT_STATUSES,
  statusLabel,
  type FiberDestination,
  type SplicePair,
  type SplitterPort,
  type SplitterPortStatus,
} from "../lib/fiberSplitter";
import { formatSplitterDisplay } from "../lib/networkInfrastructure";

export function FiberPortsGrid({
  ports,
  canEdit,
  onChange,
}: {
  ports: SplitterPort[];
  canEdit: boolean;
  onChange: (next: SplitterPort[]) => void;
}) {
  return (
    <div className="splitter-modal__grid">
      {ports.map((p, idx) => (
        <article key={p.port} className={`splitter-port splitter-port--compact splitter-port--${p.status}`}>
          <header className="splitter-port__head">
            <span
              className="splitter-port__swatch splitter-port__swatch--sm"
              style={{
                background: p.color_hex,
                borderColor: lightFiberBorder(p.color) ? "rgba(0,0,0,.25)" : "transparent",
              }}
              title={p.hint}
            />
            <strong>
              Fibra {p.port} · {p.color}
            </strong>
          </header>
          <div className="splitter-port__row2">
            <label className="splitter-modal__field">
              <span>Estado</span>
              <select
                className="select"
                disabled={!canEdit}
                value={p.status}
                onChange={(e) => {
                  const status = e.target.value as SplitterPortStatus;
                  onChange(ports.map((r, i) => (i === idx ? { ...r, status } : r)));
                }}
              >
                {SPLITTER_PORT_STATUSES.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="splitter-modal__field">
              <span>Destino</span>
              <select
                className="select"
                disabled={!canEdit}
                value={normalizeFiberDestination(p.destination)}
                onChange={(e) => {
                  const destination = e.target.value as FiberDestination;
                  onChange(ports.map((r, i) => (i === idx ? { ...r, destination } : r)));
                }}
              >
                {FIBER_DESTINATIONS.map((d) => (
                  <option key={d.value} value={d.value}>
                    {d.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label className="splitter-modal__field">
            <span>Observação</span>
            <input
              className="input"
              disabled={!canEdit}
              value={p.note}
              onChange={(e) => {
                const note = e.target.value;
                onChange(ports.map((r, i) => (i === idx ? { ...r, note } : r)));
              }}
            />
          </label>
        </article>
      ))}
    </div>
  );
}

function FiberMetaCard({
  title,
  color,
  status,
  destination,
  note,
  feedOnly = false,
}: {
  title: string;
  color: string;
  status?: string;
  destination?: string;
  note?: string;
  /** Fibra de alimentação: só título + cor (sem destino/obs). */
  feedOnly?: boolean;
}) {
  return (
    <div className={`splitter-scheme__meta${feedOnly ? " splitter-scheme__meta--feed" : ""}`}>
      <div className="splitter-scheme__meta-top">
        <strong>{title}</strong>
        {status ? <span className={`splitter-scheme__badge splitter-scheme__badge--${status}`}>{statusLabel(status as SplitterPortStatus)}</span> : null}
      </div>
      {status ? (
        <div className="splitter-scheme__meta-row">
          <span className="splitter-scheme__k">Estado</span>
          <span className="splitter-scheme__v">{statusLabel(status as SplitterPortStatus)}</span>
        </div>
      ) : (
        <div className="splitter-scheme__meta-row">
          <span className="splitter-scheme__k">Cor</span>
          <span className="splitter-scheme__v">{color}</span>
        </div>
      )}
      {feedOnly ? null : (
        <>
          <div className="splitter-scheme__meta-row">
            <span className="splitter-scheme__k">Destino</span>
            <span className="splitter-scheme__v">{destinationLabel(destination)}</span>
          </div>
          <div className="splitter-scheme__meta-row">
            <span className="splitter-scheme__k">Observação</span>
            <span className="splitter-scheme__v">{note?.trim() || "—"}</span>
          </div>
        </>
      )}
    </div>
  );
}

/** Esquema 2D do splitter: alimentação à esquerda + triângulo + saídas coloridas. */
export function SplitterScheme2D({
  ratio,
  ports,
  feedColor,
  feedHex,
  ctoName,
}: {
  ratio: string;
  ports: SplitterPort[];
  feedColor: string;
  feedHex: string;
  ctoName: string;
}) {
  const n = Math.max(ports.length, 1);
  const rowH = ports.length <= 8 ? 48 : Math.min(48, Math.max(34, Math.floor(480 / n)));
  const h = n * rowH;
  const w = 148;
  const cx = 22;
  const cy = h / 2;
  const r = 13;
  const boxX = 118;
  const boxSize = 22;
  const topY = rowH / 2;
  const bottomY = h - rowH / 2;
  const feedTextFill =
    feedColor === "Preto" || feedColor === "Azul" || feedColor === "Verde" || feedColor === "Violeta" || feedColor === "Marrom"
      ? "#fff"
      : "#111";

  return (
    <div className="splitter-scheme" aria-label={`Diagrama 2D do splitter ${ratio}`}>
      <div className="splitter-scheme__body splitter-scheme__body--with-feed" style={{ ["--scheme-row-h" as string]: `${rowH}px` }}>
        <div className="splitter-scheme__feed">
          <FiberMetaCard title="Fibra de Alimentação" color={feedColor} feedOnly />
          <span
            className="splitter-scheme__wire splitter-scheme__wire--feed"
            style={{
              background: feedHex,
              borderColor: lightFiberBorder(feedColor) ? "rgba(0,0,0,.28)" : "transparent",
            }}
            title={`Alimentação · ${feedColor}`}
          />
        </div>

        <div className="splitter-scheme__wedge" style={{ height: h }}>
          <svg viewBox={`0 0 ${w} ${h}`} width={w} height={h} aria-hidden>
            <line x1={cx + r * 0.55} y1={cy - r * 0.65} x2={boxX} y2={topY} stroke="currentColor" strokeWidth={2.2} />
            <line x1={cx + r * 0.55} y1={cy + r * 0.65} x2={boxX} y2={bottomY} stroke="currentColor" strokeWidth={2.2} />
            <line x1={boxX} y1={topY} x2={boxX} y2={bottomY} stroke="currentColor" strokeWidth={1.6} />
            {ports.map((p, i) => {
              const y = i * rowH + rowH / 2;
              return (
                <g key={p.port}>
                  <rect x={boxX - boxSize / 2} y={y - boxSize / 2} width={boxSize} height={boxSize} rx={2} fill="#111" />
                  <text
                    x={boxX}
                    y={y + 1}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="#fff"
                    fontSize={11}
                    fontWeight={700}
                    fontFamily="ui-sans-serif, system-ui, sans-serif"
                  >
                    {p.port}
                  </text>
                </g>
              );
            })}
            <circle
              cx={cx}
              cy={cy}
              r={r}
              fill={feedHex}
              stroke={lightFiberBorder(feedColor) ? "rgba(0,0,0,.45)" : "#111"}
              strokeWidth={2}
            />
            <text
              x={cx}
              y={cy + 1}
              textAnchor="middle"
              dominantBaseline="middle"
              fill={feedTextFill}
              fontSize={11}
              fontWeight={700}
              fontFamily="ui-sans-serif, system-ui, sans-serif"
            >
              1
            </text>
          </svg>
        </div>

        <div className="splitter-scheme__fibers">
          {ports.map((p) => (
            <div key={p.port} className={`splitter-scheme__fiber splitter-scheme__fiber--${p.status}`}>
              <span
                className="splitter-scheme__wire"
                style={{
                  background: p.color_hex,
                  borderColor: lightFiberBorder(p.color) ? "rgba(0,0,0,.28)" : "transparent",
                }}
                title={`${p.color} · porta ${p.port}`}
              />
              <FiberMetaCard
                title={`${p.port} · ${p.color}`}
                color={p.color}
                status={p.status}
                destination={p.destination}
                note={p.note}
              />
            </div>
          ))}
        </div>
      </div>

      <div className="splitter-scheme__caption">
        <strong>{ctoName || "Splitter"}</strong>
        <span>({formatSplitterDisplay(ratio)})</span>
      </div>
    </div>
  );
}

/** Esquema 2D de cabo: pilha de fibras com fios coloridos (sem splitter). */
export function CableFibersScheme2D({
  ports,
  cableName,
  fiberCount,
}: {
  ports: SplitterPort[];
  cableName: string;
  fiberCount: number;
}) {
  const n = Math.max(ports.length, 1);
  const rowH = Math.min(48, Math.max(36, Math.floor(520 / n)));
  const h = n * rowH;
  const boxSize = Math.min(22, rowH - 10);
  const boxX = 28;

  return (
    <div className="splitter-scheme" aria-label={`Diagrama 2D do cabo ${fiberCount} fibras`}>
      <div className="splitter-scheme__body splitter-scheme__body--cable" style={{ ["--scheme-row-h" as string]: `${rowH}px` }}>
        <div className="splitter-scheme__wedge splitter-scheme__wedge--cable" style={{ height: h }}>
          <svg viewBox={`0 0 56 ${h}`} width={56} height={h} aria-hidden>
            <line x1={boxX} y1={rowH / 2} x2={boxX} y2={h - rowH / 2} stroke="currentColor" strokeWidth={1.6} />
            {ports.map((p, i) => {
              const y = i * rowH + rowH / 2;
              return (
                <g key={p.port}>
                  <rect x={boxX - boxSize / 2} y={y - boxSize / 2} width={boxSize} height={boxSize} rx={2} fill="#111" />
                  <text
                    x={boxX}
                    y={y + 1}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="#fff"
                    fontSize={boxSize > 18 ? 11 : 9}
                    fontWeight={700}
                    fontFamily="ui-sans-serif, system-ui, sans-serif"
                  >
                    {p.port}
                  </text>
                </g>
              );
            })}
          </svg>
        </div>

        <div className="splitter-scheme__fibers">
          {ports.map((p) => (
            <div key={p.port} className={`splitter-scheme__fiber splitter-scheme__fiber--${p.status}`}>
              <span
                className="splitter-scheme__wire"
                style={{
                  background: p.color_hex,
                  borderColor: lightFiberBorder(p.color) ? "rgba(0,0,0,.28)" : "transparent",
                }}
                title={`${p.color} · fibra ${p.port}`}
              />
              <FiberMetaCard
                title={`${p.port} · ${p.color}`}
                color={p.color}
                status={p.status}
                destination={p.destination}
                note={p.note}
              />
            </div>
          ))}
        </div>
      </div>

      <div className="splitter-scheme__caption">
        <strong>{cableName || "Cabo"}</strong>
        <span>({fiberCount} fibras)</span>
      </div>
    </div>
  );
}

/** Esquema 2D de emenda: fibras à esquerda, fusão no centro, fibras à direita. */
export function SpliceEmendaScheme2D({
  pairs,
  boxName,
}: {
  pairs: SplicePair[];
  boxName: string;
}) {
  return (
    <div className="splitter-scheme" aria-label="Diagrama 2D de emenda">
      <div className="splice-emenda">
        {pairs.map((p) => (
          <div key={p.port} className={`splice-emenda__row splice-emenda__row--${p.status}`}>
            <div className="splice-emenda__side splice-emenda__side--left">
              <FiberMetaCard
                title={`${p.port} · ${p.left_color}`}
                color={p.left_color}
                status={p.status}
                destination={p.destination}
                note={p.note}
              />
              <span
                className="splitter-scheme__wire"
                style={{
                  background: p.left_color_hex,
                  borderColor: lightFiberBorder(p.left_color) ? "rgba(0,0,0,.28)" : "transparent",
                }}
              />
            </div>
            <div className="splice-emenda__joint" title={`Emenda ${p.port}`}>
              <span className="splice-emenda__port">{p.port}</span>
              <span className="splice-emenda__fuse" />
            </div>
            <div className="splice-emenda__side splice-emenda__side--right">
              <span
                className="splitter-scheme__wire"
                style={{
                  background: p.right_color_hex,
                  borderColor: lightFiberBorder(p.right_color) ? "rgba(0,0,0,.28)" : "transparent",
                }}
              />
              <FiberMetaCard title={`${p.port} · ${p.right_color}`} color={p.right_color} feedOnly />
            </div>
          </div>
        ))}
      </div>
      <div className="splitter-scheme__caption">
        <strong>{boxName || "Caixa de emenda"}</strong>
        <span>({pairs.length} emendas)</span>
      </div>
    </div>
  );
}
