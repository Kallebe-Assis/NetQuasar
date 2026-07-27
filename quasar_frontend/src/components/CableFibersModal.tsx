import { createPortal } from "react-dom";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { InfoHint } from "./InfoHint";
import { CableFibersScheme2D, FiberPortsGrid } from "./FiberSchemeViews";
import {
  buildDefaultSplitterPorts,
  CABLE_FIBER_COUNTS,
  isCableFiberCount,
  type SplitterPort,
} from "../lib/fiberSplitter";

type Props = {
  open: boolean;
  cableId: string;
  cableName: string;
  fiberCount: number | null | undefined;
  ports: SplitterPort[] | null | undefined;
  canEdit: boolean;
  onClose: () => void;
  onSaved?: () => void;
};

type TabId = "fibra" | "esquema";

export function CableFibersModal({ open, cableId, cableName, fiberCount, ports, canEdit, onClose, onSaved }: Props) {
  const qc = useQueryClient();
  const initialCount = fiberCount && fiberCount > 0 ? fiberCount : 12;
  const [count, setCount] = useState(initialCount);
  const [draft, setDraft] = useState<SplitterPort[]>([]);
  const [tab, setTab] = useState<TabId>("fibra");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const n = fiberCount && fiberCount > 0 ? fiberCount : 12;
    setCount(n);
    setTab("fibra");
    setDraft(buildDefaultSplitterPorts(n, ports ?? null));
    setErr(null);
  }, [open, fiberCount, ports, cableId]);

  useEffect(() => {
    if (!open) return;
    setDraft((prev) => buildDefaultSplitterPorts(count, prev));
  }, [count, open]);

  const countOptions = useMemo(() => {
    if (isCableFiberCount(count)) return [...CABLE_FIBER_COUNTS];
    return [...CABLE_FIBER_COUNTS, count].sort((a, b) => a - b);
  }, [count]);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (count < 1 || count > 144) {
        throw new Error("Quantidade de fibras inválida.");
      }
      await apiFetch(`/api/v1/commercial/network/cables/${cableId}`, {
        method: "PATCH",
        json: {
          fiber_count: count,
          fiber_ports: draft.map(({ port, color: c, color_hex, label, status, note, destination }) => ({
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
      await qc.invalidateQueries({ queryKey: ["map-cable-detail", cableId] });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      await qc.invalidateQueries({ queryKey: ["connections-infra"] });
      onSaved?.();
      onClose();
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar fibras do cabo."),
  });

  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop splitter-modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal splitter-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="cable-fibers-modal-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="splitter-modal__head">
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="splitter-modal__eyebrow">Cabo · fibras</div>
            <h2 id="cable-fibers-modal-title" className="splitter-modal__title">
              {cableName}
            </h2>
            <p className="splitter-modal__sub">
              {count} fibras · sequência nacional de cores
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
          <button type="button" className={tab === "esquema" ? "active" : undefined} onClick={() => setTab("esquema")}>
            Esquema
          </button>
        </div>

        <div className="splitter-modal__body">
          {tab === "fibra" ? (
            <>
              <div className="splitter-modal__toolbar">
                <label className="splitter-modal__field" style={{ maxWidth: 220 }}>
                  <span>Quantidade de fibras</span>
                  <select
                    className="select"
                    value={String(count)}
                    disabled={!canEdit}
                    onChange={(e) => setCount(Number(e.target.value))}
                  >
                    {countOptions.map((n) => (
                      <option key={n} value={n}>
                        {n} fibras
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <div className="splitter-modal__section-label">Fibras ({count})</div>
              <FiberPortsGrid ports={draft} canEdit={canEdit} onChange={setDraft} />
            </>
          ) : (
            <CableFibersScheme2D ports={draft} cableName={cableName} fiberCount={count} />
          )}

          {err ? <div className="msg msg--err">{err}</div> : null}
        </div>

        <div className="splitter-modal__foot">
          <button type="button" className="btn" onClick={onClose} disabled={saveMut.isPending}>
            Fechar
          </button>
          {canEdit ? (
            <button type="button" className="btn btn--primary" disabled={saveMut.isPending} onClick={() => saveMut.mutate()}>
              {saveMut.isPending ? "A guardar…" : "Guardar fibras"}
            </button>
          ) : null}
        </div>
      </div>
    </div>,
    document.body,
  );
}
