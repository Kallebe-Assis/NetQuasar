import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { CABLE_FIBER_COUNTS } from "../../lib/fiberSplitter";
import { CABLE_STATUSES, FIBER_COLORS, fmtCoord, normalizeSplitterInput } from "../../lib/networkInfrastructure";
import {
  KML_KIND_LABELS,
  kmlReviewSummary,
  type KmlElementKind,
  type KmlReviewItem,
} from "../../lib/parseKmlProject";

type Props = {
  open: boolean;
  fileName?: string | null;
  projectName: string;
  skipped: number;
  initialItems: KmlReviewItem[];
  onCancel: () => void;
  onConfirm: (items: KmlReviewItem[]) => void;
};

const KIND_TABS: KmlElementKind[] = ["cto", "splice_box", "pole", "cable"];

function changeKind(item: KmlReviewItem, next: KmlElementKind): KmlReviewItem | null {
  if (item.kind === next) return item;
  // Cabo → ponto: usa o 1.º ponto do trajeto
  if (item.kind === "cable" && next !== "cable") {
    const lat = item.path?.[0]?.lat ?? item.latitude;
    const lng = item.path?.[0]?.lng ?? item.longitude;
    return {
      ...item,
      kind: next,
      path: null,
      latitude: lat,
      longitude: lng,
      splitter: next === "cto" ? item.splitter || "1x8" : next === "splice_box" ? item.splitter : "",
      fiber_count: next === "splice_box" ? item.fiber_count || "12" : "",
      box_model: next === "splice_box" ? item.box_model || "emenda" : item.box_model,
    };
  }
  // Ponto → cabo: só se já tiver path (improvável); senão cria stub de 2 pontos
  if (item.kind !== "cable" && next === "cable") {
    const path =
      item.path && item.path.length >= 2
        ? item.path
        : [
            { lat: item.latitude, lng: item.longitude },
            { lat: item.latitude + 0.00005, lng: item.longitude + 0.00005 },
          ];
    return {
      ...item,
      kind: "cable",
      path,
      latitude: path[0].lat,
      longitude: path[0].lng,
      fiber_count: item.fiber_count || "12",
      cable_status: item.cable_status || "planejado",
    };
  }
  // Entre tipos pontuais
  return {
    ...item,
    kind: next,
    splitter: next === "cto" ? item.splitter || "1x8" : next === "splice_box" ? item.splitter : "",
    fiber_count: next === "splice_box" ? item.fiber_count || "12" : item.kind === "cable" ? item.fiber_count : "",
    box_model: next === "splice_box" ? item.box_model || "emenda" : item.box_model,
  };
}

export function KmlImportReviewModal({
  open,
  fileName,
  projectName,
  skipped,
  initialItems,
  onCancel,
  onConfirm,
}: Props) {
  const [items, setItems] = useState<KmlReviewItem[]>(initialItems);
  const [tab, setTab] = useState<KmlElementKind>("cto");
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [bulkKind, setBulkKind] = useState<KmlElementKind>("cto");
  const [bulkSplitter, setBulkSplitter] = useState("1x8");
  const [bulkCableType, setBulkCableType] = useState("");
  const [bulkFiberCount, setBulkFiberCount] = useState("12");
  const [bulkCableStatus, setBulkCableStatus] = useState("planejado");
  const [bulkBoxModel, setBulkBoxModel] = useState<"emenda" | "distribuicao">("emenda");
  const [bulkFiberColor, setBulkFiberColor] = useState("Desconhecido");
  const [q, setQ] = useState("");

  useEffect(() => {
    if (!open) return;
    setItems(initialItems);
    setSelected(new Set());
    setQ("");
    const firstWithItems = KIND_TABS.find((k) => initialItems.some((i) => i.kind === k)) ?? "cto";
    setTab(firstWithItems);
    setBulkKind(firstWithItems);
  }, [open, initialItems]);

  const counts = useMemo(() => {
    const c = { cto: 0, splice_box: 0, pole: 0, cable: 0, excluded: 0 };
    for (const it of items) {
      if (!it.include) c.excluded++;
      else c[it.kind]++;
    }
    return c;
  }, [items]);

  const visible = useMemo(() => {
    const qq = q.trim().toLowerCase();
    return items.filter((it) => {
      if (it.kind !== tab) return false;
      if (!qq) return true;
      return (
        it.description.toLowerCase().includes(qq) ||
        it.folder.toLowerCase().includes(qq) ||
        KML_KIND_LABELS[it.detectedKind].toLowerCase().includes(qq)
      );
    });
  }, [items, tab, q]);

  const visibleIds = useMemo(() => visible.map((v) => v.id), [visible]);
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id));

  function patchItem(id: string, patch: Partial<KmlReviewItem>) {
    setItems((prev) => prev.map((it) => (it.id === id ? { ...it, ...patch } : it)));
  }

  function toggleSelectAllVisible() {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const id of visibleIds) next.delete(id);
      } else {
        for (const id of visibleIds) next.add(id);
      }
      return next;
    });
  }

  function selectedInTab(): KmlReviewItem[] {
    return items.filter((it) => it.kind === tab && selected.has(it.id));
  }

  function applyBulkKind() {
    const ids = new Set(selectedInTab().map((i) => i.id));
    if (ids.size === 0) return;
    setItems((prev) =>
      prev.map((it) => {
        if (!ids.has(it.id)) return it;
        return changeKind(it, bulkKind) ?? it;
      }),
    );
    setSelected(new Set());
    setTab(bulkKind);
  }

  function applyBulkCtoFields() {
    const ids = new Set(selectedInTab().map((i) => i.id));
    if (ids.size === 0) return;
    const splitter = normalizeSplitterInput(bulkSplitter) ?? bulkSplitter.trim();
    setItems((prev) =>
      prev.map((it) =>
        ids.has(it.id) && it.kind === "cto"
          ? { ...it, splitter, fiber_color: bulkFiberColor }
          : it,
      ),
    );
  }

  function applyBulkCableFields() {
    const ids = new Set(selectedInTab().map((i) => i.id));
    if (ids.size === 0) return;
    setItems((prev) =>
      prev.map((it) =>
        ids.has(it.id) && it.kind === "cable"
          ? {
              ...it,
              cable_type: bulkCableType.trim() || it.cable_type,
              fiber_count: bulkFiberCount || it.fiber_count,
              cable_status: bulkCableStatus || it.cable_status,
            }
          : it,
      ),
    );
  }

  function applyBulkSpliceFields() {
    const ids = new Set(selectedInTab().map((i) => i.id));
    if (ids.size === 0) return;
    const splitter = normalizeSplitterInput(bulkSplitter) ?? bulkSplitter.trim();
    setItems((prev) =>
      prev.map((it) =>
        ids.has(it.id) && it.kind === "splice_box"
          ? {
              ...it,
              box_model: bulkBoxModel,
              fiber_count: bulkFiberCount || it.fiber_count,
              splitter: bulkBoxModel === "distribuicao" ? splitter : it.splitter,
            }
          : it,
      ),
    );
  }

  function excludeSelected() {
    const ids = new Set(selectedInTab().map((i) => i.id));
    if (ids.size === 0) return;
    setItems((prev) => prev.map((it) => (ids.has(it.id) ? { ...it, include: false } : it)));
    setSelected(new Set());
  }

  function includeSelected() {
    const ids = new Set(selectedInTab().map((i) => i.id));
    if (ids.size === 0) return;
    setItems((prev) => prev.map((it) => (ids.has(it.id) ? { ...it, include: true } : it)));
  }

  if (!open) return null;

  const includedCount = items.filter((i) => i.include).length;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onCancel}>
      <div
        className="modal modal--kml-review"
        role="dialog"
        aria-modal="true"
        aria-labelledby="kml-review-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="kml-review__head">
          <div>
            <h2 id="kml-review-title">Revisão da importação KML / KMZ</h2>
            <p className="kml-review__sub">
              {fileName ? <strong>{fileName}</strong> : null}
              {fileName ? " · " : null}
              {projectName}
              {skipped > 0 ? ` · ${skipped} placemark(s) ignorado(s)` : ""}
            </p>
            <p className="kml-review__sub">{kmlReviewSummary(items)}</p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onCancel}>
            ×
          </button>
        </div>

        <div className="kml-review__tabs" role="tablist">
          {KIND_TABS.map((k) => (
            <button
              key={k}
              type="button"
              role="tab"
              aria-selected={tab === k}
              className={`kml-review__tab${tab === k ? " kml-review__tab--active" : ""}`}
              onClick={() => {
                setTab(k);
                setSelected(new Set());
                setBulkKind(k);
              }}
            >
              {KML_KIND_LABELS[k]}
              <span className="kml-review__tab-count">{counts[k]}</span>
            </button>
          ))}
        </div>

        <div className="kml-review__bulk">
          <div className="kml-review__bulk-row">
            <input
              className="input"
              placeholder="Filtrar por nome ou pasta…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              style={{ maxWidth: 260 }}
            />
            <span style={{ fontSize: 12, color: "var(--muted)" }}>
              {selectedInTab().length} seleccionado(s) nesta aba
            </span>
            <button type="button" className="btn btn--sm" onClick={excludeSelected} disabled={selectedInTab().length === 0}>
              Excluir da importação
            </button>
            <button type="button" className="btn btn--sm" onClick={includeSelected} disabled={selectedInTab().length === 0}>
              Incluir novamente
            </button>
          </div>

          <div className="kml-review__bulk-row">
            <span className="kml-review__bulk-label">Tipo em massa</span>
            <select className="input" value={bulkKind} onChange={(e) => setBulkKind(e.target.value as KmlElementKind)}>
              {KIND_TABS.map((k) => (
                <option key={k} value={k}>
                  {KML_KIND_LABELS[k]}
                </option>
              ))}
            </select>
            <button type="button" className="btn btn--sm btn--primary" onClick={applyBulkKind} disabled={selectedInTab().length === 0}>
              Alterar tipo dos seleccionados
            </button>
          </div>

          {tab === "cto" ? (
            <div className="kml-review__bulk-row">
              <span className="kml-review__bulk-label">CTO em massa</span>
              <input
                className="input"
                style={{ maxWidth: 100 }}
                value={bulkSplitter}
                onChange={(e) => setBulkSplitter(e.target.value)}
                placeholder="1x8"
                title="Splitter"
              />
              <select className="input" value={bulkFiberColor} onChange={(e) => setBulkFiberColor(e.target.value)} style={{ maxWidth: 160 }}>
                {FIBER_COLORS.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn--sm btn--primary" onClick={applyBulkCtoFields} disabled={selectedInTab().length === 0}>
                Aplicar splitter / cor
              </button>
            </div>
          ) : null}

          {tab === "cable" ? (
            <div className="kml-review__bulk-row">
              <span className="kml-review__bulk-label">Cabo em massa</span>
              <input
                className="input"
                style={{ maxWidth: 140 }}
                value={bulkCableType}
                onChange={(e) => setBulkCableType(e.target.value)}
                placeholder="Tipo / modelo"
              />
              <select className="input" value={bulkFiberCount} onChange={(e) => setBulkFiberCount(e.target.value)} style={{ maxWidth: 110 }}>
                {CABLE_FIBER_COUNTS.map((n) => (
                  <option key={n} value={String(n)}>
                    {n} fibras
                  </option>
                ))}
              </select>
              <select className="input" value={bulkCableStatus} onChange={(e) => setBulkCableStatus(e.target.value)} style={{ maxWidth: 130 }}>
                {CABLE_STATUSES.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn--sm btn--primary" onClick={applyBulkCableFields} disabled={selectedInTab().length === 0}>
                Aplicar aos cabos
              </button>
            </div>
          ) : null}

          {tab === "splice_box" ? (
            <div className="kml-review__bulk-row">
              <span className="kml-review__bulk-label">Emenda em massa</span>
              <select
                className="input"
                value={bulkBoxModel}
                onChange={(e) => setBulkBoxModel(e.target.value as "emenda" | "distribuicao")}
                style={{ maxWidth: 140 }}
              >
                <option value="emenda">Emenda</option>
                <option value="distribuicao">Distribuição</option>
              </select>
              <select className="input" value={bulkFiberCount} onChange={(e) => setBulkFiberCount(e.target.value)} style={{ maxWidth: 110 }}>
                {CABLE_FIBER_COUNTS.map((n) => (
                  <option key={n} value={String(n)}>
                    {n} fibras
                  </option>
                ))}
              </select>
              <input
                className="input"
                style={{ maxWidth: 100 }}
                value={bulkSplitter}
                onChange={(e) => setBulkSplitter(e.target.value)}
                placeholder="1x8"
                title="Splitter (distribuição)"
              />
              <button type="button" className="btn btn--sm btn--primary" onClick={applyBulkSpliceFields} disabled={selectedInTab().length === 0}>
                Aplicar às emendas
              </button>
            </div>
          ) : null}
        </div>

        <div className="kml-review__table-wrap">
          <table className="conn-table kml-review__table" style={{ fontSize: 12 }}>
            <thead>
              <tr>
                <th style={{ width: 36 }}>
                  <input type="checkbox" checked={allVisibleSelected} onChange={toggleSelectAllVisible} aria-label="Seleccionar todos" />
                </th>
                <th>Incluir</th>
                <th>Descrição</th>
                <th>Tipo</th>
                <th>Detectado</th>
                <th>Pasta</th>
                <th>Coord.</th>
                {tab === "cto" ? (
                  <>
                    <th>Splitter</th>
                    <th>Cor fibra</th>
                  </>
                ) : null}
                {tab === "cable" ? (
                  <>
                    <th>Tipo cabo</th>
                    <th>Fibras</th>
                    <th>Status</th>
                    <th>Pontos</th>
                  </>
                ) : null}
                {tab === "splice_box" ? (
                  <>
                    <th>Modelo</th>
                    <th>Fibras</th>
                    <th>Splitter</th>
                  </>
                ) : null}
                {tab === "pole" ? <th>Tipo poste</th> : null}
              </tr>
            </thead>
            <tbody>
              {visible.length === 0 ? (
                <tr>
                  <td colSpan={12} style={{ color: "var(--muted)", textAlign: "center", padding: 24 }}>
                    Nenhum elemento neste tipo.
                  </td>
                </tr>
              ) : (
                visible.map((it) => (
                  <tr key={it.id} className={!it.include ? "kml-review__row--excluded" : undefined}>
                    <td>
                      <input
                        type="checkbox"
                        checked={selected.has(it.id)}
                        onChange={() => {
                          setSelected((prev) => {
                            const next = new Set(prev);
                            if (next.has(it.id)) next.delete(it.id);
                            else next.add(it.id);
                            return next;
                          });
                        }}
                      />
                    </td>
                    <td>
                      <input type="checkbox" checked={it.include} onChange={(e) => patchItem(it.id, { include: e.target.checked })} />
                    </td>
                    <td>
                      <input
                        className="input"
                        value={it.description}
                        onChange={(e) => patchItem(it.id, { description: e.target.value })}
                        style={{ minWidth: 140 }}
                      />
                    </td>
                    <td>
                      <select
                        className="input"
                        value={it.kind}
                        onChange={(e) => {
                          const next = changeKind(it, e.target.value as KmlElementKind);
                          if (next) patchItem(it.id, next);
                        }}
                        style={{ minWidth: 120 }}
                      >
                        {KIND_TABS.map((k) => (
                          <option key={k} value={k}>
                            {KML_KIND_LABELS[k]}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td style={{ color: "var(--muted)" }}>{KML_KIND_LABELS[it.detectedKind]}</td>
                    <td style={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis" }} title={it.folder}>
                      {it.folder || "—"}
                    </td>
                    <td className="mono" style={{ whiteSpace: "nowrap" }}>
                      {fmtCoord(it.latitude)}, {fmtCoord(it.longitude)}
                    </td>
                    {tab === "cto" ? (
                      <>
                        <td>
                          <input
                            className="input"
                            value={it.splitter}
                            onChange={(e) => patchItem(it.id, { splitter: e.target.value })}
                            placeholder="1x8"
                            style={{ width: 72 }}
                          />
                        </td>
                        <td>
                          <select
                            className="input"
                            value={it.fiber_color}
                            onChange={(e) => patchItem(it.id, { fiber_color: e.target.value })}
                            style={{ minWidth: 110 }}
                          >
                            {FIBER_COLORS.map((c) => (
                              <option key={c} value={c}>
                                {c}
                              </option>
                            ))}
                          </select>
                        </td>
                      </>
                    ) : null}
                    {tab === "cable" ? (
                      <>
                        <td>
                          <input
                            className="input"
                            value={it.cable_type}
                            onChange={(e) => patchItem(it.id, { cable_type: e.target.value })}
                            style={{ minWidth: 100 }}
                          />
                        </td>
                        <td>
                          <select
                            className="input"
                            value={it.fiber_count}
                            onChange={(e) => patchItem(it.id, { fiber_count: e.target.value })}
                            style={{ width: 90 }}
                          >
                            <option value="">—</option>
                            {CABLE_FIBER_COUNTS.map((n) => (
                              <option key={n} value={String(n)}>
                                {n}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td>
                          <select
                            className="input"
                            value={it.cable_status}
                            onChange={(e) => patchItem(it.id, { cable_status: e.target.value })}
                            style={{ minWidth: 110 }}
                          >
                            {CABLE_STATUSES.map((s) => (
                              <option key={s.value} value={s.value}>
                                {s.label}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td className="mono">{it.path?.length ?? 0}</td>
                      </>
                    ) : null}
                    {tab === "splice_box" ? (
                      <>
                        <td>
                          <select
                            className="input"
                            value={it.box_model}
                            onChange={(e) =>
                              patchItem(it.id, { box_model: e.target.value as "emenda" | "distribuicao" })
                            }
                          >
                            <option value="emenda">Emenda</option>
                            <option value="distribuicao">Distribuição</option>
                          </select>
                        </td>
                        <td>
                          <select
                            className="input"
                            value={it.fiber_count}
                            onChange={(e) => patchItem(it.id, { fiber_count: e.target.value })}
                            style={{ width: 90 }}
                          >
                            <option value="">—</option>
                            {CABLE_FIBER_COUNTS.map((n) => (
                              <option key={n} value={String(n)}>
                                {n}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td>
                          <input
                            className="input"
                            value={it.splitter}
                            onChange={(e) => patchItem(it.id, { splitter: e.target.value })}
                            placeholder="1x8"
                            style={{ width: 72 }}
                            disabled={it.box_model !== "distribuicao"}
                          />
                        </td>
                      </>
                    ) : null}
                    {tab === "pole" ? (
                      <td>
                        <input
                          className="input"
                          value={it.pole_type}
                          onChange={(e) => patchItem(it.id, { pole_type: e.target.value })}
                          style={{ minWidth: 100 }}
                        />
                      </td>
                    ) : null}
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        <div className="kml-review__foot">
          <span style={{ fontSize: 12, color: "var(--muted)" }}>
            {includedCount} elemento(s) serão importados
            {counts.excluded > 0 ? ` · ${counts.excluded} excluído(s)` : ""}
          </span>
          <div className="row" style={{ gap: 8 }}>
            <button type="button" className="btn" onClick={onCancel}>
              Cancelar
            </button>
            <button
              type="button"
              className="btn btn--primary"
              disabled={includedCount === 0}
              onClick={() => onConfirm(items)}
            >
              Confirmar e continuar
            </button>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}
