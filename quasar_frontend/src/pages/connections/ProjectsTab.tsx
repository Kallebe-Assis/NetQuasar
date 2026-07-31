import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, FileUp, MapPin, Pencil, Trash2, X } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { ConfirmModal } from "../../components/ConfirmModal";
import { LocationMapModal, type LocationMapPreview } from "../../components/LocationMapModal";
import { PageCountPill } from "../../components/PageCountPill";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { queryKeys } from "../../lib/queryKeys";
import { toastErr, toastOk } from "../../lib/operationToast";
import { filterProjectRows, NETWORK_INFRA_GC_MS, NETWORK_INFRA_STALE_MS } from "../../lib/networkInfraCache";
import { pageCachedQueryOptions, wrapPageCachedQueryFn } from "../../lib/pageDataCache";
import {
  PROJECT_STATUSES,
  fmtCoord,
  parseCoordInput,
  projectStatusLabel,
  type NetworkProject,
} from "../../lib/networkInfrastructure";
import {
  kmlReviewSummary,
  parseKmlToReviewItems,
  reviewItemsToImportElements,
  type KmlReviewItem,
} from "../../lib/parseKmlProject";
import { CoordFields, LocalitySelect } from "./ConnectionsFormFields";
import { ConnectionsPager } from "./ConnectionsPager";
import { ConnectionsTabToolbar } from "./ConnectionsTabToolbar";
import { KmlImportReviewModal } from "./KmlImportReviewModal";
import type { ConnectionsTabProps } from "./shared";
import { useConnectionsLookups } from "./useConnectionsLookups";
import { usePagedRows } from "./usePagedRows";

const EMPTY = {
  description: "",
  locality_id: "",
  color: "#3b82f6",
  status: "planejamento",
  latitude: "",
  longitude: "",
};

export function ProjectsTab({
  canMutate,
  filters,
  prefs,
  onSearchChange,
  onOpenFilters,
  onOpenSettings,
  activeFilterCount,
}: ConnectionsTabProps) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [formOpen, setFormOpen] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [editId, setEditId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [form, setForm] = useState(EMPTY);
  const [mapPreview, setMapPreview] = useState<LocationMapPreview | null>(null);
  const [kmlItems, setKmlItems] = useState<KmlReviewItem[] | null>(null);
  const [kmlFileName, setKmlFileName] = useState<string | null>(null);
  const [kmlSkipped, setKmlSkipped] = useState(0);
  const [kmlReviewOpen, setKmlReviewOpen] = useState(false);
  const [kmlReviewDraft, setKmlReviewDraft] = useState<{
    projectName: string;
    items: KmlReviewItem[];
    skipped: number;
    fileName: string;
  } | null>(null);
  const kmlInputRef = useRef<HTMLInputElement>(null);

  const debouncedQ = useDebouncedValue(filters.q, 320);
  const filterKey = useMemo(
    () =>
      JSON.stringify({
        q: debouncedQ,
        locality_id: filters.locality_id,
        status: filters.projects.status,
        sortDir: prefs.sortDir,
      }),
    [debouncedQ, filters.locality_id, filters.projects.status, prefs.sortDir],
  );

  const listQ = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: wrapPageCachedQueryFn(queryKeys.networkProjects, async () =>
      apiFetch<{ projects: NetworkProject[] }>("/api/v1/commercial/network/projects"),
    ),
    ...pageCachedQueryOptions<{ projects: NetworkProject[] }>(
      queryKeys.networkProjects,
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
    placeholderData: keepPreviousData,
  });

  const sorted = useMemo(() => {
    const rows = filterProjectRows(listQ.data?.projects ?? [], filters, debouncedQ);
    return [...rows].sort((a, b) =>
      prefs.sortDir === "asc" ? a.display_number - b.display_number : b.display_number - a.display_number,
    );
  }, [listQ.data?.projects, filters, debouncedQ, prefs.sortDir]);

  const detailQ = useQuery({
    queryKey: ["network-project", detailId],
    queryFn: () => apiFetch<NetworkProject>(`/api/v1/commercial/network/projects/${detailId}`),
    enabled: !!detailId,
  });

  const { localities } = useConnectionsLookups(formOpen);

  const { safePage, totalPages, pageRows, setPage, rangeFrom, rangeTo } = usePagedRows(
    sorted,
    prefs.pageSize,
    filterKey,
  );

  async function reloadFromDb() {
    try {
      const r = await listQ.refetch();
      if (r.error) {
        toastErr(pushToast, r.error);
      } else {
        toastOk(pushToast, "Projetos recarregados da base de dados.");
      }
    } catch (e) {
      toastErr(pushToast, e);
    }
  }

  const saveMut = useMutation({
    mutationFn: async () => {
      const description = form.description.trim();
      const payload = {
        description,
        locality_id: form.locality_id.trim() || null,
        color: form.color.trim() || null,
        status: form.status,
        latitude: parseCoordInput(form.latitude),
        longitude: parseCoordInput(form.longitude),
      };
      if (!payload.description) throw new Error("Descrição obrigatória.");
      if (editId) {
        return apiFetch(`/api/v1/commercial/network/projects/${editId}`, { method: "PATCH", json: payload });
      }
      if (kmlItems) {
        const elements = reviewItemsToImportElements(kmlItems);
        const total =
          elements.ctos.length + elements.splice_boxes.length + elements.poles.length + elements.cables.length;
        if (total === 0) throw new Error("Nenhum elemento seleccionado para importar.");
        if (payload.latitude == null || payload.longitude == null) {
          const pts = kmlItems.filter((i) => i.include);
          if (pts.length > 0) {
            payload.latitude = pts.reduce((s, p) => s + p.latitude, 0) / pts.length;
            payload.longitude = pts.reduce((s, p) => s + p.longitude, 0) / pts.length;
          }
        }
        return apiFetch<{ id: string; display_number: number; imported: Record<string, number> }>(
          "/api/v1/commercial/network/projects/import/kml",
          {
            method: "POST",
            json: {
              ...payload,
              elements,
            },
          },
        );
      }
      return apiFetch("/api/v1/commercial/network/projects", { method: "POST", json: payload });
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      const wasEdit = !!editId;
      const imported =
        data && typeof data === "object" && data !== null && "imported" in data
          ? (data as { imported: Record<string, number> }).imported
          : null;
      setFormOpen(false);
      setEditId(null);
      setForm(EMPTY);
      setKmlItems(null);
      setKmlFileName(null);
      setKmlSkipped(0);
      setKmlReviewDraft(null);
      if (imported) {
        toastOk(
          pushToast,
          `Projecto criado com ${imported.ctos ?? 0} CTO(s), ${imported.splice_boxes ?? 0} emenda(s), ${imported.poles ?? 0} poste(s), ${imported.cables ?? 0} cabo(s).`,
        );
      } else {
        toastOk(pushToast, wasEdit ? "Projeto actualizado." : "Projeto criado.");
      }
    },
    onError: (e) => toastErr(pushToast, e),
  });

  async function onKmlFile(file: File | null) {
    if (!file) return;
    try {
      const text = await file.text();
      const parsed = parseKmlToReviewItems(text);
      if (parsed.items.length === 0) {
        throw new Error("O KML não contém elementos reconhecidos.");
      }
      setKmlReviewDraft({
        projectName: parsed.projectName,
        items: parsed.items,
        skipped: parsed.skipped,
        fileName: file.name,
      });
      setKmlReviewOpen(true);
      if (!form.description.trim() && parsed.projectName) {
        setForm((f) => ({ ...f, description: parsed.projectName }));
      }
    } catch (e) {
      setKmlReviewDraft(null);
      setKmlReviewOpen(false);
      toastErr(pushToast, e);
    }
  }

  function clearKml() {
    setKmlItems(null);
    setKmlFileName(null);
    setKmlSkipped(0);
    setKmlReviewDraft(null);
    setKmlReviewOpen(false);
  }

  const deleteMut = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/network/projects/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      setDeleteId(null);
      toastOk(pushToast, "Projeto e elementos removidos.");
    },
    onError: (e) => toastErr(pushToast, e),
  });

  if (listQ.isPending && !listQ.data) return <p>A carregar projetos…</p>;
  if (listQ.isError && !listQ.data) return <div className="msg msg--err">{errorMessageFromUnknown(listQ.error)}</div>;

  const detail = detailQ.data;

  function renderElements(title: string, items?: Array<{ display_number: number; description: string }>, prefix?: string) {
    if (!items?.length) return null;
    return (
      <div style={{ marginTop: 10 }}>
        <strong style={{ fontSize: 12 }}>{title}</strong>
        <ul style={{ margin: "6px 0 0", paddingLeft: 18, fontSize: 12 }}>
          {items.map((el) => (
            <li key={`${prefix}-${el.display_number}`}>
              {prefix} {el.display_number} — {el.description}
            </li>
          ))}
        </ul>
      </div>
    );
  }

  return (
    <>
      <ConnectionsTabToolbar
        search={filters.q}
        onSearchChange={onSearchChange}
        searchPlaceholder="Descrição, ID do projeto…"
        onOpenFilters={onOpenFilters}
        onOpenSettings={onOpenSettings}
        activeFilterCount={activeFilterCount}
        onReload={() => void reloadFromDb()}
        reloading={listQ.isFetching}
        reloadTitle="Recarregar projetos da base de dados"
      >
        <PageCountPill label="Projetos" count={sorted.length} />
        {canMutate ? (
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => {
              setEditId(null);
              setForm(EMPTY);
              clearKml();
              setFormOpen(true);
            }}
          >
            Novo projeto
          </button>
        ) : null}
      </ConnectionsTabToolbar>

      <div className="table-wrap">
        <table className="conn-table conn-table--center" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              <th>ID</th>
              <th>Descrição</th>
              <th>Localidade</th>
              <th>Status</th>
              <th>Cor</th>
              <th className="mono">Coord.</th>
              <th style={{ width: 110 }} />
            </tr>
          </thead>
          <tbody>
            {pageRows.map((p) => {
              const hasCoords = p.latitude != null && p.longitude != null;
              return (
              <tr key={p.id}>
                <td className="mono">Projeto {p.display_number}</td>
                <td>{p.description}</td>
                <td>{p.locality_name ?? "—"}</td>
                <td>{projectStatusLabel(p.status)}</td>
                <td>
                  {p.color ? (
                    <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                      <span style={{ width: 12, height: 12, borderRadius: 3, background: p.color, border: "1px solid var(--border)" }} />
                      {p.color}
                    </span>
                  ) : (
                    "—"
                  )}
                </td>
                <td className="mono">
                  {hasCoords ? `${fmtCoord(p.latitude!)}, ${fmtCoord(p.longitude!)}` : "—"}
                </td>
                <td>
                  <div className="conn-row-actions">
                    {hasCoords ? (
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Ver no mapa"
                        onClick={() =>
                          setMapPreview({
                            title: p.description,
                            subtitle: `Projeto ${p.display_number}`,
                            lat: p.latitude!,
                            lng: p.longitude!,
                            kind: "project",
                            color: p.color,
                          })
                        }
                      >
                        <MapPin size={15} />
                      </button>
                    ) : null}
                    <button type="button" className="btn btn--icon" title="Ver elementos" onClick={() => setDetailId(p.id)}>
                      <Eye size={15} />
                    </button>
                    {canMutate ? (
                      <>
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Editar"
                          onClick={() => {
                            setEditId(p.id);
                            setForm({
                              description: p.description,
                              locality_id: p.locality_id ?? "",
                              color: p.color ?? "#3b82f6",
                              status: p.status,
                              latitude: p.latitude != null ? String(p.latitude) : "",
                              longitude: p.longitude != null ? String(p.longitude) : "",
                            });
                            setFormOpen(true);
                          }}
                        >
                          <Pencil size={15} />
                        </button>
                        <button type="button" className="btn btn--icon" title="Remover" onClick={() => setDeleteId(p.id)}>
                          <Trash2 size={15} />
                        </button>
                      </>
                    ) : null}
                  </div>
                </td>
              </tr>
            );
            })}
          </tbody>
        </table>
      </div>

      <ConnectionsPager
        safePage={safePage}
        totalPages={totalPages}
        total={sorted.length}
        rangeFrom={rangeFrom}
        rangeTo={rangeTo}
        onPrev={() => setPage((p) => p - 1)}
        onNext={() => setPage((p) => p + 1)}
      />

      {formOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !saveMut.isPending && setFormOpen(false)}>
          <div
            className="modal conn-form-modal conn-form-modal--infra"
            role="dialog"
            aria-modal="true"
            aria-labelledby="project-form-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="conn-form-modal__head">
              <h2 id="project-form-title">{editId ? "Editar projeto" : "Novo projeto"}</h2>
              <p>Agrupa CTOs, emendas, cabos e postes numa área de implantação.</p>
            </div>
            <div className="conn-form-modal__body">
              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Identificação</h3>
                <div className="conn-form-modal__grid">
                  <div className="conn-form-modal__field field--full">
                    <span className="conn-form-modal__field-label">Descrição *</span>
                    <input className="input" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
                  </div>
                  <LocalitySelect
                    value={form.locality_id}
                    localities={localities}
                    onChange={(id) => setForm({ ...form, locality_id: id })}
                  />
                  <div className="conn-form-modal__field">
                    <span className="conn-form-modal__field-label">Cor</span>
                    <input className="input" type="color" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} />
                  </div>
                  <div className="conn-form-modal__field">
                    <span className="conn-form-modal__field-label">Status</span>
                    <select className="input" value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                      {PROJECT_STATUSES.map((s) => (
                        <option key={s.value} value={s.value}>
                          {s.label}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              </section>
              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Localização</h3>
                <div className="conn-form-modal__grid">
                  <CoordFields
                    latitude={form.latitude}
                    longitude={form.longitude}
                    onChange={(lat, lon) => setForm({ ...form, latitude: lat, longitude: lon })}
                  />
                </div>
              </section>
              {!editId ? (
                <section className="conn-form-modal__section">
                  <h3 className="conn-form-modal__section-title">Importar KML</h3>
                  <p style={{ margin: "0 0 10px", fontSize: 12, color: "var(--muted)" }}>
                    Após escolher o ficheiro, abre-se um modal para rever e corrigir em massa o tipo e os atributos de
                    cada elemento (CTO, emenda, poste, cabo).
                  </p>
                  <input
                    ref={kmlInputRef}
                    type="file"
                    accept=".kml,application/vnd.google-earth.kml+xml,application/xml,text/xml"
                    style={{ display: "none" }}
                    onChange={(e) => {
                      const f = e.target.files?.[0] ?? null;
                      e.target.value = "";
                      void onKmlFile(f);
                    }}
                  />
                  <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "center" }}>
                    <button
                      type="button"
                      className="btn"
                      disabled={saveMut.isPending}
                      onClick={() => kmlInputRef.current?.click()}
                    >
                      <FileUp size={15} style={{ marginRight: 6 }} />
                      Importar KML
                    </button>
                    {kmlItems ? (
                      <>
                        <button
                          type="button"
                          className="btn"
                          disabled={saveMut.isPending}
                          onClick={() => {
                            if (!kmlItems) return;
                            setKmlReviewDraft({
                              projectName: form.description.trim() || "Projecto importado",
                              items: kmlItems,
                              skipped: kmlSkipped,
                              fileName: kmlFileName ?? "KML",
                            });
                            setKmlReviewOpen(true);
                          }}
                        >
                          <Pencil size={15} style={{ marginRight: 6 }} />
                          Rever elementos
                        </button>
                        <button type="button" className="btn btn--icon" title="Remover KML" onClick={clearKml}>
                          <X size={15} />
                        </button>
                      </>
                    ) : null}
                  </div>
                  {kmlItems ? (
                    <div
                      style={{
                        marginTop: 10,
                        padding: "8px 10px",
                        borderRadius: 8,
                        border: "1px solid var(--border)",
                        background: "var(--panel2, transparent)",
                        fontSize: 12,
                      }}
                    >
                      <div style={{ fontWeight: 600 }}>{kmlFileName ?? "KML"}</div>
                      <div style={{ color: "var(--muted)", marginTop: 4 }}>{kmlReviewSummary(kmlItems)}</div>
                    </div>
                  ) : null}
                </section>
              ) : null}
            </div>
            <div className="conn-form-modal__foot">
              <button type="button" className="btn" onClick={() => setFormOpen(false)} disabled={saveMut.isPending}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" onClick={() => saveMut.mutate()} disabled={saveMut.isPending}>
                {kmlItems && !editId ? "Criar e importar" : "Guardar"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {detailId && detail ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setDetailId(null)}>
          <div className="modal modal--wide" onMouseDown={(e) => e.stopPropagation()}>
            <h3>
              Projeto {detail.display_number} — {detail.description}
            </h3>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              {projectStatusLabel(detail.status)}
              {detail.locality_name ? ` · ${detail.locality_name}` : ""}
            </p>
            {renderElements("CTOs", detail.elements?.ctos, "CTO")}
            {renderElements("Caixas de emenda", detail.elements?.splice_boxes, "Emenda")}
            {renderElements("Cabos", detail.elements?.cables, "Cabo")}
            {renderElements("Postes", detail.elements?.poles, "Poste")}
            {!detail.elements?.ctos?.length &&
            !detail.elements?.splice_boxes?.length &&
            !detail.elements?.cables?.length &&
            !detail.elements?.poles?.length ? (
              <p style={{ color: "var(--muted)", fontSize: 12 }}>Nenhum elemento vinculado a este projeto.</p>
            ) : null}
            <div className="row" style={{ justifyContent: "flex-end", marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setDetailId(null)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleteId ? (
        <ConfirmModal
          open
          title="Excluir projeto e elementos"
          message="Esta acção é irreversível. O projeto e todos os elementos vinculados (CTOs, caixas de emenda, cabos e postes) serão permanentemente removidos."
          confirmLabel="Excluir tudo"
          danger
          onCancel={() => setDeleteId(null)}
          onConfirm={() => deleteMut.mutate(deleteId)}
          busy={deleteMut.isPending}
        />
      ) : null}

      {kmlReviewDraft ? (
        <KmlImportReviewModal
          open={kmlReviewOpen}
          fileName={kmlReviewDraft.fileName}
          projectName={kmlReviewDraft.projectName}
          skipped={kmlReviewDraft.skipped}
          initialItems={kmlReviewDraft.items}
          onCancel={() => {
            setKmlReviewOpen(false);
            // Se ainda não havia confirmação prévia, limpa o draft.
            if (!kmlItems) setKmlReviewDraft(null);
          }}
          onConfirm={(items) => {
            setKmlItems(items);
            setKmlFileName(kmlReviewDraft.fileName);
            setKmlSkipped(kmlReviewDraft.skipped);
            setKmlReviewDraft({ ...kmlReviewDraft, items });
            setKmlReviewOpen(false);
            toastOk(pushToast, `Revisão pronta: ${kmlReviewSummary(items)}`);
          }}
        />
      ) : null}

      <LocationMapModal preview={mapPreview} onClose={() => setMapPreview(null)} />
    </>
  );
}
