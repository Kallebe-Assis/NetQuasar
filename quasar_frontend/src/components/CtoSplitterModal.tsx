import { createPortal } from "react-dom";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { InfoHint } from "./InfoHint";
import { FiberPortsGrid, SplitterScheme2D } from "./FiberSchemeViews";
import {
  buildDefaultSplitterPorts,
  fiberSpecByName,
  formatFeedFiberColor,
  parseSplitterOutputs,
  type SplitterPort,
} from "../lib/fiberSplitter";
import { FIBER_COLORS, formatSplitterDisplay, normalizeSplitterInput } from "../lib/networkInfrastructure";

type Props = {
  open: boolean;
  ctoId: string;
  ctoName: string;
  splitter: string | null | undefined;
  feedFiberColor: string | null | undefined;
  ports: SplitterPort[] | null | undefined;
  canEdit: boolean;
  onClose: () => void;
  onSaved?: () => void;
};

type TabId = "fibra" | "splitter";

export function CtoSplitterModal({ open, ctoId, ctoName, splitter, feedFiberColor, ports, canEdit, onClose, onSaved }: Props) {
  const qc = useQueryClient();
  const initialRatio = normalizeSplitterInput(splitter ?? "") ?? (splitter?.trim() || "1x8");
  const [ratio, setRatio] = useState(initialRatio);
  const [feedColor, setFeedColor] = useState(formatFeedFiberColor(feedFiberColor));
  const [draft, setDraft] = useState<SplitterPort[]>([]);
  const [tab, setTab] = useState<TabId>("fibra");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const r = normalizeSplitterInput(splitter ?? "") ?? (splitter?.trim() || "1x8");
    setRatio(r);
    setFeedColor(formatFeedFiberColor(feedFiberColor));
    setTab("fibra");
    const n = parseSplitterOutputs(r) ?? 8;
    setDraft(buildDefaultSplitterPorts(n, ports ?? null));
    setErr(null);
  }, [open, splitter, feedFiberColor, ports, ctoId]);

  const outputs = useMemo(() => parseSplitterOutputs(ratio) ?? draft.length, [ratio, draft.length]);
  const feedSpec = useMemo(() => fiberSpecByName(feedColor), [feedColor]);

  useEffect(() => {
    if (!open) return;
    const n = parseSplitterOutputs(ratio);
    if (n == null) return;
    setDraft((prev) => buildDefaultSplitterPorts(n, prev));
  }, [ratio, open]);

  const saveMut = useMutation({
    mutationFn: async () => {
      const normalized = normalizeSplitterInput(ratio);
      if (!normalized || parseSplitterOutputs(normalized) == null) {
        throw new Error("Informe um splitter válido (ex.: 1x8, 1x16, 1x32).");
      }
      const color = formatFeedFiberColor(feedColor);
      await apiFetch(`/api/v1/commercial/network/ctos/${ctoId}`, {
        method: "PATCH",
        json: {
          splitter: normalized,
          fiber_color: color,
          splitter_ports: draft.map(({ port, color: c, color_hex, label, status, note, destination }) => ({
            port,
            color: c,
            color_hex,
            label,
            status,
            note,
            destination,
          })),
        },
      });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["map-cto-detail", ctoId] });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      onSaved?.();
      onClose();
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar splitter."),
  });

  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop splitter-modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal splitter-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="splitter-modal-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="splitter-modal__head">
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="splitter-modal__eyebrow">CTO · splitter e fibras</div>
            <h2 id="splitter-modal-title" className="splitter-modal__title">
              {ctoName}
            </h2>
            <p className="splitter-modal__sub">
              Divisão <strong>{formatSplitterDisplay(ratio)}</strong> · {outputs} fibras de saída
              <InfoHint label="Legenda das cores de fibra">
                <p>
                  <strong>Verde</strong> = piloto
                  <br />
                  <strong>Amarelo</strong> = direcional
                  <br />
                  Demais cores seguem a sequência nacional (até 12, depois ciclam).
                </p>
              </InfoHint>
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="tabs" style={{ marginBottom: 4, flexShrink: 0 }}>
          <button type="button" className={tab === "fibra" ? "active" : undefined} onClick={() => setTab("fibra")}>
            Fibra
          </button>
          <button type="button" className={tab === "splitter" ? "active" : undefined} onClick={() => setTab("splitter")}>
            Splitter
          </button>
        </div>

        <div className="splitter-modal__body">
          {tab === "fibra" ? (
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
                      borderColor: feedSpec.name === "Branco" || feedSpec.name === "Amarelo" || feedSpec.name === "Desconhecido" ? "rgba(0,0,0,.25)" : "transparent",
                    }}
                  />
                  <label className="splitter-modal__field" style={{ flex: 1, minWidth: 0 }}>
                    <span>Fibra de Alimentação</span>
                    <select className="select" disabled={!canEdit} value={feedColor} onChange={(e) => setFeedColor(e.target.value)}>
                      {FIBER_COLORS.map((c) => (
                        <option key={c} value={c}>
                          {c}
                        </option>
                      ))}
                      {(FIBER_COLORS as readonly string[]).includes(feedColor) ? null : feedColor ? (
                        <option value={feedColor}>{feedColor}</option>
                      ) : null}
                    </select>
                  </label>
                </div>
              </div>

              <div className="splitter-modal__section-label">Fibras de saída ({outputs})</div>
              <FiberPortsGrid ports={draft} canEdit={canEdit} onChange={setDraft} />
            </>
          ) : (
            <SplitterScheme2D
              ratio={ratio}
              ports={draft}
              feedColor={feedSpec.name}
              feedHex={feedSpec.hex}
              ctoName={ctoName}
            />
          )}

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
