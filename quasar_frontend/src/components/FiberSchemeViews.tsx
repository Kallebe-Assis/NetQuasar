import {
  destinationLabel,
  FIBER_DESTINATIONS,
  lightFiberBorder,
  lightFiberShadow,
  normalizeFiberDestination,
  SPLITTER_PORT_STATUSES,
  statusLabel,
  cableFiberDestinationLabel,
  cableFiberStatusLabel,
  type SplicePair,
  type SplitterPort,
  type SplitterPortStatus,
} from "../lib/fiberSplitter";
import { formatSplitterDisplay } from "../lib/networkInfrastructure";

type SelectOption = { value: string; label: string };

export function FiberPortsGrid({
  ports,
  canEdit,
  onChange,
  // Splitter/CTO (omitidos) usa Livre/Ocupada/Reserva técnica/Defeito + Disponível/Cliente/CTO;
  // CableFibersModal passa CABLE_FIBER_STATUSES/CABLE_FIBER_DESTINATIONS (vocabulário de cabo).
  statusOptions = SPLITTER_PORT_STATUSES,
  destinationOptions = FIBER_DESTINATIONS,
  normalizeDestinationValue = normalizeFiberDestination,
  // Só em CableFibersModal: destino só é editável quando o Estado é diferente de "livre" — ver
  // fiberSplitter.ts (normalizeCableFiberDestination) e o pedido do utilizador.
  destinationRequiresNonFreeStatus = false,
  // Campo livre "Cliente" por porta — só no splitter da CTO / foguete de distribuição, onde a
  // porta atende um assinante; o modal de fibras de cabo não passa isto.
  showClientName = false,
}: {
  ports: SplitterPort[];
  canEdit: boolean;
  onChange: (next: SplitterPort[]) => void;
  statusOptions?: readonly SelectOption[];
  destinationOptions?: readonly SelectOption[];
  normalizeDestinationValue?: (raw?: string | null) => string;
  destinationRequiresNonFreeStatus?: boolean;
  showClientName?: boolean;
}) {
  return (
    <div className="splitter-modal__grid">
      {ports.map((p, idx) => {
        const isFree = destinationRequiresNonFreeStatus && p.status === "livre";
        return (
          <article key={p.port} className={`splitter-port splitter-port--compact splitter-port--${p.status}`}>
            <header className="splitter-port__head">
              <span
                className="splitter-port__swatch splitter-port__swatch--sm"
                style={{
                  background: p.color_hex,
                  borderColor: lightFiberBorder(p.color) ? "rgba(0,0,0,.25)" : "transparent",
                  boxShadow: lightFiberShadow(p.color),
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
                    const status = e.target.value;
                    onChange(
                      ports.map((r, i) =>
                        i === idx
                          ? { ...r, status, destination: destinationRequiresNonFreeStatus && status === "livre" ? "" : r.destination }
                          : r,
                      ),
                    );
                  }}
                >
                  {statusOptions.map((s) => (
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
                  disabled={!canEdit || isFree}
                  value={isFree ? "" : normalizeDestinationValue(p.destination)}
                  title={isFree ? "Defina um Estado diferente de Livre para escolher o destino" : undefined}
                  onChange={(e) => {
                    const destination = e.target.value;
                    onChange(ports.map((r, i) => (i === idx ? { ...r, destination } : r)));
                  }}
                >
                  {isFree ? <option value="">—</option> : null}
                  {destinationOptions.map((d) => (
                    <option key={d.value} value={d.value}>
                      {d.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            {showClientName ? (
              <label className="splitter-modal__field">
                <span>Cliente</span>
                <input
                  className="input"
                  disabled={!canEdit}
                  placeholder="Nome do cliente nesta porta"
                  value={p.client_name ?? ""}
                  onChange={(e) => {
                    const client_name = e.target.value;
                    onChange(ports.map((r, i) => (i === idx ? { ...r, client_name } : r)));
                  }}
                />
              </label>
            ) : null}
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
        );
      })}
    </div>
  );
}

/** Card de metadados de uma fibra dentro do esquema 2D — 2 visões (ver o pedido do utilizador):
 * "detalhado" (a original: título, badge, linhas Estado/Destino, observação) e "simples"
 * (só a cor/estado como badge + destino, mais enxuto — cabe muito mais fibra por ecrã sem
 * amontoar). O chamador já resolve statusText/destinationText com o vocabulário certo
 * (splitter vs cabo) — este componente só formata, não decide rótulos. */
function FiberMetaCard({
  title,
  color,
  statusValue,
  statusText,
  destinationText,
  note,
  clientName,
  feedOnly = false,
  compact = false,
}: {
  title: string;
  color: string;
  statusValue?: string;
  statusText?: string;
  destinationText?: string;
  note?: string;
  /** Nome do cliente vinculado a esta porta (só splitter de CTO / foguete de distribuição). */
  clientName?: string;
  /** Fibra de alimentação: só título + cor (sem destino/obs). */
  feedOnly?: boolean;
  compact?: boolean;
}) {
  if (compact) {
    return (
      <div className={`splitter-scheme__meta splitter-scheme__meta--compact${feedOnly ? " splitter-scheme__meta--feed" : ""}`}>
        {statusValue ? (
          <span className={`splitter-scheme__badge splitter-scheme__badge--${statusValue}`}>{statusText}</span>
        ) : (
          <span className="splitter-scheme__v">{color}</span>
        )}
        {!feedOnly && destinationText ? <span className="splitter-scheme__v splitter-scheme__v--muted">{destinationText}</span> : null}
        {!feedOnly && clientName?.trim() ? <span className="splitter-scheme__v">{clientName.trim()}</span> : null}
      </div>
    );
  }
  return (
    <div className={`splitter-scheme__meta${feedOnly ? " splitter-scheme__meta--feed" : ""}`}>
      <div className="splitter-scheme__meta-top">
        <strong>{title}</strong>
        {statusValue ? <span className={`splitter-scheme__badge splitter-scheme__badge--${statusValue}`}>{statusText}</span> : null}
      </div>
      {statusValue ? (
        <div className="splitter-scheme__meta-row">
          <span className="splitter-scheme__k">Estado</span>
          <span className="splitter-scheme__v">{statusText}</span>
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
            <span className="splitter-scheme__v">{destinationText || "—"}</span>
          </div>
          {clientName?.trim() ? (
            <div className="splitter-scheme__meta-row">
              <span className="splitter-scheme__k">Cliente</span>
              <span className="splitter-scheme__v">{clientName.trim()}</span>
            </div>
          ) : null}
          {note?.trim() ? (
            <div className="splitter-scheme__meta-row">
              <span className="splitter-scheme__k">Observação</span>
              <span className="splitter-scheme__v">{note.trim()}</span>
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}

// Altura fixa por linha em cada modo — NÃO encolhe conforme a quantidade de fibras (era isso que
// causava fibras amontoando-se: com 24+ fibras a altura calculada ficava menor que o conteúdo do
// card, sobrepondo uma linha na outra). Contagens grandes agora só geram um esquema mais alto,
// que rola dentro do modal (.splitter-modal__body já tem overflow:auto) — é para isso que serve
// o modo "Simples", bem mais compacto por linha.
const DETAILED_ROW_H = 80;
const COMPACT_ROW_H = 44;

/** Esquema 2D do splitter: alimentação à esquerda + triângulo + saídas coloridas. */
export function SplitterScheme2D({
  ratio,
  ports,
  feedColor,
  feedHex,
  ctoName,
  compact = false,
}: {
  ratio: string;
  ports: SplitterPort[];
  feedColor: string;
  feedHex: string;
  ctoName: string;
  compact?: boolean;
}) {
  const n = Math.max(ports.length, 1);
  const rowH = compact ? COMPACT_ROW_H : DETAILED_ROW_H;
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
          <FiberMetaCard title="Fibra de Alimentação" color={feedColor} feedOnly compact={compact} />
          <span
            className="splitter-scheme__wire splitter-scheme__wire--feed"
            style={{
              background: feedHex,
              borderColor: lightFiberBorder(feedColor) ? "rgba(0,0,0,.28)" : "transparent",
              boxShadow: lightFiberShadow(feedColor),
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
                  boxShadow: lightFiberShadow(p.color),
                }}
                title={`${p.color} · porta ${p.port}`}
              />
              <FiberMetaCard
                title={`${p.port} · ${p.color}`}
                color={p.color}
                statusValue={p.status}
                statusText={statusLabel(p.status as SplitterPortStatus)}
                destinationText={destinationLabel(p.destination)}
                note={p.note}
                clientName={p.client_name}
                compact={compact}
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
  compact = false,
}: {
  ports: SplitterPort[];
  cableName: string;
  fiberCount: number;
  compact?: boolean;
}) {
  const n = Math.max(ports.length, 1);
  const rowH = compact ? COMPACT_ROW_H : DETAILED_ROW_H;
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
                  boxShadow: lightFiberShadow(p.color),
                }}
                title={`${p.color} · fibra ${p.port}`}
              />
              <FiberMetaCard
                title={`${p.port} · ${p.color}`}
                color={p.color}
                statusValue={p.status}
                statusText={cableFiberStatusLabel(p.status)}
                destinationText={cableFiberDestinationLabel(p.destination)}
                note={p.note}
                compact={compact}
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
  compact = false,
}: {
  pairs: SplicePair[];
  boxName: string;
  compact?: boolean;
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
                statusValue={p.status}
                statusText={statusLabel(p.status as SplitterPortStatus)}
                destinationText={p.destination?.trim() || undefined}
                note={p.note}
                compact={compact}
              />
              <span
                className="splitter-scheme__wire"
                style={{
                  background: p.left_color_hex,
                  borderColor: lightFiberBorder(p.left_color) ? "rgba(0,0,0,.28)" : "transparent",
                  boxShadow: lightFiberShadow(p.left_color),
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
                  boxShadow: lightFiberShadow(p.right_color),
                }}
              />
              <FiberMetaCard title={`${p.port} · ${p.right_color}`} color={p.right_color} feedOnly compact={compact} />
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
