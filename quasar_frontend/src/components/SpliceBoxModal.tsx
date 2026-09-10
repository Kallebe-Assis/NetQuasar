import { createPortal } from "react-dom";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { InfoHint } from "./InfoHint";
import { FiberPortsGrid, SpliceEmendaScheme2D, SplitterScheme2D } from "./FiberSchemeViews";
import {
  buildDefaultSplicePairs,
  buildDefaultSplitterPorts,
  CABLE_FIBER_COUNTS,
  fiberSpecByName,
  formatFeedFiberColor,
  isCableFiberCount,
  lightFiberBorder,
  lightFiberShadow,
  parseSplitterOutputs,
  type SpliceBoxModel,
  type SplicePair,
  type SplitterPort,
} from "../lib/fiberSplitter";
import { FIBER_COLORS, formatSplitterDisplay, normalizeSplitterInput } from "../lib/networkInfrastructure";

type Props = {
  open: boolean;
  spliceId: string;
  spliceName: string;
  boxModel?: string | null;
  fiberCount?: number | null;
  splitter?: string | null;
  feedFiberColor?: string | null;
  ports?: SplitterPort[] | null;
  pairs?: SplicePair[] | null;
  canEdit: boolean;
  onClose: () => void;
  onSaved?: () => void;
};

type TabId = "fibra" | "esquema";

function colorOptions(current: string): string[] {
  const base: string[] = [...FIBER_COLORS];
  if (current && !base.includes(current)) base.push(current);
  return base;
}

export function SpliceBoxModal({
  open,
  spliceId,
  spliceName,
  boxModel,
  fiberCount,
  splitter,
  feedFiberColor,
  ports,
  pairs,
  canEdit,
  onClose,
  onSaved,
}: Props) {
  const qc = useQueryClient();
  const [model, setModel] = useState<SpliceBoxModel>(boxModel === "distribuicao" ? "distribuicao" : "emenda");
  const [count, setCount] = useState(fiberCount && fiberCount > 0 ? fiberCount : 12);
  const [ratio, setRatio] = useState(normalizeSplitterInput(splitter ?? "") ?? (splitter?.trim() || "1x8"));
  const [feedColor, setFeedColor] = useState(formatFeedFiberColor(feedFiberColor));
  const [draftPorts, setDraftPorts] = useState<SplitterPort[]>([]);
  const [draftPairs, setDraftPairs] = useState<SplicePair[]>([]);
  const [tab, setTab] = useState<TabId>("fibra");
  const [viewMode, setViewMode] = useState<"detalhado" | "simples">("detalhado");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const m: SpliceBoxModel = boxModel === "distribuicao" ? "distribuicao" : "emenda";
    setModel(m);
    setTab("fibra");
    setFeedColor(formatFeedFiberColor(feedFiberColor));
    const r = normalizeSplitterInput(splitter ?? "") ?? (splitter?.trim() || "1x8");
    setRatio(r);
    const nPorts = parseSplitterOutputs(r) ?? 8;
    setDraftPorts(buildDefaultSplitterPorts(nPorts, ports ?? null));
    const nPairs = fiberCount && fiberCount > 0 ? fiberCount : 12;
    setCount(nPairs);
    setDraftPairs(buildDefaultSplicePairs(nPairs, pairs ?? null));
    setErr(null);
  }, [open, boxModel, fiberCount, splitter, feedFiberColor, ports, pairs, spliceId]);

  useEffect(() => {
    if (!open || model !== "distribuicao") return;
    const n = parseSplitterOutputs(ratio);
    if (n == null) return;
    setDraftPorts((prev) => buildDefaultSplitterPorts(n, prev));
  }, [ratio, open, model]);

  useEffect(() => {
    if (!open || model !== "emenda") return;
    setDraftPairs((prev) => buildDefaultSplicePairs(count, prev));
  }, [count, open, model]);

  const outputs = useMemo(() => parseSplitterOutputs(ratio) ?? draftPorts.length, [ratio, draftPorts.length]);
  const feedSpec = useMemo(() => fiberSpecByName(feedColor), [feedColor]);
  const countOptions = useMemo(() => {
    if (isCableFiberCount(count)) return [...CABLE_FIBER_COUNTS];
    return [...CABLE_FIBER_COUNTS, count].sort((a, b) => a - b);
  }, [count]);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (model === "emenda") {
        if (count < 1 || count > 144) throw new Error("Quantidade de fibras inválida.");
        await apiFetch(`/api/v1/commercial/network/splice-boxes/${spliceId}`, {
          method: "PATCH",
          json: {
            box_model: "emenda",
            fiber_count: count,
            splice_pairs: draftPairs.map((p) => ({
              port: p.port,
              left_color: p.left_color,
              left_color_hex: p.left_color_hex,
              right_color: p.right_color,
              right_color_hex: p.right_color_hex,
              status: p.status,
              note: p.note,
              destination: p.destination,
            })),
          },
        });
        return;
      }
      const normalized = normalizeSplitterInput(ratio);
      if (!normalized || parseSplitterOutputs(normalized) == null) {
        throw new Error("Informe um splitter válido (ex.: 1x8, 1x16).");
      }
      await apiFetch(`/api/v1/commercial/network/splice-boxes/${spliceId}`, {
        method: "PATCH",
        json: {
          box_model: "distribuicao",
          splitter: normalized,
          fiber_color: formatFeedFiberColor(feedColor),
          fiber_count: parseSplitterOutputs(normalized),
          splitter_ports: draftPorts.map(({ port, color: c, color_hex, label, status, note, destination, client_name }) => ({
            port,
            color: c,
            color_hex,
            label,
            status,
            note,
            destination,
            client_name: client_name?.trim() || "",
          })),
        },
      });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["map-splice-detail", spliceId] });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      await qc.invalidateQueries({ queryKey: ["connections-infra"] });
      onSaved?.();
      onClose();
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar caixa de emenda."),
  });

  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop splitter-modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal splitter-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="splice-box-modal-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="splitter-modal__head">
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="splitter-modal__eyebrow">Caixa de emenda · foguete</div>
            <h2 id="splice-box-modal-title" className="splitter-modal__title">
              {spliceName}
            </h2>
            <p className="splitter-modal__sub">
              Modelo <strong>{model === "emenda" ? "Emenda" : "Distribuição"}</strong>
              {model === "distribuicao" ? (
                <>
                  {" "}
                  · {formatSplitterDisplay(ratio)} · {outputs} saídas
                </>
              ) : (
                <> · {count} pares de fusão</>
              )}
              <InfoHint label="Modelos da caixa">
                <p>
                  <strong>Emenda</strong>: fibras de um lado fundidas com as do outro.
                  <br />
                  <strong>Distribuição</strong>: splitter interno, como numa CTO.
                </p>
              </InfoHint>
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="splitter-modal__tabsrow">
          <div className="tabs" style={{ marginBottom: 0 }}>
            <button type="button" className={tab === "fibra" ? "active" : undefined} onClick={() => setTab("fibra")}>
              Fibra
            </button>
            <button type="button" className={tab === "esquema" ? "active" : undefined} onClick={() => setTab("esquema")}>
              {model === "distribuicao" ? "Splitter" : "Esquema"}
            </button>
          </div>
          <label className="splitter-modal__field splitter-modal__field--inline">
            <span>Modelo da caixa</span>
            <select
              className="select"
              disabled={!canEdit}
              value={model}
              onChange={(e) => setModel(e.target.value as SpliceBoxModel)}
            >
              <option value="emenda">Emenda</option>
              <option value="distribuicao">Distribuição</option>
            </select>
          </label>
        </div>

        <div className="splitter-modal__body">
        {tab === "fibra" && model === "distribuicao" ? (
          <>
            <div className="splitter-modal__toolbar splitter-modal__toolbar--pair">
              <label className="splitter-modal__field">
                <span>Tipo de splitter</span>
                <select className="select" value={ratio} disabled={!canEdit} onChange={(e) => setRatio(e.target.value)}>
                  {["1x2", "1x4", "1x8", "1x16", "1x32", "1x64"].map((opt) => (
                    <option key={opt} value={opt}>
                      {opt}
                    </option>
                  ))}
                </select>
              </label>
              <div className="splitter-modal__feed-inline">
                <span
                  className="splitter-port__swatch"
                  style={{
                    background: feedSpec.hex,
                    borderColor:
                      feedSpec.name === "Branco" || feedSpec.name === "Amarelo" || feedSpec.name === "Desconhecido"
                        ? "rgba(0,0,0,.25)"
                        : "transparent",
                    boxShadow: lightFiberShadow(feedSpec.name),
                  }}
                />
                <label className="splitter-modal__field" style={{ flex: 1, minWidth: 0 }}>
                  <span>Fibra de Alimentação</span>
                  <select className="select" disabled={!canEdit} value={feedColor} onChange={(e) => setFeedColor(e.target.value)}>
                    {colorOptions(feedColor).map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </div>
            <div className="splitter-modal__section-label">Fibras de saída ({outputs})</div>
            <FiberPortsGrid ports={draftPorts} canEdit={canEdit} onChange={setDraftPorts} showClientName />
          </>
        ) : null}

        {tab === "fibra" && model === "emenda" ? (
          <>
            <div className="splitter-modal__toolbar">
              <label className="splitter-modal__field" style={{ maxWidth: 220 }}>
                <span>Quantidade de fibras</span>
                <select className="select" value={String(count)} disabled={!canEdit} onChange={(e) => setCount(Number(e.target.value))}>
                  {countOptions.map((n) => (
                    <option key={n} value={n}>
                      {n} fibras
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div className="splitter-modal__section-label">Pares de emenda ({count})</div>
            <div className="splitter-modal__grid">
              {draftPairs.map((p, idx) => (
                <article key={p.port} className={`splitter-port splitter-port--${p.status}`}>
                  <header className="splitter-port__head">
                    <strong>Emenda {p.port}</strong>
                  </header>
                  <div className="splice-pair-colors">
                    <label className="splitter-modal__field">
                      <span>Fibra esquerda</span>
                      <div className="splitter-modal__feed-inline">
                        <span
                          className="splitter-port__swatch splitter-port__swatch--sm"
                          style={{
                            background: p.left_color_hex,
                            borderColor: lightFiberBorder(p.left_color) ? "rgba(0,0,0,.25)" : "transparent",
                            boxShadow: lightFiberShadow(p.left_color),
                          }}
                          title={p.left_color}
                        />
                        <select
                          className="select"
                          disabled={!canEdit}
                          value={p.left_color}
                          onChange={(e) => {
                            // Antes caía na cor do fio "de série" da porta (fiberSpecForPort) sempre
                            // que o nome resolvido era "Desconhecido" — incluindo quando o
                            // utilizador escolhe "Desconhecido" de propósito no <select>, mostrando
                            // p.ex. verde na fibra 1 em vez de nenhuma cor. fiberSpecByName já
                            // devolve o cinza certo para esse caso — usar sempre o resultado dela.
                            const spec = fiberSpecByName(e.target.value);
                            setDraftPairs((rows) =>
                              rows.map((r, i) => (i === idx ? { ...r, left_color: spec.name, left_color_hex: spec.hex } : r)),
                            );
                          }}
                        >
                          {colorOptions(p.left_color).map((c) => (
                            <option key={c} value={c}>
                              {c}
                            </option>
                          ))}
                        </select>
                      </div>
                    </label>
                    <label className="splitter-modal__field">
                      <span>Fibra direita</span>
                      <div className="splitter-modal__feed-inline">
                        <span
                          className="splitter-port__swatch splitter-port__swatch--sm"
                          style={{
                            background: p.right_color_hex,
                            borderColor: lightFiberBorder(p.right_color) ? "rgba(0,0,0,.25)" : "transparent",
                            boxShadow: lightFiberShadow(p.right_color),
                          }}
                          title={p.right_color}
                        />
                        <select
                          className="select"
                          disabled={!canEdit}
                          value={p.right_color}
                          onChange={(e) => {
                            const spec = fiberSpecByName(e.target.value);
                            setDraftPairs((rows) =>
                              rows.map((r, i) => (i === idx ? { ...r, right_color: spec.name, right_color_hex: spec.hex } : r)),
                            );
                          }}
                        >
                          {colorOptions(p.right_color).map((c) => (
                            <option key={c} value={c}>
                              {c}
                            </option>
                          ))}
                        </select>
                      </div>
                    </label>
                  </div>
                  <label className="splitter-modal__field">
                    <span>Destino</span>
                    <input
                      className="input"
                      disabled={!canEdit}
                      value={p.destination}
                      placeholder="Opcional"
                      onChange={(e) => {
                        const destination = e.target.value;
                        setDraftPairs((rows) => rows.map((r, i) => (i === idx ? { ...r, destination } : r)));
                      }}
                    />
                  </label>
                  <label className="splitter-modal__field">
                    <span>Observação</span>
                    <input
                      className="input"
                      disabled={!canEdit}
                      value={p.note}
                      onChange={(e) => {
                        const note = e.target.value;
                        setDraftPairs((rows) => rows.map((r, i) => (i === idx ? { ...r, note } : r)));
                      }}
                    />
                  </label>
                </article>
              ))}
            </div>
          </>
        ) : null}

        {tab === "esquema" ? (
          <div className="splitter-scheme__viewmode">
            <button
              type="button"
              className={`btn btn--sm${viewMode === "detalhado" ? " is-active" : ""}`}
              onClick={() => setViewMode("detalhado")}
            >
              Detalhado
            </button>
            <button
              type="button"
              className={`btn btn--sm${viewMode === "simples" ? " is-active" : ""}`}
              onClick={() => setViewMode("simples")}
            >
              Simples
            </button>
          </div>
        ) : null}

        {tab === "esquema" && model === "distribuicao" ? (
          <SplitterScheme2D
            ratio={ratio}
            ports={draftPorts}
            feedColor={feedSpec.name}
            feedHex={feedSpec.hex}
            ctoName={spliceName}
            compact={viewMode === "simples"}
          />
        ) : null}

        {tab === "esquema" && model === "emenda" ? (
          <SpliceEmendaScheme2D pairs={draftPairs} boxName={spliceName} compact={viewMode === "simples"} />
        ) : null}

        {err ? <div className="msg msg--err">{err}</div> : null}
        </div>

        <div className="splitter-modal__foot">
          <button type="button" className="btn" onClick={onClose} disabled={saveMut.isPending}>
            Fechar
          </button>
          {canEdit ? (
            <button type="button" className="btn btn--primary" disabled={saveMut.isPending} onClick={() => saveMut.mutate()}>
              {saveMut.isPending ? "A guardar…" : "Guardar"}
            </button>
          ) : null}
        </div>
      </div>
    </div>,
    document.body,
  );
}
